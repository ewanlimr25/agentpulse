package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type projectHealth struct {
	CollectorReachable bool       `json:"CollectorReachable"`
	LastSpanAt         *time.Time `json:"LastSpanAt"`
	SpanCount          int64      `json:"SpanCount"`
	SpansPerMinute     int64      `json:"SpansPerMinute"`
}

type healthEnvelope struct {
	Data  *projectHealth `json:"data"`
	Error string         `json:"error"`
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		project   = fs.String("project", "", "Project ID (required)")
		apiKey    = fs.String("api-key", "", "API key (or set AGENTPULSE_API_KEY)")
		endpoint  = fs.String("endpoint", "", "AgentPulse API base URL (or set AGENTPULSE_ENDPOINT)")
		threshold = fs.Int("threshold", 300, "Seconds since last span before marking unhealthy")
		jsonOut   = fs.Bool("json", false, "Output JSON instead of human-readable text")
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

	reqURL := fmt.Sprintf("%s/api/v1/projects/%s/health",
		strings.TrimRight(*endpoint, "/"), url.PathEscape(*project))

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

	var env healthEnvelope
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

	health := env.Data

	// Apply threshold: override CollectorReachable if last span is older than threshold.
	healthy := health.CollectorReachable
	if health.LastSpanAt != nil && time.Since(*health.LastSpanAt) > time.Duration(*threshold)*time.Second {
		healthy = false
	}

	if *jsonOut {
		type jsonOutData struct {
			Healthy        bool       `json:"healthy"`
			CollectorAlive bool       `json:"collector_alive"`
			LastSpanAt     *time.Time `json:"last_span_at"`
			SpansPerMinute int64      `json:"spans_per_minute"`
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(jsonOutData{
			Healthy:        healthy,
			CollectorAlive: health.CollectorReachable,
			LastSpanAt:     health.LastSpanAt,
			SpansPerMinute: health.SpansPerMinute,
		})
		if healthy {
			os.Exit(exitPass)
		}
		os.Exit(exitFail)
	}

	// Human-readable output.
	indicator := "●"
	statusText := "healthy"
	if !healthy {
		indicator = "○"
		statusText = "unhealthy"
	}

	lastSpanDesc := "never"
	if health.LastSpanAt != nil {
		lastSpanDesc = "last span " + formatTime(health.LastSpanAt.Format(time.RFC3339Nano))
	}

	fmt.Printf("Collector:  %s %s (%s)\n", indicator, statusText, lastSpanDesc)
	fmt.Printf("Throughput: %d spans/min\n", health.SpansPerMinute)

	if healthy {
		os.Exit(exitPass)
	}
	os.Exit(exitFail)
}
