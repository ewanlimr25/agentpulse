package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// demoScenario is a scenario function that also accepts a userID for cost attribution.
type demoScenario func(ctx context.Context, tracer trace.Tracer, projectID, userID string) error

// demoProject describes a project to create and the runs to seed for it.
type demoProject struct {
	name      string
	scenarios []weightedScenario
	runs      int
	users     []string // round-robin user IDs assigned to each run; empty = no user attribution
}

// weightedScenario pairs a scenario with a display name and error probability.
type weightedScenario struct {
	name     string
	fn       demoScenario
	errorPct int // 0-100: probability this run ends with an error span
}

// budgetRule describes a cost threshold rule to create for a demo project.
type budgetRule struct {
	name         string
	thresholdUSD float64
	action       string // "notify" | "halt"
	scope        string // "run" | "agent"
}

// alertRule describes a signal-based alert rule to create for a demo project.
// Thresholds are tuned so seeded runs will trigger them.
type alertRule struct {
	name          string
	signalType    string  // "error_rate" | "latency_p95" | "quality_score" | "tool_failure"
	threshold     float64 // % for error/tool_failure, ms for latency, 0–1 for quality
	compareOp     string  // "gt" | "lt"
	windowSeconds int
	scopeFilter   string // tool name for tool_failure; empty otherwise
}

// sessionScenarioFn is a scenario that also accepts a sessionID and userID.
type sessionScenarioFn func(ctx context.Context, tracer trace.Tracer, projectID, sessionID, userID string) error

// sessionGroup defines a group of runs that share the same session_id,
// simulating a multi-turn conversation. userID is the customer attributed to the session.
type sessionGroup struct {
	sessionID string
	userID    string
	turns     []sessionScenarioFn
}

type demoProjectWithRules struct {
	demoProject
	budgetRule    budgetRule
	alertRules    []alertRule
	sessionGroups []sessionGroup
}

