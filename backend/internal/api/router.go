package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/alert"
	"github.com/agentpulse/agentpulse/backend/internal/api/handler"
	"github.com/agentpulse/agentpulse/backend/internal/api/middleware"
	"github.com/agentpulse/agentpulse/backend/internal/eval"
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
	piiConfigs store.ProjectPIIConfigStore,
	spanFeedback store.SpanFeedbackStore,
	payloads store.PayloadStore,
	pgPool *pgxpool.Pool,
	hub *alert.Hub,
	corsAllowedOrigins []string,
	corsDevMode bool,
	providerKeys eval.ProviderKeys,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.Recoverer)
	r.Use(middleware.NewCORS(corsAllowedOrigins, corsDevMode))
	r.Use(middleware.Logger)

	// Health check — always public
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	projectHandler := handler.NewProjectHandler(projects)
	runHandler := handler.NewRunHandler(runs, spans, loops, topology, evals, payloads)
	topologyHandler := handler.NewTopologyHandler(topology)
	budgetHandler := handler.NewBudgetHandler(budget)
	evalHandler := handler.NewEvalHandler(evals, spanFeedback)
	evalConfigHandler := handler.NewEvalConfigHandlerWithKeys(evalConfigs, providerKeys)
	alertHandler := handler.NewAlertRuleHandler(alertRules)
	analyticsHandler := handler.NewAnalyticsHandler(analytics)
	loopHandler := handler.NewLoopHandler(loops)
	sessionHandler := handler.NewSessionHandler(sessions, runs)
	userHandler := handler.NewUserHandler(users)
	searchHandler := handler.NewSearchHandler(search)
	settingsHandler := handler.NewSettingsHandler(piiConfigs, pgPool)
	feedbackWriteLimiter := middleware.NewRateLimiter(10, time.Minute)
	spanFeedbackHandler := handler.NewSpanFeedbackHandler(spanFeedback)

	bearerAuth := middleware.BearerAuth(projects)
	adminKeyAuth := middleware.AdminKeyAuth(projects)
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
				r.Get("/alerts/recent", budgetHandler.ListRecent)
			})

			r.Route("/alerts", func(r chi.Router) {
				alertHandler.Routes(r)
				r.Get("/events/recent", alertHandler.ListRecent)
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

			// Span feedback — human-in-the-loop ratings.
			// POST uses a tighter write limiter (10/min) to prevent bulk phantom writes.
			// GET/DELETE share the default 60/min bucket from the parent group.
			r.With(feedbackWriteLimiter.Middleware()).
				Post("/spans/{spanID}/feedback", spanFeedbackHandler.Upsert)
			r.Get("/spans/{spanID}/feedback", spanFeedbackHandler.Get)
			r.Delete("/spans/{spanID}/feedback", spanFeedbackHandler.Delete)
			r.Get("/runs/{runID}/feedback", spanFeedbackHandler.ListByRun)

			// Settings (read) — authenticated via BearerAuth inherited from parent route group.
			r.Get("/settings", settingsHandler.GetSettings)
		})

		// Settings (write) — authenticated via AdminKeyAuth (separate from SDK Bearer token).
		// Mounted outside the bearerAuth group since it uses a different auth mechanism.
		r.Route("/projects/{projectID}/settings", func(r chi.Router) {
			r.Use(adminKeyAuth)
			r.Put("/", settingsHandler.PutSettings)
		})

		// ── Run-scoped routes ─────────────────────────────────────────────────
		// RunAuth resolves the run's project_id from ClickHouse and validates
		// the Bearer token belongs to that project, preventing IDOR.
		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Use(runAuth)
			r.Use(middleware.RateLimit)
			r.Get("/", runHandler.Get)
			r.Get("/spans", runHandler.ListSpans)
			r.Get("/spans/{spanID}", runHandler.GetSpan)
			r.Get("/evals", evalHandler.ListByRun)
			r.Get("/evals/grouped", evalHandler.ListByRunGrouped)
			r.Get("/loops", loopHandler.ListByRun)
			r.Route("/topology", func(r chi.Router) {
				topologyHandler.Routes(r)
			})
		})

		// WebSocket — real-time budget alerts (validates token inline)
		r.Get("/ws/alerts", hub.ServeWS)
	})

	return r
}
