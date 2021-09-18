package mid

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/arammikayelyan/garagesale/internal/platform/web"
	"go.opencensus.io/trace"
)

// Logger writes some information about the request to the logs in
// the format: TraceID: (200) GET /foo -> IP ADDR (latency)
func Logger(log *log.Logger) web.Middleware {

	// This is the actual middleware function to be executed.
	f := func(before web.Handler) web.Handler {

		h := func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {

			ctx, span := trace.StartSpan(ctx, "internal.mid.logger")
			defer span.End()

			v, ok := ctx.Value(web.KeyValues).(*web.Values)
			if !ok {
				return web.NewShutdownError("web values missing from context")
				// return errors.New("web values missing from context")
			}

			// Run the handler chain and catch any propagated error.
			err := before(ctx, w, r)

			log.Printf(
				"%s : (%d) : %s %s -> %s (%s)",
				v.TraceID, v.StatusCode,
				r.Method, r.URL.Path,
				r.RemoteAddr, time.Since(v.Start),
			)

			// Return the error to be handled further up the chain.
			return err
		}

		return h
	}

	return f
}