var demoProjects = []demoProjectWithRules{
	{
		demoProject: demoProject{
			name: "loop-detection-demo",
			runs: 6,
			scenarios: []weightedScenario{
				// 3 runs of the stuck-search scenario → Tier 1 high-confidence loops
				{"stuck-search-loop", scenarioStuckSearchLoop, 100},
				// 3 runs of the rapid-poll scenario → Tier 2 low-confidence loops
				{"rapid-poll-loop", scenarioRapidPollLoop, 100},
			},
		},
		budgetRule: budgetRule{"loop demo cap", 0.01, "notify", "run"},
		alertRules: []alertRule{
			// Fires as soon as 1 loop is detected in the 1-hour window
			{"Agent loop detected", "agent_loop", 0, "gt", 3600, ""},
		},
	},
	{
		demoProject: demoProject{
			name: "customer-support-bot",
			runs: 12,
			scenarios: []weightedScenario{
				{"support-triage", scenarioSupportTriage, 5},
				{"support-escalation", scenarioSupportEscalation, 20},
			},
			users: []string{"user-alice", "user-bob", "user-carol"},
		},
		budgetRule: budgetRule{"per-run cost cap", 0.0005, "notify", "run"},
		alertRules: []alertRule{
			// ~12% combined error rate across triage + escalation scenarios → fires at 8% threshold
			{"High error rate", "error_rate", 8.0, "gt", 3600, ""},
			// create_ticket errors 20% of the time in escalation → fires at 10% threshold
			{"Ticket service failures", "tool_failure", 10.0, "gt", 3600, "create_ticket"},
		},
		// Multi-turn sessions simulating customer conversations across multiple agent runs.
		sessionGroups: []sessionGroup{
			{
				sessionID: "session-billing-dispute-001",
				userID:    "user-alice",
				turns: []sessionScenarioFn{
					scenarioSupportTriageSession,     // turn 1: initial contact, classified as billing
					scenarioSupportTriageSession,     // turn 2: follow-up, still in triage
					scenarioSupportEscalationSession, // turn 3: escalated to human agent
				},
			},
			{
				sessionID: "session-tech-support-002",
				userID:    "user-bob",
				turns: []sessionScenarioFn{
					scenarioSupportTriageSession, // turn 1: initial report
					scenarioSupportTriageSession, // turn 2: kb search attempt
					scenarioSupportTriageSession, // turn 3: kb resolves issue
					scenarioSupportTriageSession, // turn 4: confirmation follow-up
				},
			},
			{
				sessionID: "session-shipping-003",
				userID:    "user-carol",
				turns: []sessionScenarioFn{
					scenarioSupportTriageSession,     // turn 1: package not arrived
					scenarioSupportEscalationSession, // turn 2: escalated, ticket created
					scenarioSupportTriageSession,     // turn 3: resolution confirmed
				},
			},
		},
	},
	{
		demoProject: demoProject{
			name: "research-assistant",
			runs: 8,
			scenarios: []weightedScenario{
				{"deep-research", scenarioDeepResearch, 0},
				{"fact-check", scenarioFactCheck, 10},
			},
			users: []string{"user-researcher-1", "user-researcher-2"},
		},
		budgetRule: budgetRule{"research budget", 0.005, "notify", "run"},
		alertRules: []alertRule{
			// deep-research runs take ~2500–3500ms total → fires at 2000ms threshold
			{"Slow research runs", "latency_p95", 2000.0, "gt", 3600, ""},
			// mock evals average ~0.81; threshold 0.90 → fires since 0.81 < 0.90
			{"Quality regression", "quality_score", 0.90, "lt", 3600, ""},
		},
	},
	{
		demoProject: demoProject{
			name: "code-review-agent",
			runs: 10,
			scenarios: []weightedScenario{
				{"pr-review", scenarioPRReview, 15},
				{"security-scan", scenarioSecurityScan, 5},
			},
			users: []string{"user-dev-1", "user-dev-2", "user-dev-3"},
		},
		budgetRule: budgetRule{"review cost cap", 0.002, "notify", "run"},
		alertRules: []alertRule{
			// ~9% combined error rate from pr-review 15% × half the runs → fires at 8%
			{"Review failure rate", "error_rate", 8.0, "gt", 3600, ""},
		},
	},
	{
		demoProject: demoProject{
			name: "data-pipeline-monitor",
			runs: 15,
			scenarios: []weightedScenario{
				{"pipeline-health", scenarioPipelineHealth, 25},
			},
			users: []string{"user-pipeline-1", "user-pipeline-2"},
		},
		budgetRule: budgetRule{"pipeline micro-cap", 0.0001, "halt", "run"},
		alertRules: []alertRule{
			// 25% error rate on pipeline-health → fires at 15% threshold
			{"Pipeline degraded", "error_rate", 15.0, "gt", 3600, ""},
		},
	},
}

