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
	evals store.EvalStore,
	evalConfigs store.EvalConfigStore,
	alertRules store.AlertRuleStore,
	analytics store.AnalyticsStore,
	loops store.LoopStore,
	sessions store.SessionStore,
	users store.UserStore,
	search store.SearchStore,
	hub *alert.Hub,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS)
	r.Use(middleware.Logger)

	// Health check — always public
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	projectHandler := handler.NewProjectHandler(projects)
	runHandler := handler.NewRunHandler(runs, spans, loops, topology, evals)
	topologyHandler := handler.NewTopologyHandler(topology)
	budgetHandler := handler.NewBudgetHandler(budget)
	evalHandler := handler.NewEvalHandler(evals)
	evalConfigHandler := handler.NewEvalConfigHandler(evalConfigs)
	alertHandler := handler.NewAlertRuleHandler(alertRules)
	analyticsHandler := handler.NewAnalyticsHandler(analytics)
	loopHandler := handler.NewLoopHandler(loops)
	sessionHandler := handler.NewSessionHandler(sessions, runs)
	userHandler := handler.NewUserHandler(users)
	searchHandler := handler.NewSearchHandler(search)

	bearerAuth := middleware.BearerAuth(projects)
	runAuth := middleware.RunAuth(projects, runs)

	r.Route("/api/v1", func(r chi.Router) {

		// ── Public project routes ─────────────────────────────────────────────
		// GET /projects and POST /projects stay unauthenticated so the frontend
		// can list projects without knowing which key to use, and so new
		// projects can be created from the UI without a prior key.
		r.Get("/projects", projectHandler.List)
		r.Post("/projects", projectHandler.Create)

		// ── Authenticated project-scoped routes ───────────────────────────────
		// All routes under /{projectID} require a valid Bearer token that
		// belongs to that specific project.
		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Use(bearerAuth)
			r.Use(middleware.RateLimit)

			r.Get("/", projectHandler.Get)

			r.Get("/runs", runHandler.List)
			r.Get("/runs/compare", runHandler.Compare)
			r.Get("/evals/summary", evalHandler.SummaryByProject)
			r.Get("/evals/baseline", evalHandler.BaselineByProject)

			r.Route("/evals", func(r chi.Router) {
				evalConfigHandler.Routes(r)
			})

			r.Route("/budget", func(r chi.Router) {
				budgetHandler.Routes(r)
			})

			r.Route("/alerts", func(r chi.Router) {
				alertHandler.Routes(r)
			})

			r.Route("/analytics", func(r chi.Router) {
				analyticsHandler.Routes(r)
			})

			r.Route("/sessions", func(r chi.Router) {
				sessionHandler.Routes(r)
			})

			r.Route("/users", func(r chi.Router) {
				userHandler.Routes(r)
			})

			r.Get("/search", searchHandler.Search)
		})

		// ── Run-scoped routes ─────────────────────────────────────────────────
		// RunAuth resolves the run's project_id from ClickHouse and validates
		// the Bearer token belongs to that project, preventing IDOR.
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Use(runAuth)
			r.Use(middleware.RateLimit)
			r.Get("/", runHandler.Get)
			r.Get("/spans", runHandler.ListSpans)
			r.Get("/evals", evalHandler.ListByRun)
			r.Get("/loops", loopHandler.ListByRun)
			r.Route("/topology", func(r chi.Router) {
				topologyHandler.Routes(r)
			})
		})

		// Cross-project recent alerts — unauthenticated for now
		r.Get("/budget/alerts/recent", budgetHandler.ListRecent)
		r.Get("/alerts/events/recent", alertHandler.ListRecent)

		// WebSocket — real-time budget alerts (validates token inline)
		r.Get("/ws/alerts", hub.ServeWS)
	})

	return r
}
