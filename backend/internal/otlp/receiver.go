// Package otlp implements a minimal embedded OTLP/HTTP+JSON receiver used by
// indie mode. It accepts span batches at /v1/traces, validates a bearer ingest
// token against IngestTokenStore, applies AgentPulse-semantic enrichment, and
// hands each row to a SpanWriter (the DuckDB SpanStore in indie mode).
//
// We deliberately avoid pulling in go.opentelemetry.io/collector — that would
// re-introduce a 60+ MB dependency closure that defeats the indie-mode pitch.
// OTLP/JSON has a stable, small wire format and is enough for SDK senders.
//
//go:build duckdb

package otlp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
	"github.com/agentpulse/agentpulse/backend/internal/store/duckdb"
)

// SpanWriter is the minimal interface the receiver needs from the underlying
// span store. Implemented by *duckdb.SpanStore.
type SpanWriter interface {
	Insert(ctx context.Context, rows []duckdb.SpanRow) error
}

// Receiver is an OTLP/HTTP receiver for indie mode.
type Receiver struct {
	tokens store.IngestTokenStore
	writer SpanWriter
	logger *slog.Logger
}

// NewReceiver constructs an OTLP receiver. tokens is consulted on every request
// to validate the Bearer header; writer receives enriched span rows.
func NewReceiver(tokens store.IngestTokenStore, writer SpanWriter, logger *slog.Logger) *Receiver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Receiver{tokens: tokens, writer: writer, logger: logger}
}

// Handler returns an http.Handler that serves /v1/traces (OTLP/HTTP).
func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.serveTraces)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return mux
}

func (r *Receiver) serveTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := r.authenticate(req)
	if err != nil {
		r.logger.Warn("otlp: auth failed", "err", err, "remote", req.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 64<<20)) // 64 MB cap
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var msg exportTraceServiceRequest
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "invalid OTLP/JSON", http.StatusBadRequest)
		return
	}

	rows := flattenSpans(msg, projectID)
	if len(rows) == 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
		return
	}
	if err := r.writer.Insert(req.Context(), rows); err != nil {
		r.logger.Error("otlp: insert failed", "err", err, "rows", len(rows))
		http.Error(w, "insert", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

// authenticate verifies the Authorization header carries a valid ingest token
// and returns the project_id it belongs to.
func (r *Receiver) authenticate(req *http.Request) (string, error) {
	auth := req.Header.Get("Authorization")
	if auth == "" {
		return "", errors.New("missing Authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", errors.New("Authorization must be Bearer")
	}
	raw := strings.TrimSpace(auth[len(prefix):])
	if raw == "" {
		return "", errors.New("empty bearer")
	}
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])
	tok, err := r.tokens.GetByHash(req.Context(), hash)
	if err != nil {
		return "", fmt.Errorf("token lookup: %w", err)
	}
	return tok.ProjectID, nil
}

// ── OTLP/JSON wire types (subset we read) ────────────────────────────────────

type exportTraceServiceRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scopeSpans struct {
	Spans []otlpSpan `json:"spans"`
}

type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId"`
	Name              string     `json:"name"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Attributes        []keyValue `json:"attributes"`
	Status            otlpStatus `json:"status"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value otlpAny  `json:"value"`
}

