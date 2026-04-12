package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const (
	exportMaxRows       = 500_000
	exportTimeout       = 120 * time.Second
	exportFlushInterval = 100 // flush every N rows
)

// ExportHandler serves streaming data export endpoints.
type ExportHandler struct {
	exports   store.ExportStore
	analytics store.AnalyticsStore

	// inFlight tracks per-project concurrency: only one export per project at a time.
	inFlight sync.Map
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(exports store.ExportStore, analytics store.AnalyticsStore) *ExportHandler {
	return &ExportHandler{
		exports:   exports,
		analytics: analytics,
	}
}

// Routes mounts the export sub-routes.
func (h *ExportHandler) Routes(r chi.Router) {
	r.Get("/spans", h.ExportSpans)
	r.Get("/runs", h.ExportRuns)
	r.Get("/analytics", h.ExportAnalytics)
}

// exportParams holds parsed query parameters shared across export endpoints.
type exportParams struct {
	Format    string // "csv" or "jsonl"
	From      time.Time
	To        time.Time
	AgentName string
	Model     string
	Window    int // seconds, for analytics only
}

// parseExportParams extracts and validates shared export query parameters.
func parseExportParams(r *http.Request) (*exportParams, error) {
	q := r.URL.Query()

	format := q.Get("format")
	if format == "" {
		format = "jsonl"
	}
	if format != "csv" && format != "jsonl" {
		return nil, fmt.Errorf("invalid format %q: must be csv or jsonl", format)
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, fmt.Errorf("invalid from parameter: %w", err)
		}
		from = t.UTC()
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, fmt.Errorf("invalid to parameter: %w", err)
		}
		to = t.UTC()
	}

	windowSeconds := 24 * 3600
	switch q.Get("window") {
	case "7d":
		windowSeconds = 7 * 24 * 3600
	}

	return &exportParams{
		Format:    format,
		From:      from,
		To:        to,
		AgentName: q.Get("agent_name"),
		Model:     q.Get("model"),
		Window:    windowSeconds,
	}, nil
}

// acquireExport tries to acquire the per-project export lock.
// Returns false if another export is already in-flight for the project.
func (h *ExportHandler) acquireExport(projectID string) bool {
	_, loaded := h.inFlight.LoadOrStore(projectID, true)
	return !loaded
}

// releaseExport releases the per-project export lock.
func (h *ExportHandler) releaseExport(projectID string) {
	h.inFlight.Delete(projectID)
}

// setExportHeaders sets common response headers for export endpoints.
func setExportHeaders(w http.ResponseWriter, format, entity string, count int64) {
	today := time.Now().UTC().Format("2006-01-02")
	ext := ".jsonl"
	contentType := "application/x-ndjson"
	if format == "csv" {
		ext = ".csv"
		contentType = "text/csv; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s%s", entity, today, ext))
	w.Header().Set("X-Export-Row-Count", strconv.FormatInt(count, 10))
	w.Header().Set("Transfer-Encoding", "chunked")
}

// ExportSpans streams span data as CSV or JSON Lines.
func (h *ExportHandler) ExportSpans(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	if !h.acquireExport(projectID) {
		httputil.Error(w, http.StatusTooManyRequests, "An export is already in progress for this project. Please wait and retry.")
		return
	}
	defer h.releaseExport(projectID)

	ctx, cancel := context.WithTimeout(r.Context(), exportTimeout)
	defer cancel()

	params, err := parseExportParams(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	ep := &domain.ExportParams{
		ProjectID: projectID,
		From:      params.From,
		To:        params.To,
		AgentName: params.AgentName,
		Model:     params.Model,
	}

	// Preflight count check.
	count, err := h.exports.CountSpans(ctx, ep)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to count spans for export")
		return
	}
	if count > exportMaxRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("Export too large (%d rows). Narrow your time range or add filters.", count),
			"count": count,
		})
		return
	}

	setExportHeaders(w, params.Format, "spans", count)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	if params.Format == "csv" {
		h.exportSpansCSV(ctx, w, flusher, ep)
	} else {
		h.exportSpansJSONL(ctx, w, flusher, ep)
	}
}

