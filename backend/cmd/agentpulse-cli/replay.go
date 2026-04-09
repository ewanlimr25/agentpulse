package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// runReplay implements: agentpulse-cli replay <run-id> [--api-url] [--api-key] [--out path]
//
// It downloads a replay bundle from the AgentPulse API and writes it to a
// local JSON file that an SDK can load in replay mode.
func runReplay(args []string) {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		apiURL = fs.String("api-url", "", "AgentPulse API base URL (or set AGENTPULSE_API_URL, default http://localhost:8080)")
		apiKey = fs.String("api-key", "", "Project API key (or set AGENTPULSE_API_KEY)")
		out    = fs.String("out", "", "Output file path (default ./replay-<run-id>.json)")
	)

	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: agentpulse-cli replay <run-id> [--api-url URL] [--api-key KEY] [--out PATH]")
		os.Exit(2)
	}
	runID := fs.Arg(0)

	if *apiURL == "" {
		*apiURL = os.Getenv("AGENTPULSE_API_URL")
	}
	if *apiURL == "" {
		*apiURL = "http://localhost:8080"
	}
	if *apiKey == "" {
		*apiKey = os.Getenv("AGENTPULSE_API_KEY")
	}
	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: --api-key flag or AGENTPULSE_API_KEY env var is required")
		os.Exit(2)
	}

	outPath := *out
	if outPath == "" {
		outPath = fmt.Sprintf("./replay-%s.json", runID)
	}

	url := fmt.Sprintf("%s/api/v1/runs/%s/replay-bundle", *apiURL, runID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: build request: %v\n", err)
		os.Exit(2)
	}
	req.Header.Set("Authorization", "Bearer "+*apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: GET %s: %v\n", url, err)
		os.Exit(2)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "error: API returned %d: %s\n", resp.StatusCode, string(body))
		os.Exit(2)
	}

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: create %s: %v\n", outPath, err)
		os.Exit(2)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "error: write %s: %v\n", outPath, err)
		os.Exit(2)
	}

	fmt.Printf("Wrote replay bundle to %s\n\n", outPath)
	fmt.Println("Load it in your SDK:")
	fmt.Printf("  Python: from agentpulse import replay; bundle = replay.load_bundle(%q)\n", outPath)
	fmt.Printf("  TS:     import { loadBundle } from \"@agentpulse/sdk/replay\"; const bundle = loadBundle(%q);\n", outPath)
	fmt.Println("  Then run your agent in replay mode against the bundle.")
}
