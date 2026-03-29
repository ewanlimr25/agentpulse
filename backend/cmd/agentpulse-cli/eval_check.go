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

// evalTypeBaseline mirrors domain.EvalTypeBaseline for JSON decoding.
type evalTypeBaseline struct {
	EvalName  string  `json:"eval_name"`
	AvgScore  float32 `json:"avg_score"`
	SpanCount int     `json:"span_count"`
	RunCount  int     `json:"run_count"`
}

// evalBaseline mirrors domain.EvalBaseline for JSON decoding.
type evalBaseline struct {
	ProjectID      string             `json:"project_id"`
	RunsConsidered int                `json:"runs_considered"`
	Types          []evalTypeBaseline `json:"types"`
	OverallScore   float32            `json:"overall_score"`
}

// apiEnvelope is the standard AgentPulse API response wrapper.
type apiEnvelope struct {
	Data  *evalBaseline `json:"data"`
	Error string        `json:"error"`
}

// jsonOutput is the --json output structure.
type jsonOutput struct {
	Pass      bool             `json:"pass"`
	ExitCode  int              `json:"exit_code"`
	Threshold float64          `json:"threshold"`
	EvalType  string           `json:"eval_type,omitempty"`
	Baseline  *evalBaseline    `json:"baseline"`
	Message   string           `json:"message"`
}

// Exit codes:
//
//	0 — scores meet threshold (pass)
//	1 — scores below threshold (fail)
//	2 — usage error, API error, or insufficient data
const (
	exitPass        = 0
	exitFail        = 1
	exitError       = 2
)

func runEvalCheck(args []string) {
	fs := flag.NewFlagSet("eval check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		project   = fs.String("project", "", "Project ID (required)")
		apiKey    = fs.String("api-key", "", "API key (or set AGENTPULSE_API_KEY)")
		threshold = fs.Float64("threshold", 0.0, "Minimum passing score 0.0–1.0 (required)")
		evalType  = fs.String("eval-type", "", "Check a specific eval type (e.g. relevance, hallucination)")
		runs      = fs.Int("runs", 10, "Number of recent runs to average (1–100)")
		endpoint  = fs.String("endpoint", "", "AgentPulse API base URL (or set AGENTPULSE_ENDPOINT, default https://api.agentpulse.io)")
		minRuns   = fs.Int("min-runs", 1, "Minimum runs required to gate; exits 2 if fewer runs have data")
		failOpen  = fs.Bool("fail-open", false, "Exit 0 (pass) when the API is unreachable instead of failing")
		jsonOut   = fs.Bool("json", false, "Output JSON instead of human-readable text")
	)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			os.Exit(exitError)
		}
		os.Exit(exitError)
	}

	// Resolve from env vars when flags not set.
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

	// Validate required flags.
	var errs []string
	if *project == "" {
		errs = append(errs, "--project is required")
	}
	if *apiKey == "" {
		errs = append(errs, "--api-key or AGENTPULSE_API_KEY is required")
	}
	if *threshold == 0.0 {
		errs = append(errs, "--threshold is required")
	}
	if *threshold < 0.0 || *threshold > 1.0 {
		errs = append(errs, "--threshold must be between 0.0 and 1.0")
	}
	if *runs < 1 || *runs > 100 {
		errs = append(errs, "--runs must be between 1 and 100")
	}
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "error:", e)
		}
		fmt.Fprintln(os.Stderr, "\nRun 'agentpulse-cli eval check --help' for usage.")
		os.Exit(exitError)
	}

	// Validate endpoint URL — must be http or https to prevent SSRF.
	if err := validateEndpoint(*endpoint); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}

	// Build request URL.
	reqURL := fmt.Sprintf("%s/api/v1/projects/%s/evals/baseline?runs=%d",
		strings.TrimRight(*endpoint, "/"), *project, *runs)
	if *evalType != "" {
		reqURL += "&eval_type=" + url.QueryEscape(*evalType)
	}

	// Call the API.
	baseline, apiErr := fetchBaseline(reqURL, *apiKey)
	if apiErr != nil {
		if *failOpen {
			msg := fmt.Sprintf("WARNING: AgentPulse API unreachable (%v) — passing due to --fail-open", apiErr)
			if *jsonOut {
				printJSON(&jsonOutput{Pass: true, ExitCode: exitPass, Threshold: *threshold, Message: msg, EvalType: *evalType})
			} else {
				fmt.Fprintln(os.Stderr, msg)
			}
			os.Exit(exitPass)
		}
		if *jsonOut {
			printJSON(&jsonOutput{Pass: false, ExitCode: exitError, Threshold: *threshold, Message: apiErr.Error(), EvalType: *evalType})
		} else {
			fmt.Fprintln(os.Stderr, "error:", apiErr)
		}
		os.Exit(exitError)
	}

	// Insufficient data check.
	if baseline.RunsConsidered < *minRuns {
		msg := fmt.Sprintf("insufficient data: only %d run(s) found, --min-runs requires %d", baseline.RunsConsidered, *minRuns)
		if *jsonOut {
			printJSON(&jsonOutput{Pass: false, ExitCode: exitError, Threshold: *threshold, Baseline: baseline, Message: msg, EvalType: *evalType})
		} else {
			fmt.Fprintln(os.Stderr, "SKIP:", msg)
		}
		os.Exit(exitError)
	}

	// Determine score to gate on.
	pass, score, message := evaluate(baseline, *threshold, *evalType)

	if *jsonOut {
		code := exitPass
		if !pass {
			code = exitFail
		}
		printJSON(&jsonOutput{
			Pass:      pass,
			ExitCode:  code,
			Threshold: *threshold,
			EvalType:  *evalType,
			Baseline:  baseline,
			Message:   message,
		})
	} else {
		printHuman(baseline, *threshold, *evalType, pass, score, message)
	}

	if pass {
		os.Exit(exitPass)
	}
	os.Exit(exitFail)
}