func (h *ExportHandler) exportSpansCSV(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, params *domain.ExportParams) {
	cw := csv.NewWriter(w)
	header := []string{
		"trace_id", "span_id", "parent_span_id", "run_id",
		"agent_span_kind", "agent_name", "model_id",
		"span_name", "service_name", "status_code", "status_message",
		"start_time", "end_time", "duration_ms",
		"input_tokens", "output_tokens", "total_tokens", "cost_usd",
	}
	_ = cw.Write(header)

	rowIdx := 0
	_ = h.exports.ExportSpans(ctx, params, func(row *domain.ExportSpanRow) error {
		record := []string{
			row.TraceID, row.SpanID, row.ParentSpanID, row.RunID,
			row.AgentSpanKind, row.AgentName, row.ModelID,
			row.SpanName, row.ServiceName, row.StatusCode, row.StatusMessage,
			row.StartTime.Format(time.RFC3339Nano),
			row.EndTime.Format(time.RFC3339Nano),
			strconv.FormatFloat(row.DurationMS, 'f', 2, 64),
			strconv.FormatUint(uint64(row.InputTokens), 10),
			strconv.FormatUint(uint64(row.OutputTokens), 10),
			strconv.FormatUint(uint64(row.TotalTokens), 10),
			strconv.FormatFloat(row.CostUSD, 'f', 8, 64),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
		rowIdx++
		if rowIdx%exportFlushInterval == 0 {
			cw.Flush()
			if flusher != nil {
				flusher.Flush()
			}
		}
		return nil
	})
	cw.Flush()
	if flusher != nil {
		flusher.Flush()
	}
}

func (h *ExportHandler) exportSpansJSONL(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, params *domain.ExportParams) {
	enc := json.NewEncoder(w)

	rowIdx := 0
	_ = h.exports.ExportSpans(ctx, params, func(row *domain.ExportSpanRow) error {
		if err := enc.Encode(row); err != nil {
			return err
		}
		rowIdx++
		if rowIdx%exportFlushInterval == 0 && flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if flusher != nil {
		flusher.Flush()
	}
}

// ExportRuns streams run data as CSV or JSON Lines.
func (h *ExportHandler) ExportRuns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	if !h.acquireExport(projectID) {
		httputil.Error(w, http.StatusTooManyRequests, "An export is already in progress for this project. Please wait and retry.")
		return
	}
	defer h.releaseExport(projectID)

	ctx, cancel := context.WithTimeout(r.Context(), exportTimeout)
	defer cancel()

	params, err := parseExportParams(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	ep := &domain.ExportParams{
		ProjectID: projectID,
		From:      params.From,
		To:        params.To,
		AgentName: params.AgentName,
		Model:     params.Model,
	}

	count, err := h.exports.CountRuns(ctx, ep)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to count runs for export")
		return
	}
	if count > exportMaxRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": fmt.Sprintf("Export too large (%d rows). Narrow your time range or add filters.", count),
			"count": count,
		})
		return
	}

	setExportHeaders(w, params.Format, "runs", count)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	if params.Format == "csv" {
		h.exportRunsCSV(ctx, w, flusher, ep)
	} else {
		h.exportRunsJSONL(ctx, w, flusher, ep)
	}
}