type otlpAny struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"` // OTLP encodes 64-bit ints as strings
	DoubleValue *float64 `json:"doubleValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
}

func (a otlpAny) String() string {
	switch {
	case a.StringValue != nil:
		return *a.StringValue
	case a.IntValue != nil:
		return *a.IntValue
	case a.DoubleValue != nil:
		return strconv.FormatFloat(*a.DoubleValue, 'f', -1, 64)
	case a.BoolValue != nil:
		if *a.BoolValue {
			return "true"
		}
		return "false"
	}
	return ""
}

// ── flatten + enrich ──────────────────────────────────────────────────────────

func flattenSpans(req exportTraceServiceRequest, projectID string) []duckdb.SpanRow {
	var rows []duckdb.SpanRow
	for _, rs := range req.ResourceSpans {
		resAttrs := kvToMap(rs.Resource.Attributes)
		serviceName := resAttrs["service.name"]
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				row := flattenSpan(sp, resAttrs, serviceName, projectID)
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func flattenSpan(sp otlpSpan, resAttrs map[string]string, serviceName, fallbackProject string) duckdb.SpanRow {
	attrs := kvToMap(sp.Attributes)

	row := duckdb.SpanRow{
		TraceID:       sp.TraceID,
		SpanID:        sp.SpanID,
		ParentSpanID:  sp.ParentSpanID,
		SpanName:      sp.Name,
		ServiceName:   serviceName,
		StatusCode:    statusCode(sp.Status.Code),
		StatusMessage: sp.Status.Message,
		StartTime:     unixNano(sp.StartTimeUnixNano),
		EndTime:       unixNano(sp.EndTimeUnixNano),
		Attributes:    attrs,
		ResourceAttrs: resAttrs,
	}

	// AgentPulse-semantic enrichment — same keys agentsemanticproc reads in team mode.
	row.ProjectID = pickProjectID(attrs, resAttrs, fallbackProject)
	row.RunID = attrs["agentpulse.run_id"]
	row.SessionID = attrs["agentpulse.session_id"]
	row.UserID = attrs["agentpulse.user_id"]
	row.AgentSpanKind = stringOrDefault(attrs["agentpulse.span_kind"], string(domain.SpanKindUnknown))
	row.AgentName = attrs["agentpulse.agent.name"]
	row.ModelID = attrs["agentpulse.model_id"]
	row.InputTokens = parseUint32(attrs["agentpulse.input_tokens"])
	row.OutputTokens = parseUint32(attrs["agentpulse.output_tokens"])
	row.CostUSD = parseFloat(attrs["agentpulse.cost_usd"])
	row.TtftMs = parseFloat(attrs["agentpulse.ttft_ms"])

	// MCP normalization — promote the un-prefixed semconv attributes to the
	// AgentPulse-prefixed keys so DuckDB queries see a consistent surface.
	normalizeMCP(row.Attributes)
	return row
}

// normalizeMCP copies MCP semantic-convention attributes (mcp.* / agentpulse.mcp.*)
// to a canonical agentpulse.mcp.* key so consumers don't need to handle multiple
// variants. Mirrors collector/processor/agentsemanticproc field_extraction logic.
func normalizeMCP(attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	pairs := []struct{ canonical, alt string }{
		{"agentpulse.mcp.server_name", "mcp.server.name"},
		{"agentpulse.mcp.tool_name", "mcp.tool.name"},
		{"agentpulse.mcp.session_id", "mcp.session.id"},
		{"agentpulse.mcp.request_id", "mcp.request.id"},
		{"agentpulse.mcp.client_name", "mcp.client.name"},
		{"agentpulse.mcp.transport", "mcp.transport"},
	}
	for _, p := range pairs {
		if attrs[p.canonical] == "" {
			if v := attrs[p.alt]; v != "" {
				attrs[p.canonical] = v
			}
		}
	}
}

func pickProjectID(attrs, resAttrs map[string]string, fallback string) string {
	if v := resAttrs["agentpulse.project_id"]; v != "" {
		return v
	}
	if v := attrs["agentpulse.project_id"]; v != "" {
		return v
	}
	return fallback
}

func statusCode(code int) string {
	switch code {
	case 1:
		return string(domain.StatusOK)
	case 2:
		return string(domain.StatusError)
	default:
		return string(domain.StatusUnset)
	}
}

func unixNano(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

func parseUint32(s string) uint32 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func kvToMap(kvs []keyValue) map[string]string {
	if len(kvs) == 0 {
		return nil
	}
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		out[kv.Key] = kv.Value.String()
	}
	return out
}
