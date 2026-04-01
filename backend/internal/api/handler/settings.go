package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const maxPIICustomRules = 20

// broadnessProbes are test strings used to reject patterns that are far too permissive.
// A pattern that matches any of these is considered "too broad" for use as a PII rule.
var broadnessProbes = []string{"", "hello world"}

// SettingsHandler handles GET and PUT for project PII/redaction settings.
type SettingsHandler struct {
	piiConfigs store.ProjectPIIConfigStore
	pgPool     *pgxpool.Pool
}

func NewSettingsHandler(piiConfigs store.ProjectPIIConfigStore, pgPool *pgxpool.Pool) *SettingsHandler {
	return &SettingsHandler{piiConfigs: piiConfigs, pgPool: pgPool}
}

// GetSettings handles GET /api/v1/projects/{projectID}/settings.
// Authenticated via BearerAuth (read access).
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	cfg, err := h.piiConfigs.Get(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to get settings")
		return
	}
	httputil.JSON(w, http.StatusOK, cfg)
}

type putSettingsRequest struct {
	PIIRedactionEnabled bool                    `json:"pii_redaction_enabled"`
	PIICustomRules      []domain.PIICustomRule  `json:"pii_custom_rules"`
}

// PutSettings handles PUT /api/v1/projects/{projectID}/settings.
// Authenticated via AdminKeyAuth (write access).
func (h *SettingsHandler) PutSettings(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var req putSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.PIICustomRules) > maxPIICustomRules {
		httputil.Error(w, http.StatusBadRequest, "maximum 20 custom rules allowed")
		return
	}

	// Validate each custom rule.
	for i, rule := range req.PIICustomRules {
		if rule.Name == "" {
			httputil.Error(w, http.StatusBadRequest, "rule name must not be empty")
			return
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "rule "+rule.Name+": invalid pattern: "+err.Error())
			return
		}
		for _, probe := range broadnessProbes {
			if compiled.MatchString(probe) {
				httputil.Error(w, http.StatusBadRequest, "rule "+rule.Name+": pattern is too broad")
				return
			}
		}
		_ = i
	}

	cfg := &domain.ProjectPIIConfig{
		ProjectID:           projectID,
		PIIRedactionEnabled: req.PIIRedactionEnabled,
		PIICustomRules:      req.PIICustomRules,
	}
	if cfg.PIICustomRules == nil {
		cfg.PIICustomRules = []domain.PIICustomRule{}
	}

	if err := h.piiConfigs.Upsert(r.Context(), cfg); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	// Audit log — required to provide a trail for admin key usage.
	adminKey := r.Header.Get("X-Admin-Key")
	adminKeyPrefix := ""
	if len(adminKey) >= 8 {
		adminKeyPrefix = adminKey[:8]
	} else {
		adminKeyPrefix = adminKey
	}
	slog.Info("pii settings updated",
		"project_id", projectID,
		"pii_redaction_enabled", req.PIIRedactionEnabled,
		"admin_key_prefix", adminKeyPrefix,
	)

	// Notify the collector via Postgres LISTEN/NOTIFY so it can reload rules.
	if h.pgPool != nil {
		go func() {
			conn, err := h.pgPool.Acquire(context.Background())
			if err != nil {
				slog.Error("pii settings notify: acquire conn", "error", err)
				return
			}
			defer conn.Release()
			if _, err := conn.Exec(context.Background(), "SELECT pg_notify('pii_settings_changed', $1)", projectID); err != nil {
				slog.Error("pii settings notify: pg_notify", "error", err)
			}
		}()
	}

	// Return the fresh config (re-read so timestamps are accurate).
	updated, err := h.piiConfigs.Get(r.Context(), projectID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to retrieve updated settings")
		return
	}
	httputil.JSON(w, http.StatusOK, updated)
}
