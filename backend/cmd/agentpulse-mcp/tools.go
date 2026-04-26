package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// toolDef binds a tool name + JSON schema to a handler that takes opaque
// arguments and returns any JSON-marshalable result.
type toolDef struct {
	name        string
	description string
	inputSchema map[string]any
	call        func(ctx context.Context, args json.RawMessage) (any, error)
}

func registerTools(c *apiClient, cfg *Config) map[string]*toolDef {
	tools := map[string]*toolDef{}

	register := func(t *toolDef) { tools[t.name] = t }

	register(searchTracesTool(c, cfg))
	register(getRunDetailsTool(c, cfg))
	register(comparePromptsTool(c, cfg))
	register(replayRunTool(c, cfg))
	register(currentBudgetStatusTool(c, cfg))

	return tools
}

// stringSchema returns a JSON-schema object with the given field shape.
func stringSchema(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func intSchema(desc string, def, min, max int) map[string]any {
	out := map[string]any{
		"type":        "integer",
		"description": desc,
	}
	if def != 0 {
		out["default"] = def
	}
	if min != 0 {
		out["minimum"] = min
	}
	if max != 0 {
		out["maximum"] = max
	}
	return out
}

// resolveProject returns project_id from args, falling back to the configured
// default. Most callers never need to pass it explicitly.
func resolveProject(cfg *Config, raw json.RawMessage) (string, map[string]any, error) {
	var args map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", nil, fmt.Errorf("parse arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	pid := cfg.ProjectID
	if v, ok := args["project_id"].(string); ok && v != "" {
		pid = v
	}
	return pid, args, nil
}

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// ── search_traces ─────────────────────────────────────────────────────────────

func searchTracesTool(c *apiClient, cfg *Config) *toolDef {
	return &toolDef{
		name:        "search_traces",
		description: "Full-text search across spans for a project. Returns matching spans with run_id, span_id, agent_name, span_name, and a snippet.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q":          stringSchema("Search query (>= 3 chars)"),
				"project_id": stringSchema("Optional: override the configured project_id"),
				"span_kind":  stringSchema("Optional filter: llm.call | tool.call | mcp.tool_call | mcp.server | agent.handoff | memory.read | memory.write"),
				"limit":      intSchema("Max rows to return", 20, 1, 50),
			},
			"required": []string{"q"},
		},
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			projectID, args, err := resolveProject(cfg, raw)
			if err != nil {
				return nil, err
			}
			q := argString(args, "q")
			if len(q) < 3 {
				return nil, fmt.Errorf("query 'q' must be at least 3 characters")
			}
			limit := argInt(args, "limit")
			if limit <= 0 || limit > 50 {
				limit = 20
			}

			vals := url.Values{}
			vals.Set("q", q)
			vals.Set("limit", strconv.Itoa(limit))
			if sk := argString(args, "span_kind"); sk != "" {
				vals.Set("span_kind", sk)
			}

			var out json.RawMessage
			if err := c.get(ctx, "/api/v1/projects/"+projectID+"/search", vals, &out); err != nil {
				return nil, err
			}
			return json.RawMessage(out), nil
		},
	}
}

// ── get_run_details ───────────────────────────────────────────────────────────

func getRunDetailsTool(c *apiClient, cfg *Config) *toolDef {
	return &toolDef{
		name:        "get_run_details",
		description: "Fetch a single run's metadata (cost, duration, status, agent_name, span_count) plus its spans.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id":     stringSchema("Run ID to fetch"),
				"with_spans": map[string]any{"type": "boolean", "default": true, "description": "Include the span list"},
			},
			"required": []string{"run_id"},
		},
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			_, args, err := resolveProject(cfg, raw)
			if err != nil {
				return nil, err
			}
			runID := argString(args, "run_id")
			if runID == "" {
				return nil, fmt.Errorf("run_id is required")
			}
			withSpans := true
			if v, ok := args["with_spans"].(bool); ok {
				withSpans = v
			}

			var run json.RawMessage
			if err := c.get(ctx, "/api/v1/runs/"+runID, nil, &run); err != nil {
				return nil, err
			}
			if !withSpans {
				return map[string]any{"run": run}, nil
			}
			var spans json.RawMessage
			if err := c.get(ctx, "/api/v1/runs/"+runID+"/spans", nil, &spans); err != nil {
				return map[string]any{"run": run, "spans_error": err.Error()}, nil
			}
			return map[string]any{"run": run, "spans": spans}, nil
		},
	}
}

