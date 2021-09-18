package database

import (
	"context"
	"net/url"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Register the postgres database/sql driver.
)

type Config struct {
	User       string
	Password   string
	Host       string
	Name       string
	DisableTLS bool
}

// Open function opens a database connection
func Open(cfg Config) (*sqlx.DB, error) {
	q := url.Values{}

	q.Set("sslmode", "require")
	if cfg.DisableTLS {
		q.Set("sslmode", "disable")
	}
	q.Set("timezone", "utc")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host,
		Path:     cfg.Name,
		RawQuery: q.Encode(),
	}

	return sqlx.Open("postgres", u.String())
}

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func StatusCheck(ctx context.Context, db *sqlx.DB) error {

	// Run a simple query to determine connectivity. The db has a "Ping" method
	// but it can be false-positive when it was previously able to talk to the
	// database but the database has since gone away. Running this query forces a
	// round trip to the database.
	const q = `SELECT true`
	var tmp bool
	return db.QueryRowContext(ctx, q).Scan(&tmp)
}
