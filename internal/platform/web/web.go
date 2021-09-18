package web

import (
	"context"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"go.opencensus.io/plugin/ochttp"
	_ "go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	_ "go.opencensus.io/plugin/ochttp/propagation/tracecontext"
	"go.opencensus.io/trace"
)

// ctxKey represents the type of context key.
type ctxKey int

// KeyValues is how request values or stored/retreived.
const KeyValues ctxKey = 1

// Values carries information about each request.
type Values struct {
	StatusCode int
	Start      time.Time
	TraceID    string
}

// Handler is the signature that all application handlers will implement
type Handler func(context.Context, http.ResponseWriter, *http.Request) error

type App struct {
	mux      *chi.Mux
	log      *log.Logger
	mw       []Middleware
	och      *ochttp.Handler
	shutdown chan os.Signal
}

// NewApp constructs an App to handle a set of routes. Any middleware
// provided will be ran for every request.
func NewApp(shutdown chan os.Signal, logger *log.Logger, mw ...Middleware) *App {
	app := App{
		mux:      chi.NewRouter(),
		log:      logger,
		mw:       mw,
		shutdown: shutdown,
	}

	// Create an OpenCensus HTTP Handler which wraps the router. This will
	// start the initial span and annotate it with information about the
	// request/response.
	//
	// This is configured to use the W3C TraceContext standard to set the
	// remote parent if an client request includes the appropriate headers.
	// https://w3c.github.io/trace-context/
	app.och = &ochttp.Handler{
		Handler:     app.mux,
		Propagation: &tracecontext.HTTPFormat{},
	}

	return &app
}

// Handle connects a http method with URL pattern to a particular application handler
// It converts our custom handler type to the std lib Handler type. It captures
// errors from the handler and serves them to the client in a uniform way.
func (a *App) Handle(method, pattern string, h Handler, mw ...Middleware) {

	// First wrap handler specific middleware around this handler.
	h = wrapMiddleware(mw, h)

	// Add the application's general middleware to the handler chain.
	h = wrapMiddleware(a.mw, h)

	// Because of the fact that MethodFunc accepts http.HandleFunc we cannaot send our
	// custom handler as an argument. That is why we create anonymous function which
	// translates from our custom handler to the expected handler.
	fn := func(w http.ResponseWriter, r *http.Request) {

		ctx, span := trace.StartSpan(r.Context(), "internal.platform.web")
		defer span.End()

		// Create a Values struct to record state for the request. Store the
		// address in the request's context so it is sent down the call chain.
		v := Values{
			TraceID: span.SpanContext().TraceID.String(),
			Start:   time.Now(),
		}
		ctx = context.WithValue(ctx, KeyValues, &v)

		// Run the handler chain and catch any propagated error.
		if err := h(ctx, w, r); err != nil {
			a.log.Printf("%s : Unhandled error %+v", v.TraceID, err)
			if IsShutdown(err) {
				a.SignalShutdown()
			}
		}
	}

	a.mux.MethodFunc(method, pattern, fn)
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.och.ServeHTTP(w, r)
}

// SignalShutdown is used to graacefully shutdown the app when an integrity
// issue is identified.
func (a *App) SignalShutdown() {
	a.log.Println("error returned from handler indicated integrity issue, shutting down service")
	a.shutdown <- syscall.SIGSTOP
}
