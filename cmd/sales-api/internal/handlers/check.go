package handlers

import (
	"context"
	"net/http"

	"github.com/arammikayelyan/garagesale/internal/platform/database"
	"github.com/arammikayelyan/garagesale/internal/platform/web"
	"github.com/jmoiron/sqlx"
)

// Check has handlers to implement service orchestration.
type Check struct {
	DB *sqlx.DB
}

// Health responds with 200 OK if service is healthy and ready to traffic
func (c *Check) Health(ctx context.Context, w http.ResponseWriter, r *http.Request) error {

	var health struct {
		Status string `json:"status"`
	}
	if err := database.StatusCheck(ctx, c.DB); err != nil {
		health.Status = "db not ready"
		return web.Respond(ctx, w, health, http.StatusInternalServerError)
	}

	health.Status = "OK"
	return web.Respond(ctx, w, health, http.StatusOK)
}
