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

// budgetRule describes a rule to create for a demo project.
// ThresholdUSD is intentionally low so seeded runs will trigger it.
type budgetRule struct {
	name         string
	thresholdUSD float64
	action       string // "notify" | "halt"
	scope        string // "run" | "agent"
}

type demoProjectWithRule struct {
	demoProject
	rule budgetRule
}

var demoProjects = []demoProjectWithRule{
	{
		demoProject: demoProject{
			name: "customer-support-bot",
			runs: 12,
			scenarios: []weightedScenario{
				{"support-triage", scenarioSupportTriage, 5},
				{"support-escalation", scenarioSupportEscalation, 20},
			},
		},
		// haiku + sonnet mix ~$0.001–0.003/run; threshold fires on most runs
		rule: budgetRule{"per-run cost cap", 0.0005, "notify", "run"},
	},
	{
		demoProject: demoProject{
			name: "research-assistant",
			runs: 8,
			scenarios: []weightedScenario{
				{"deep-research", scenarioDeepResearch, 0},
				{"fact-check", scenarioFactCheck, 10},
			},
		},
		// gpt-4o ~$0.01–0.05/run; threshold fires on deep-research runs
		rule: budgetRule{"research budget", 0.005, "notify", "run"},
	},
	{
		demoProject: demoProject{
			name: "code-review-agent",
			runs: 10,
			scenarios: []weightedScenario{
				{"pr-review", scenarioPRReview, 15},
				{"security-scan", scenarioSecurityScan, 5},
			},
		},
		// sonnet ~$0.003–0.01/run; threshold fires on ~half the runs
		rule: budgetRule{"review cost cap", 0.002, "notify", "run"},
	},
	{
		demoProject: demoProject{
			name: "data-pipeline-monitor",
			runs: 15,
			scenarios: []weightedScenario{
				{"pipeline-health", scenarioPipelineHealth, 25},
			},
		},
		// gpt-4o-mini ~$0.0002–0.001/run; very low threshold fires frequently
		rule: budgetRule{"pipeline micro-cap", 0.0001, "halt", "run"},
	},
}

// runDemo creates all demo projects via the backend API and seeds each with runs.
func runDemo(ctx context.Context, tracer trace.Tracer, apiBase, endpoint string, delay time.Duration) error {
	for _, dp := range demoProjects {
		proj := dp.demoProject
		projectID, err := createProject(apiBase, proj.name)
		if err != nil {
			return fmt.Errorf("create project %q: %w", proj.name, err)
		}
		log.Printf("created project %-30s  id=%s", proj.name, projectID)

		if err := createBudgetRule(apiBase, projectID, dp.rule); err != nil {
			log.Printf("  warning: budget rule creation failed: %v", err)
		} else {
			log.Printf("  budget rule created: %s ($%.4f, %s)", dp.rule.name, dp.rule.thresholdUSD, dp.rule.action)
		}

		// Wait for the budget processor to pick up the new rule.
		// The collector refreshes rules every 3s (dev config); 5s is a safe margin.
		log.Printf("  waiting 5s for budget processor to load rule...")
		time.Sleep(5 * time.Second)

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

// createBudgetRule POSTs a budget rule to the backend API for a project.
func createBudgetRule(apiBase, projectID string, rule budgetRule) error {
	body, _ := json.Marshal(map[string]any{
		"name":          rule.name,
		"threshold_usd": rule.thresholdUSD,
		"action":        rule.action,
		"scope":         rule.scope,
		"enabled":       true,
	})
	url := fmt.Sprintf("%s/api/v1/projects/%s/budget/rules", apiBase, projectID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, raw)
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