// evaluate determines pass/fail and returns the gated score and a summary message.
func evaluate(b *evalBaseline, threshold float64, evalType string) (pass bool, score float32, message string) {
	if evalType != "" {
		for _, t := range b.Types {
			if t.EvalName == evalType {
				pass = t.AvgScore >= float32(threshold)
				score = t.AvgScore
				if t.RunCount < b.RunsConsidered {
					message = fmt.Sprintf("%s score %.3f (based on %d/%d runs, %d spans)",
						t.EvalName, t.AvgScore, t.RunCount, b.RunsConsidered, t.SpanCount)
				} else {
					message = fmt.Sprintf("%s score %.3f (%d runs, %d spans)",
						t.EvalName, t.AvgScore, t.RunCount, t.SpanCount)
				}
				return
			}
		}
		// Not found — API should have returned 400, but handle defensively.
		return false, 0, fmt.Sprintf("eval type %q not found in baseline response", evalType)
	}

	score = b.OverallScore
	pass = score >= float32(threshold)
	message = fmt.Sprintf("overall score %.3f across %d eval type(s), %d run(s)",
		score, len(b.Types), b.RunsConsidered)
	return
}

func printHuman(b *evalBaseline, threshold float64, evalType string, pass bool, score float32, message string) {
	verdict := "PASS ✓"
	if !pass {
		verdict = "FAIL ✗"
	}

	fmt.Printf("AgentPulse Eval Check — project: %s\n", b.ProjectID)
	fmt.Printf("Threshold: %.3f  |  Runs considered: %d\n\n", threshold, b.RunsConsidered)

	if len(b.Types) > 0 {
		fmt.Printf("%-24s  %8s  %8s  %8s\n", "eval type", "score", "runs", "spans")
		fmt.Println(strings.Repeat("-", 56))
		for _, t := range b.Types {
			coverage := ""
			if t.RunCount < b.RunsConsidered {
				coverage = fmt.Sprintf(" (%d/%d runs)", t.RunCount, b.RunsConsidered)
			}
			indicator := "  "
			if evalType == "" || evalType == t.EvalName {
				if float64(t.AvgScore) < threshold {
					indicator = "✗ "
				} else {
					indicator = "✓ "
				}
			}
			fmt.Printf("%s%-22s  %8.3f  %8d  %8d%s\n",
				indicator, t.EvalName, t.AvgScore, t.RunCount, t.SpanCount, coverage)
		}
		fmt.Println()
	}

	fmt.Printf("Result: %s  —  %s\n", verdict, message)
}

func printJSON(out *jsonOutput) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// fetchBaseline calls the baseline endpoint and returns the decoded result.
// It never includes the API key in error output.
func fetchBaseline(reqURL, apiKey string) (*evalBaseline, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer [REDACTED]") // placeholder; replaced below
	req.Header.Del("Authorization")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401) — check that AGENTPULSE_API_KEY belongs to project %q", extractProject(reqURL))
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("access denied (403) — API key does not belong to this project")
	}
	if resp.StatusCode == http.StatusBadRequest {
		var env apiEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		if env.Error != "" {
			return nil, fmt.Errorf("bad request (400): %s", env.Error)
		}
		return nil, fmt.Errorf("bad request (400)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from API", resp.StatusCode)
	}

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}
	if env.Data == nil {
		return nil, fmt.Errorf("API returned empty data")
	}
	return env.Data, nil
}

// validateEndpoint ensures the endpoint URL uses http or https only.
func validateEndpoint(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint must use http:// or https:// (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint URL missing host")
	}
	if u.User != nil {
		return fmt.Errorf("endpoint URL must not contain credentials in the host")
	}
	return nil
}

// extractProject pulls the project ID from a baseline URL for error messages.
func extractProject(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	parts := strings.Split(u.Path, "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}
