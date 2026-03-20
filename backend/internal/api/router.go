package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/api/handler"
	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// NewRouter builds and returns the full Chi router.
func NewRouter(
	projects store.ProjectStore,
	runs store.RunStore,
	spans store.SpanStore,
	topology store.TopologyStore,
	budget store.BudgetStore,
	hub *alert.Hub,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS)
	r.Use(middleware.Logger)

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	projectHandler := handler.NewProjectHandler(projects)
	runHandler := handler.NewRunHandler(runs, spans)
	topologyHandler := handler.NewTopologyHandler(topology)
	budgetHandler := handler.NewBudgetHandler(budget)

	r.Route("/api/v1", func(r chi.Router) {
		// Projects
		r.Route("/projects", func(r chi.Router) {
			projectHandler.Routes(r)

			// Runs nested under project
			r.Route("/{projectID}/runs", func(r chi.Router) {
				r.Get("/", runHandler.List)
			})

			// Budget nested under project
			r.Route("/{projectID}/budget", func(r chi.Router) {
				budgetHandler.Routes(r)
			})
		})

		// Runs (standalone — for run detail + spans + topology)
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Get("/", runHandler.Get)
			r.Get("/spans", runHandler.ListSpans)
			r.Route("/topology", func(r chi.Router) {
				topologyHandler.Routes(r)
			})
		})

		// WebSocket — real-time budget alerts
		r.Get("/ws/alerts", hub.ServeWS)
	})

	return r
}
