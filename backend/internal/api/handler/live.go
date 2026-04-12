package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentpulse/agentpulse/backend/internal/httputil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// LiveHandler serves the SSE endpoint for real-time run streaming.
type LiveHandler struct {
	spans store.SpanStore
	runs  store.RunStore
}

func NewLiveHandler(spans store.SpanStore, runs store.RunStore) *LiveHandler {
	return &LiveHandler{spans: spans, runs: runs}
}

// StreamRunSpans streams new spans for an in-progress run via SSE.
// Route: GET /api/v1/runs/{runID}/live
//
// SSE event types:
//   - event: initial  — full span list as JSON array (on connect)
//   - event: span     — single new span as JSON object (incremental)
//   - event: metrics  — updated run metrics JSON (cost/tokens) after each batch
//   - event: done     — run has finished (no new spans for idle timeout)
//   - : keepalive     — SSE comment sent every 15s to prevent proxy timeouts
func (h *LiveHandler) StreamRunSpans(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")

	// Parse optional idle timeout (default 60s, max 300s).
	idleTimeout := 60 * time.Second
	if v := r.URL.Query().Get("idle_timeout"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 300 {
			idleTimeout = time.Duration(n) * time.Second
		}
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.Error(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Use ResponseController to slide the write deadline forward on each write,
	// defeating the server's 30s WriteTimeout without disabling it globally.
	rc := http.NewResponseController(w)

	writeSSE := func(event, data string) error {
		// Extend write deadline by 35s on each write cycle.
		if err := rc.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
			slog.Warn("live_handler: could not set write deadline", "err", err)
			// Non-fatal: continue even if SetWriteDeadline fails (middleware chain
			// may not support Unwrap). The 30s server timeout may still apply.
		}
		if event != "" {
			if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	writeKeepAlive := func() error {
		if err := rc.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
			slog.Warn("live_handler: could not set write deadline", "err", err)
		}
		if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	ctx := r.Context()

	// Send initial full span list.
	initial, err := h.spans.ListByRun(ctx, runID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		slog.Error("live_handler: initial span fetch failed", "run_id", runID, "err", err)
		return
	}
	initialJSON, _ := json.Marshal(initial)
	if err := writeSSE("initial", string(initialJSON)); err != nil {
		return
	}

	// Track the latest start_time seen so far for incremental polling.
	var lastSeen time.Time
	for _, sp := range initial {
		if sp.StartTime.After(lastSeen) {
			lastSeen = sp.StartTime
		}
	}

	// Idle detection: track when max(end_time) last advanced.
	var lastMaxEnd time.Time
	var stableCount int // consecutive poll cycles where max_end didn't advance

	pollTick := time.NewTicker(2 * time.Second)
	keepaliveTick := time.NewTicker(15 * time.Second)
	defer pollTick.Stop()
	defer keepaliveTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-keepaliveTick.C:
			if err := writeKeepAlive(); err != nil {
				return
			}

		case <-pollTick.C:
			newSpans, err := h.spans.ListByRunSince(ctx, runID, lastSeen)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				slog.Warn("live_handler: poll failed", "run_id", runID, "err", err)
				continue
			}

			for _, sp := range newSpans {
				spJSON, _ := json.Marshal(sp)
				if err := writeSSE("span", string(spJSON)); err != nil {
					return
				}
				if sp.StartTime.After(lastSeen) {
					lastSeen = sp.StartTime
				}
			}

			// Check idle: compute max end_time across ALL spans seen so far.
			// We use a separate query to get the current max_end without pulling all spans.
			if len(newSpans) > 0 {
				var currentMaxEnd time.Time
				for _, sp := range newSpans {
					if sp.EndTime.After(currentMaxEnd) {
						currentMaxEnd = sp.EndTime
					}
				}
				if currentMaxEnd.After(lastMaxEnd) {
					lastMaxEnd = currentMaxEnd
					stableCount = 0
				} else {
					stableCount++
				}
			} else {
				stableCount++
			}

			// Emit done when max_end has been stable for idleTimeout.
			// stableCount * 2s (poll interval) >= idleTimeout.
			if stableCount >= int(idleTimeout.Seconds()/2) && !lastMaxEnd.IsZero() && time.Since(lastMaxEnd) >= idleTimeout {
				_ = writeSSE("done", `{}`)
				return
			}

			// After delivering new spans, emit updated metrics for the cost ticker.
			if len(newSpans) > 0 {
				run, err := h.runs.Get(ctx, runID)
				if err == nil {
					metricsJSON, _ := json.Marshal(run)
					if err := writeSSE("metrics", string(metricsJSON)); err != nil {
						return
					}
				}
			}
		}
	}
}

// StreamProjectSpans streams new spans for ALL runs in a project via SSE.
// Route: GET /api/v1/projects/{projectID}/live
//
// SSE event types:
//   - event: span     — single new span as JSON object (incremental)
//   - : keepalive     — SSE comment sent every 15s to prevent proxy timeouts
//
// Unlike StreamRunSpans, this endpoint never emits a "done" event — the
// project stream is open-ended and only closes when the client disconnects.
func (h *LiveHandler) StreamProjectSpans(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.Error(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	rc := http.NewResponseController(w)

	writeSSE := func(event, data string) error {
		if err := rc.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
			slog.Warn("live_handler: could not set write deadline", "err", err)
		}
		if event != "" {
			if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	writeKeepAlive := func() error {
		if err := rc.SetWriteDeadline(time.Now().Add(35 * time.Second)); err != nil {
			slog.Warn("live_handler: could not set write deadline", "err", err)
		}
		if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	ctx := r.Context()
	lastSeen := time.Now()

	pollTick := time.NewTicker(2 * time.Second)
	keepaliveTick := time.NewTicker(15 * time.Second)
	defer pollTick.Stop()
	defer keepaliveTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-keepaliveTick.C:
			if err := writeKeepAlive(); err != nil {
				return
			}

		case <-pollTick.C:
			newSpans, err := h.spans.ListByProjectSince(ctx, projectID, lastSeen)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				slog.Warn("live_handler: project poll failed", "project_id", projectID, "err", err)
				continue
			}

			for _, sp := range newSpans {
				spJSON, _ := json.Marshal(sp)
				if err := writeSSE("span", string(spJSON)); err != nil {
					return
				}
				if sp.StartTime.After(lastSeen) {
					lastSeen = sp.StartTime
				}
			}
		}
	}
}
