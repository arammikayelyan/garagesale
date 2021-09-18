package main

import (
	"context"
	"crypto/rsa"
	_ "expvar" // Register the /debug/vars handler
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof" // Register the /debug/pprof handlers
	"os"
	"os/signal"
	"syscall"
	"time"

	"contrib.go.opencensus.io/exporter/zipkin"
	"github.com/arammikayelyan/garagesale/cmd/sales-api/internal/handlers"
	"github.com/arammikayelyan/garagesale/internal/platform/auth"
	"github.com/arammikayelyan/garagesale/internal/platform/conf"
	"github.com/arammikayelyan/garagesale/internal/platform/database"
	"github.com/arammikayelyan/garagesale/internal/schema"
	jwt "github.com/dgrijalva/jwt-go"
	openzipkin "github.com/openzipkin/zipkin-go"
	zipkinHTTP "github.com/openzipkin/zipkin-go/reporter/http"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	log := log.New(os.Stdout, "SALES : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	var cfg struct {
		Web struct {
			Address         string        `conf:"default:localhost:8000"`
			Debug           string        `conf:"default:localhost:6060"`
			ReadTimeout     time.Duration `conf:"default:5s"`
			WriteTimeout    time.Duration `conf:"default:5s"`
			ShutdownTimeout time.Duration `conf:"default:5s"`
		}
		DB struct {
			User       string `conf:"default:postgres"`
			Password   string `conf:"default:postgres,noprint"`
			Host       string `conf:"default:localhost"`
			Name       string `conf:"default:postgres"`
			DisableTLS bool   `conf:"default:false"`
		}
		Auth struct {
			PrivateKeyFile string `conf:"default:private.pem"`
			KeyID          string `conf:"default:1"`
			Algorithm      string `conf:"default:RS256"`
		}
		Trace struct {
			URL         string  `conf:"default:http://localhost:9411/api/v2/spans"`
			Service     string  `conf:"default:sales-api"`
			Probability float64 `conf:"default:1"`
		}
	}

	// App starting
	log.Println("main: Started")
	defer log.Println("main: Completed")

	// Parse configuration
	if err := conf.Parse(os.Args[1:], "SALES", &cfg); err != nil {
		if err == conf.ErrHelpWanted {
			usage, err := conf.Usage("SALES", &cfg)
			if err != nil {
				return errors.Wrap(err, "generated config usage")
			}
			fmt.Println(usage)
			return nil
		}
		return errors.Wrap(err, "parsing config")
	}

	out, err := conf.String(&cfg)
	if err != nil {
		return errors.Wrap(err, "generating config for the output")
	}
	log.Printf("main: Config: \n%v\n", out)

	// Connect to DB
	db, err := database.Open(database.Config{
		Host:       cfg.DB.Host,
		User:       cfg.DB.User,
		Password:   cfg.DB.Password,
		Name:       cfg.DB.Name,
		DisableTLS: cfg.DB.DisableTLS,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// """"""""""""""""""""""""""""
	// Start Tracing Support
	closer, err := registerTracer(
		cfg.Trace.Service,
		cfg.Web.Address,
		cfg.Trace.URL,
		cfg.Trace.Probability,
	)
	if err != nil {
		return err
	}
	defer closer()

	// """"""""""""""""""""""""""
	// Initialize authentication
	authenticator, err := createAuth(
		cfg.Auth.PrivateKeyFile,
		cfg.Auth.KeyID,
		cfg.Auth.Algorithm,
	)
	if err != nil {
		return errors.Wrap(err, "constructing authentication")
	}

	flag.Parse()
	switch flag.Arg(0) {
	case "migrate":
		if err := schema.Migrate(db); err != nil {
			log.Fatal("applying migrations", err)
		}
		log.Println("Migrations complete")
		return nil
	case "seed":
		if err := schema.Seed(db); err != nil {
			log.Fatal("applying seed data", err)
		}
		log.Println("Seed data inserted")
		return nil
	}

	// Start Debug service
	go func() {
		log.Printf("main : Debug service listening on : %s", cfg.Web.Debug)
		err := http.ListenAndServe(cfg.Web.Debug, http.DefaultServeMux)
		log.Printf("main : Debug service ended : %v", err)
	}()

	// Make a channel for listening to interrupts or terminate signal from the OS.
	// Use buffered channel because the signal package requires to.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start API service
	api := &http.Server{
		Addr:         cfg.Web.Address,
		Handler:      handlers.API(shutdown, log, db, authenticator),
		ReadTimeout:  cfg.Web.ReadTimeout,
		WriteTimeout: cfg.Web.WriteTimeout,
	}

	// Make a channel to listen for errors coming from listener. Use a
	// buffered channel so the goroutine can exit if we don't collect the error
	serverErrors := make(chan error, 1)

	// Start the service for listening to requests.
	go func() {
		log.Printf("api listening on: %s", api.Addr)
		serverErrors <- api.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		return errors.Wrap(err, "listening and serving on")
	case sig := <-shutdown:
		log.Println("Start shutdown", sig)

		// give outstanding requests a deadline to shutdown
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		err := api.Shutdown(ctx)
		if err != nil {
			log.Printf("main: graceful shutdown did not complete in %v: %v", cfg.Web.ShutdownTimeout, err)
			err = api.Close()
		}
		if err != nil {
			return errors.Wrap(err, "shutdown gracefully")
		}

		if sig == syscall.SIGSTOP {
			return errors.New("integrity error detected, asking for self shutdown")
		}
	}

	return nil
}

func createAuth(privateKeyFile, keyID, algorithm string) (*auth.Authenticator, error) {

	keyContents, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "reading auth private key")
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyContents)
	if err != nil {
		return nil, errors.Wrap(err, "parsing auth private key")
	}

	public := auth.NewSimpleKeyLookupFunc(keyID, key.Public().(*rsa.PublicKey))

	return auth.NewAuthenticator(key, keyID, algorithm, public)
}

func registerTracer(service, httpAddr, traceURL string, probability float64) (func() error, error) {
	localEndpoint, err := openzipkin.NewEndpoint(service, httpAddr)
	if err != nil {
		return nil, errors.Wrap(err, "creating the local zipkinEndpoint")
	}
	reporter := zipkinHTTP.NewReporter(traceURL)

	trace.RegisterExporter(zipkin.NewExporter(reporter, localEndpoint))
	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.ProbabilitySampler(probability),
	})

	return reporter.Close, nil
}
