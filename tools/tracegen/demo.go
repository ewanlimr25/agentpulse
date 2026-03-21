package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// demoProject describes a project to create and the runs to seed for it.
type demoProject struct {
	name      string
	scenarios []weightedScenario
	runs      int
}

// weightedScenario pairs a scenario with a display name and error probability.
type weightedScenario struct {
	name     string
	fn       scenario
	errorPct int // 0-100: probability this run ends with an error span
}

var demoProjects = []demoProject{
	{
		name: "customer-support-bot",
		runs: 12,
		scenarios: []weightedScenario{
			{"support-triage", scenarioSupportTriage, 5},
			{"support-escalation", scenarioSupportEscalation, 20},
		},
	},
	{
		name: "research-assistant",
		runs: 8,
		scenarios: []weightedScenario{
			{"deep-research", scenarioDeepResearch, 0},
			{"fact-check", scenarioFactCheck, 10},
		},
	},
	{
		name: "code-review-agent",
		runs: 10,
		scenarios: []weightedScenario{
			{"pr-review", scenarioPRReview, 15},
			{"security-scan", scenarioSecurityScan, 5},
		},
	},
	{
		name: "data-pipeline-monitor",
		runs: 15,
		scenarios: []weightedScenario{
			{"pipeline-health", scenarioPipelineHealth, 25},
		},
	},
}

// runDemo creates all demo projects via the backend API and seeds each with runs.
func runDemo(ctx context.Context, tracer trace.Tracer, apiBase, endpoint string, delay time.Duration) error {
	for _, proj := range demoProjects {
		projectID, err := createProject(apiBase, proj.name)
		if err != nil {
			return fmt.Errorf("create project %q: %w", proj.name, err)
		}
		log.Printf("created project %-30s  id=%s", proj.name, projectID)

		// Re-init tracer provider with the new project's service name so spans
		// carry a meaningful service.name resource attribute.
		tp, err := newTracerProvider(ctx, endpoint)
		if err != nil {
			return fmt.Errorf("tracer provider for %s: %w", proj.name, err)
		}

		for i := range proj.runs {
			ws := proj.scenarios[i%len(proj.scenarios)]
			log.Printf("  run %2d/%-2d  scenario=%-22s  project=%s", i+1, proj.runs, ws.name, proj.name)
			if err := ws.fn(ctx, tracer, projectID); err != nil {
				log.Printf("  scenario error: %v", err)
			}
			time.Sleep(delay)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("tracer shutdown: %v", err)
		}
		cancel()
	}
	return nil
}

// createProject POSTs to the backend API and returns the new project ID.
func createProject(apiBase, name string) (string, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(apiBase+"/api/v1/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", resp.StatusCode, raw)
	}

	var out struct {
		Data struct {
			Project struct {
				ID string `json:"ID"`
			} `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if out.Data.Project.ID == "" {
		return "", fmt.Errorf("empty project ID in response: %s", raw)
	}
	return out.Data.Project.ID, nil
}