// ── compare_prompts ───────────────────────────────────────────────────────────

func comparePromptsTool(c *apiClient, cfg *Config) *toolDef {
	return &toolDef{
		name:        "compare_prompts",
		description: "Diff the LLM prompts and completions between two runs of the same agent flow. Returns per-span deltas.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id_a":   stringSchema("First run ID"),
				"run_id_b":   stringSchema("Second run ID"),
				"project_id": stringSchema("Optional: override the configured project_id"),
			},
			"required": []string{"run_id_a", "run_id_b"},
		},
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			projectID, args, err := resolveProject(cfg, raw)
			if err != nil {
				return nil, err
			}
			a := argString(args, "run_id_a")
			b := argString(args, "run_id_b")
			if a == "" || b == "" {
				return nil, fmt.Errorf("run_id_a and run_id_b are required")
			}
			vals := url.Values{}
			vals.Set("a", a)
			vals.Set("b", b)
			var out json.RawMessage
			if err := c.get(ctx, "/api/v1/projects/"+projectID+"/runs/compare/prompt-diff", vals, &out); err != nil {
				return nil, err
			}
			return json.RawMessage(out), nil
		},
	}
}

// ── replay_run ────────────────────────────────────────────────────────────────

func replayRunTool(c *apiClient, cfg *Config) *toolDef {
	return &toolDef{
		name:        "replay_run",
		description: "Return the replay bundle metadata for a run. Use the bundle URL with `apctl replay` to re-execute the run locally with overrides.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id": stringSchema("Run ID to fetch the replay bundle for"),
			},
			"required": []string{"run_id"},
		},
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			_, args, err := resolveProject(cfg, raw)
			if err != nil {
				return nil, err
			}
			runID := argString(args, "run_id")
			if runID == "" {
				return nil, fmt.Errorf("run_id is required")
			}
			var bundle json.RawMessage
			if err := c.get(ctx, "/api/v1/runs/"+runID+"/replay-bundle", nil, &bundle); err != nil {
				return nil, err
			}
			cli := fmt.Sprintf("apctl replay --base-url %s --token <ingest-token> %s", c.baseURL, runID)
			return map[string]any{
				"bundle":      bundle,
				"replay_cmd":  cli,
				"docs_link":   "https://github.com/agentpulse/agentpulse#replay",
				"description": "Pipe the bundle into apctl to re-run with overridden prompts/tools.",
			}, nil
		},
	}
}

// ── current_budget_status ─────────────────────────────────────────────────────

func currentBudgetStatusTool(c *apiClient, cfg *Config) *toolDef {
	return &toolDef{
		name:        "current_budget_status",
		description: "Show active budget rules for the project plus the most recent budget alerts.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_id": stringSchema("Optional: override the configured project_id"),
			},
		},
		call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			projectID, _, err := resolveProject(cfg, raw)
			if err != nil {
				return nil, err
			}
			var rules json.RawMessage
			if err := c.get(ctx, "/api/v1/projects/"+projectID+"/budget", nil, &rules); err != nil {
				return nil, err
			}
			var recent json.RawMessage
			_ = c.get(ctx, "/api/v1/projects/"+projectID+"/budget/alerts/recent", nil, &recent)
			return map[string]any{
				"project_id":    projectID,
				"rules":         rules,
				"recent_alerts": recent,
			}, nil
		},
	}
}
