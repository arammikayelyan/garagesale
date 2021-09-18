package handlers

import (
	"log"
	"net/http"
	"os"

	"github.com/arammikayelyan/garagesale/internal/mid"
	"github.com/arammikayelyan/garagesale/internal/platform/auth"
	"github.com/arammikayelyan/garagesale/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// API constructs a handler that knows about all API routes
func API(shutdown chan os.Signal, log *log.Logger, db *sqlx.DB, authenticator *auth.Authenticator) http.Handler {
	app := web.NewApp(shutdown, log, mid.Logger(log), mid.Errors(log), mid.Metrics(), mid.Panics())

	c := Check{DB: db}
	app.Handle(http.MethodGet, "/v1/health", c.Health)

	u := Users{DB: db, authenticator: authenticator}
	app.Handle(http.MethodGet, "/v1/users/token", u.Token)

	p := Product{DB: db, Log: log}
	app.Handle(http.MethodGet, "/v1/products", p.List, mid.Authenticate(authenticator))
	app.Handle(http.MethodPost, "/v1/products", p.Create, mid.Authenticate(authenticator))
	app.Handle(http.MethodGet, "/v1/products/{id}", p.Retrieve, mid.Authenticate(authenticator))
	app.Handle(http.MethodPut, "/v1/products/{id}", p.Update, mid.Authenticate(authenticator))
	app.Handle(http.MethodDelete, "/v1/products/{id}", p.Delete, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))

	app.Handle(http.MethodPost, "/v1/products/{id}/sales", p.AddSale, mid.Authenticate(authenticator), mid.HasRole(auth.RoleAdmin))
	app.Handle(http.MethodGet, "/v1/products/{id}/sales", p.ListSales, mid.Authenticate(authenticator))

	return app
}