func (h *ExportHandler) exportRunsCSV(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, params *domain.ExportParams) {
	cw := csv.NewWriter(w)
	header := []string{
		"run_id", "trace_id", "session_id", "user_id",
		"start_time", "end_time",
		"span_count", "llm_calls", "tool_calls",
		"input_tokens", "output_tokens", "total_tokens", "total_cost_usd",
		"error_count",
	}
	_ = cw.Write(header)

	rowIdx := 0
	_ = h.exports.ExportRuns(ctx, params, func(row *domain.ExportRunRow) error {
		record := []string{
			row.RunID, row.TraceID, row.SessionID, row.UserID,
			row.StartTime.Format(time.RFC3339Nano),
			row.EndTime.Format(time.RFC3339Nano),
			strconv.FormatUint(row.SpanCount, 10),
			strconv.FormatUint(row.LLMCalls, 10),
			strconv.FormatUint(row.ToolCalls, 10),
			strconv.FormatUint(row.InputTokens, 10),
			strconv.FormatUint(row.OutputTokens, 10),
			strconv.FormatUint(row.TotalTokens, 10),
			strconv.FormatFloat(row.TotalCostUSD, 'f', 8, 64),
			strconv.FormatUint(row.ErrorCount, 10),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
		rowIdx++
		if rowIdx%exportFlushInterval == 0 {
			cw.Flush()
			if flusher != nil {
				flusher.Flush()
			}
		}
		return nil
	})
	cw.Flush()
	if flusher != nil {
		flusher.Flush()
	}
}

func (h *ExportHandler) exportRunsJSONL(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, params *domain.ExportParams) {
	enc := json.NewEncoder(w)

	rowIdx := 0
	_ = h.exports.ExportRuns(ctx, params, func(row *domain.ExportRunRow) error {
		if err := enc.Encode(row); err != nil {
			return err
		}
		rowIdx++
		if rowIdx%exportFlushInterval == 0 && flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if flusher != nil {
		flusher.Flush()
	}
}

// analyticsExportRow is a combined analytics row for export, with a type discriminator.
type analyticsExportRow struct {
	Type string `json:"type"`

	// Tool stats fields
	ToolName     string  `json:"tool_name,omitempty"`
	CallCount    uint64  `json:"call_count"`
	ErrorCount   uint64  `json:"error_count"`
	ErrorRate    float64 `json:"error_rate,omitempty"`
	AvgLatencyMS float64 `json:"avg_latency_ms,omitempty"`
	P95LatencyMS float64 `json:"p95_latency_ms,omitempty"`

	// Agent cost fields
	AgentName      string  `json:"agent_name,omitempty"`
	CostPercent    float64 `json:"cost_percent,omitempty"`
	AvgCostPerCall float64 `json:"avg_cost_per_call,omitempty"`

	// Model stats fields
	ModelID              string  `json:"model_id,omitempty"`
	InputTokens          uint64  `json:"input_tokens,omitempty"`
	OutputTokens         uint64  `json:"output_tokens,omitempty"`
	TotalTokens          uint64  `json:"total_tokens,omitempty"`
	CostPerMillionTokens float64 `json:"cost_per_million_tokens,omitempty"`

	// Shared cost field
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// ExportAnalytics exports combined analytics data (tool stats, agent cost, model stats).
func (h *ExportHandler) ExportAnalytics(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	if !h.acquireExport(projectID) {
		httputil.Error(w, http.StatusTooManyRequests, "An export is already in progress for this project. Please wait and retry.")
		return
	}
	defer h.releaseExport(projectID)

	ctx, cancel := context.WithTimeout(r.Context(), exportTimeout)
	defer cancel()

	params, err := parseExportParams(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Fetch all three analytics data sets (small data, no streaming needed).
	tools, err := h.analytics.ToolStats(ctx, projectID, params.Window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query tool stats for export")
		return
	}
	agents, err := h.analytics.AgentCostStats(ctx, projectID, params.Window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query agent cost stats for export")
		return
	}
	models, err := h.analytics.ModelStats(ctx, projectID, params.Window)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "failed to query model stats for export")
		return
	}

	totalRows := int64(len(tools) + len(agents) + len(models))
	setExportHeaders(w, params.Format, "analytics", totalRows)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	// Build combined rows.
	var rows []analyticsExportRow
	for _, t := range tools {
		rows = append(rows, analyticsExportRow{
			Type:         "tool_stats",
			ToolName:     t.ToolName,
			CallCount:    t.CallCount,
			ErrorCount:   t.ErrorCount,
			ErrorRate:    t.ErrorRate,
			AvgLatencyMS: t.AvgLatencyMS,
			P95LatencyMS: t.P95LatencyMS,
			TotalCostUSD: t.TotalCostUSD,
		})
	}
	for _, a := range agents {
		rows = append(rows, analyticsExportRow{
			Type:           "agent_cost",
			AgentName:      a.AgentName,
			CallCount:      a.CallCount,
			TotalCostUSD:   a.TotalCostUSD,
			CostPercent:    a.CostPercent,
			AvgCostPerCall: a.AvgCostPerCall,
		})
	}
	for _, m := range models {
		rows = append(rows, analyticsExportRow{
			Type:                 "model_stats",
			ModelID:              m.ModelID,
			CallCount:            m.CallCount,
			ErrorCount:           m.ErrorCount,
			ErrorRate:            m.ErrorRate,
			AvgLatencyMS:         m.AvgLatencyMS,
			P95LatencyMS:         m.P95LatencyMS,
			TotalCostUSD:         m.TotalCostUSD,
			AvgCostPerCall:       m.AvgCostPerCall,
			InputTokens:          m.InputTokens,
			OutputTokens:         m.OutputTokens,
			TotalTokens:          m.TotalTokens,
			CostPerMillionTokens: m.CostPerMillionTokens,
		})
	}

	if params.Format == "csv" {
		h.exportAnalyticsCSV(w, rows)
	} else {
		h.exportAnalyticsJSONL(w, rows)
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func (h *ExportHandler) exportAnalyticsCSV(w http.ResponseWriter, rows []analyticsExportRow) {
	cw := csv.NewWriter(w)
	header := []string{
		"type", "tool_name", "agent_name", "model_id",
		"call_count", "error_count", "error_rate",
		"avg_latency_ms", "p95_latency_ms",
		"total_cost_usd", "cost_percent", "avg_cost_per_call",
		"input_tokens", "output_tokens", "total_tokens", "cost_per_million_tokens",
	}
	_ = cw.Write(header)

	for _, row := range rows {
		record := []string{
			row.Type, row.ToolName, row.AgentName, row.ModelID,
			strconv.FormatUint(row.CallCount, 10),
			strconv.FormatUint(row.ErrorCount, 10),
			strconv.FormatFloat(row.ErrorRate, 'f', 2, 64),
			strconv.FormatFloat(row.AvgLatencyMS, 'f', 2, 64),
			strconv.FormatFloat(row.P95LatencyMS, 'f', 2, 64),
			strconv.FormatFloat(row.TotalCostUSD, 'f', 8, 64),
			strconv.FormatFloat(row.CostPercent, 'f', 2, 64),
			strconv.FormatFloat(row.AvgCostPerCall, 'f', 8, 64),
			strconv.FormatUint(row.InputTokens, 10),
			strconv.FormatUint(row.OutputTokens, 10),
			strconv.FormatUint(row.TotalTokens, 10),
			strconv.FormatFloat(row.CostPerMillionTokens, 'f', 4, 64),
		}
		_ = cw.Write(record)
	}
	cw.Flush()
}

func (h *ExportHandler) exportAnalyticsJSONL(w http.ResponseWriter, rows []analyticsExportRow) {
	enc := json.NewEncoder(w)
	for _, row := range rows {
		_ = enc.Encode(row)
	}
}