// runDemo creates all demo projects via the backend API and seeds each with runs.
func runDemo(ctx context.Context, tracer trace.Tracer, apiBase, endpoint string, delay time.Duration) error {
	type projectKey struct{ name, id, key string }
	var created []projectKey

	for _, dp := range demoProjects {
		proj := dp.demoProject
		projectID, apiKey, err := createProject(apiBase, proj.name)
		if err != nil {
			return fmt.Errorf("create project %q: %w", proj.name, err)
		}
		created = append(created, projectKey{proj.name, projectID, apiKey})
		log.Printf("created project %-30s  id=%s  key=%s", proj.name, projectID, apiKey)

		if err := createBudgetRule(apiBase, projectID, apiKey, dp.budgetRule); err != nil {
			log.Printf("  warning: budget rule creation failed: %v", err)
		} else {
			log.Printf("  budget rule created: %s ($%.4f, %s)", dp.budgetRule.name, dp.budgetRule.thresholdUSD, dp.budgetRule.action)
		}

		for _, ar := range dp.alertRules {
			if err := createAlertRule(apiBase, projectID, apiKey, ar); err != nil {
				log.Printf("  warning: alert rule %q creation failed: %v", ar.name, err)
			} else {
				log.Printf("  alert rule created: %s (%s %s %.2f)", ar.name, ar.signalType, ar.compareOp, ar.threshold)
			}
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
			userID := ""
			if len(proj.users) > 0 {
				userID = proj.users[i%len(proj.users)]
			}
			log.Printf("  run %2d/%-2d  scenario=%-22s  project=%s  user=%s", i+1, proj.runs, ws.name, proj.name, userID)
			if err := ws.fn(ctx, tracer, projectID, userID); err != nil {
				log.Printf("  scenario error: %v", err)
			}
			time.Sleep(delay)
		}

		// Seed multi-turn session groups (if any configured for this project).
		for _, sg := range dp.sessionGroups {
			log.Printf("  seeding session %s (%d turns)  user=%s", sg.sessionID, len(sg.turns), sg.userID)
			for t, fn := range sg.turns {
				log.Printf("    turn %d/%d  session=%s", t+1, len(sg.turns), sg.sessionID)
				if err := fn(ctx, tracer, projectID, sg.sessionID, sg.userID); err != nil {
					log.Printf("    session turn error: %v", err)
				}
				time.Sleep(delay)
			}
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("tracer shutdown: %v", err)
		}
		cancel()
	}

	// Write a .seed-keys file so keys are recoverable after the run.
	var sb strings.Builder
	sb.WriteString("# Seed project API keys — generated by make seed\n")
	sb.WriteString("# Paste each key into the UI when prompted, or use as Bearer token.\n\n")
	for _, pk := range created {
		sb.WriteString(fmt.Sprintf("%-30s  id=%-38s  key=%s\n", pk.name, pk.id, pk.key))
	}
	if err := os.WriteFile(".seed-keys", []byte(sb.String()), 0600); err != nil {
		log.Printf("warning: could not write .seed-keys: %v", err)
	} else {
		log.Printf("API keys written to .seed-keys")
	}
	return nil
}

// authedPost sends a POST request with a JSON body and Bearer auth header.
func authedPost(url, apiKey string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return http.DefaultClient.Do(req)
}

// createBudgetRule POSTs a budget rule to the backend API for a project.
func createBudgetRule(apiBase, projectID, apiKey string, rule budgetRule) error {
	body, _ := json.Marshal(map[string]any{
		"name":          rule.name,
		"threshold_usd": rule.thresholdUSD,
		"action":        rule.action,
		"scope":         rule.scope,
		"enabled":       true,
	})
	url := fmt.Sprintf("%s/api/v1/projects/%s/budget/rules", apiBase, projectID)
	resp, err := authedPost(url, apiKey, body)
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

// createAlertRule POSTs a signal-based alert rule to the backend API for a project.
func createAlertRule(apiBase, projectID, apiKey string, rule alertRule) error {
	payload := map[string]any{
		"name":           rule.name,
		"signal_type":    rule.signalType,
		"threshold":      rule.threshold,
		"compare_op":     rule.compareOp,
		"window_seconds": rule.windowSeconds,
		"enabled":        true,
	}
	if rule.scopeFilter != "" {
		payload["scope_filter"] = rule.scopeFilter
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/v1/projects/%s/alerts/rules", apiBase, projectID)
	resp, err := authedPost(url, apiKey, body)
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

// createProject POSTs to the backend API and returns the new project ID and API key.
func createProject(apiBase, name string) (projectID, apiKey string, err error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := http.Post(apiBase+"/api/v1/projects", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API returned %d: %s", resp.StatusCode, raw)
	}

	var out struct {
		Data struct {
			Project struct {
				ID string `json:"ID"`
			} `json:"project"`
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("parse response: %w", err)
	}
	if out.Data.Project.ID == "" {
		return "", "", fmt.Errorf("empty project ID in response: %s", raw)
	}
	return out.Data.Project.ID, out.Data.APIKey, nil
}
