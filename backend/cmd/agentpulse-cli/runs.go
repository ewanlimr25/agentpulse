package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ── runs list ─────────────────────────────────────────────────────────────

type runSummary struct {
	RunID        string  `json:"RunID"`
	StartTime    string  `json:"StartTime"`
	EndTime      string  `json:"EndTime"`
	DurationMS   float64 `json:"DurationMS"`
	SpanCount    uint64  `json:"SpanCount"`
	TotalCostUSD float64 `json:"TotalCostUSD"`
	Status       string  `json:"Status"`
	IsActive     bool    `json:"IsActive"`
	LoopDetected bool    `json:"LoopDetected"`
}

type runsListData struct {
	Runs   []runSummary `json:"runs"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
	Total  int          `json:"total"`
}

type runsListEnvelope struct {
	Data  *runsListData `json:"data"`
	Error string        `json:"error"`
}

func runRunsList(args []string) {
	fs := flag.NewFlagSet("runs list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		project  = fs.String("project", "", "Project ID (required)")
		apiKey   = fs.String("api-key", "", "API key (or set AGENTPULSE_API_KEY)")
		limit    = fs.Int("limit", 20, "Number of runs to show (1–100)")
		endpoint = fs.String("endpoint", "", "AgentPulse API base URL (or set AGENTPULSE_ENDPOINT)")
		jsonOut  = fs.Bool("json", false, "Output JSON instead of human-readable text")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			os.Exit(exitError)
		}
		os.Exit(exitError)
	}

	if *apiKey == "" {
		*apiKey = os.Getenv("AGENTPULSE_API_KEY")
	}
	if *endpoint == "" {
		if v := os.Getenv("AGENTPULSE_ENDPOINT"); v != "" {
			*endpoint = v
		} else {
			*endpoint = "https://api.agentpulse.io"
		}
	}

	var errs []string
	if *project == "" {
		errs = append(errs, "--project is required")
	}
	if *apiKey == "" {
		errs = append(errs, "--api-key or AGENTPULSE_API_KEY is required")
	}
	if *limit < 1 || *limit > 100 {
		errs = append(errs, "--limit must be between 1 and 100")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "error:", e)
		}
		os.Exit(exitError)
	}

	if err := validateEndpoint(*endpoint); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}

	reqURL := fmt.Sprintf("%s/api/v1/projects/%s/runs?limit=%d&offset=0",
		strings.TrimRight(*endpoint, "/"), url.PathEscape(*project), *limit)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: failed to build request:", err)
		os.Exit(exitError)
	}
	req.Header.Set("Authorization", "Bearer "+*apiKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: request failed:", err)
		os.Exit(exitError)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "error: authentication failed (401) — check AGENTPULSE_API_KEY")
		os.Exit(exitError)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: unexpected HTTP status %d\n", resp.StatusCode)
		os.Exit(exitError)
	}

	var env runsListEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		fmt.Fprintln(os.Stderr, "error: failed to decode response:", err)
		os.Exit(exitError)
	}
	if env.Error != "" {
		fmt.Fprintln(os.Stderr, "error:", env.Error)
		os.Exit(exitError)
	}
	if env.Data == nil {
		fmt.Fprintln(os.Stderr, "error: empty response from API")
		os.Exit(exitError)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(env.Data)
		os.Exit(exitPass)
	}

	printRunsTable(env.Data)
	os.Exit(exitPass)
}

func printRunsTable(data *runsListData) {
	fmt.Printf("Project runs  (showing %d of %d)\n\n", len(data.Runs), data.Total)
	fmt.Printf("%-26s  %-20s  %-8s  %8s  %10s  %6s  %s\n",
		"RUN ID", "STARTED", "DURATION", "SPANS", "COST", "STATUS", "FLAGS")
	fmt.Println(strings.Repeat("-", 90))
	for _, r := range data.Runs {
		started := formatTime(r.StartTime)
		duration := formatDurationMS(r.DurationMS)
		cost := fmt.Sprintf("$%.4f", r.TotalCostUSD)
		status := r.Status
		flags := ""
		if r.IsActive {
			flags += "LIVE "
		}
		if r.LoopDetected {
			flags += "LOOP"
		}
		runID := r.RunID
		if len(runID) > 24 {
			runID = runID[:24]
		}
		fmt.Printf("%-26s  %-20s  %-8s  %8d  %10s  %6s  %s\n",
			runID, started, duration, r.SpanCount, cost, status, flags)
	}
}

func formatTime(iso string) string {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return iso
	}
	since := time.Since(t)
	switch {
	case since < time.Minute:
		return fmt.Sprintf("%ds ago", int(since.Seconds()))
	case since < time.Hour:
		return fmt.Sprintf("%dm ago", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(since.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}

func formatDurationMS(ms float64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%.0fms", ms)
	case ms < 60000:
		return fmt.Sprintf("%.1fs", ms/1000)
	default:
		return fmt.Sprintf("%.1fm", ms/60000)
	}
}

// ── runs tail ─────────────────────────────────────────────────────────────

type spanEvent struct {
	RunID      string  `json:"RunID"`
	SpanID     string  `json:"SpanID"`
	SpanName   string  `json:"SpanName"`
	AgentName  string  `json:"AgentName"`
	StartTime  string  `json:"StartTime"`
	DurationNS int64   `json:"DurationNS"`
	CostUSD    float64 `json:"CostUSD"`
	Status     string  `json:"StatusCode"`
}

func runRunsTail(args []string) {
	fs := flag.NewFlagSet("runs tail", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		project  = fs.String("project", "", "Project ID (required)")
		apiKey   = fs.String("api-key", "", "API key (or set AGENTPULSE_API_KEY)")
		endpoint = fs.String("endpoint", "", "AgentPulse API base URL (or set AGENTPULSE_ENDPOINT)")
		format   = fs.String("format", "pretty", "Output format: pretty or json")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			os.Exit(exitError)
		}
		os.Exit(exitError)
	}

	if *apiKey == "" {
		*apiKey = os.Getenv("AGENTPULSE_API_KEY")
	}
	if *endpoint == "" {
		if v := os.Getenv("AGENTPULSE_ENDPOINT"); v != "" {
			*endpoint = v
		} else {
			*endpoint = "https://api.agentpulse.io"
		}
	}

	var errs []string
	if *project == "" {
		errs = append(errs, "--project is required")
	}
	if *apiKey == "" {
		errs = append(errs, "--api-key or AGENTPULSE_API_KEY is required")
	}
	if *format != "pretty" && *format != "json" {
		errs = append(errs, "--format must be pretty or json")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "error:", e)
		}
		os.Exit(exitError)
	}

	if err := validateEndpoint(*endpoint); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}

	sseURL := fmt.Sprintf("%s/api/v1/projects/%s/live",
		strings.TrimRight(*endpoint, "/"), url.PathEscape(*project))

	// Set up signal handling for clean Ctrl+C exit.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *format == "pretty" {
		fmt.Fprintf(os.Stderr, "Tailing spans for project %s (Ctrl+C to stop)...\n\n", *project)
	}

	var spanCount int
	err := tailSSE(ctx, sseURL, *apiKey, func(eventType, data string) {
		if eventType != "span" {
			return
		}
		spanCount++
		if *format == "json" {
			fmt.Println(data)
			return
		}
		var sp spanEvent
		if err := json.Unmarshal([]byte(data), &sp); err != nil {
			return
		}
		ts := formatTime(sp.StartTime)
		dur := formatDurationNS(sp.DurationNS)
		runShort := sp.RunID
		if len(runShort) > 12 {
			runShort = runShort[:12]
		}
		fmt.Printf("[%s] %s > %s (%s) $%.5f\n",
			ts, runShort, sp.SpanName, dur, sp.CostUSD)
	})

	if *format == "pretty" {
		fmt.Fprintf(os.Stderr, "\nTailed %d span(s). Exiting.\n", spanCount)
	}

	if err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}
	os.Exit(exitPass)
}

func formatDurationNS(ns int64) string {
	ms := float64(ns) / 1e6
	return formatDurationMS(ms)
}

// tailSSE connects to an SSE endpoint and calls fn for each event until ctx is cancelled.
// Reconnects up to 3 times on transient failures with exponential backoff.
func tailSSE(ctx context.Context, sseURL, apiKey string, fn func(eventType, data string)) error {
	maxRetries := 3
	backoff := time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff = time.Duration(math.Min(float64(backoff*2), float64(30*time.Second)))
				fmt.Fprintf(os.Stderr, "warning: connection lost, reconnecting (attempt %d/%d)...\n", attempt, maxRetries)
			}
		}

		err := connectAndStream(ctx, sseURL, apiKey, fn)
		if err == nil || err == context.Canceled {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Continue to retry on transient errors.
	}
	return fmt.Errorf("failed to connect after %d attempts", maxRetries)
}

func connectAndStream(ctx context.Context, sseURL, apiKey string, fn func(eventType, data string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{} // no timeout — SSE is long-lived
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed (401) — check AGENTPULSE_API_KEY")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	// Parse SSE stream line by line.
	scanner := bufio.NewScanner(resp.Body)
	var currentEvent, currentData string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return context.Canceled
		}
		line := scanner.Text()

		if line == "" {
			// Blank line = end of event block.
			if currentData != "" {
				fn(currentEvent, currentData)
			}
			currentEvent = ""
			currentData = ""
			continue
		}

		if strings.HasPrefix(line, ":") {
			// SSE comment (keepalive) — ignore.
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			currentData = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("stream read error: %w", err)
	}
	return nil
}
