package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// withMCPTool returns attributes for an MCP tool call span.
func withMCPTool(serverName, toolName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span_kind", "mcp.tool_call"),
		attribute.String("agentpulse.mcp.server_name", serverName),
		attribute.String("agentpulse.mcp.tool_name", toolName),
		attribute.String("tool.name", toolName),
	}
}

// withMCPListTools returns attributes for an MCP list_tools span.
func withMCPListTools(serverName string, toolCount int, discoveredTools string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agentpulse.span_kind", "mcp.list_tools"),
		attribute.String("agentpulse.mcp.server_name", serverName),
		attribute.String("agentpulse.mcp.tool_count", fmt.Sprintf("%d", toolCount)),
		attribute.String("agentpulse.mcp.discovered_tools", discoveredTools),
	}
}

// ── Scenario: mcp-gateway ─────────────────────────────────────────────────────
//
// orchestrator-agent
//   ├─ mcp.list_tools:  filesystem-server  (discovers 4 tools)
//   ├─ mcp.list_tools:  github-server      (discovers 3 tools)
//   ├─ llm:             plan steps         (claude-haiku)
//   ├─ mcp.tool_call:   filesystem-server/read_file
//   ├─ mcp.tool_call:   filesystem-server/search_files
//   ├─ llm:             analyze findings   (claude-sonnet)
//   ├─ mcp.tool_call:   github-server/search_code
//   ├─ mcp.tool_call:   github-server/create_issue
//   └─ llm:             summarize results  (claude-haiku)

func scenarioMCPGateway(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "orchestrator-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			[]attribute.KeyValue{
				attribute.String("agentpulse.span_kind", "agent.handoff"),
				attribute.String("agentpulse.handoff.target", "mcp-tools"),
			},
		)...),
	)
	defer root.End()

	// Discover filesystem tools
	_, fsDiscover := tracer.Start(rootCtx, "mcp.list_tools",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPListTools("filesystem-server", 4, "read_file,write_file,list_directory,search_files"),
		)...),
	)
	time.Sleep(jitterD(40 * time.Millisecond))
	fsDiscover.End()

	// Discover GitHub tools
	_, ghDiscover := tracer.Start(rootCtx, "mcp.list_tools",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPListTools("github-server", 3, "create_issue,search_code,get_pr"),
		)...),
	)
	time.Sleep(jitterD(30 * time.Millisecond))
	ghDiscover.End()

	// LLM plans what to do with the discovered tools
	in, out := randTokens(200, 400, 100, 250)
	_, planSpan := tracer.Start(rootCtx, "llm.plan",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withLLM("claude-haiku-4-5", "openai", in, out),
		)...),
	)
	time.Sleep(jitterD(200 * time.Millisecond))
	planSpan.End()

	// Read a file via MCP
	_, readFile := tracer.Start(rootCtx, "mcp.read_file",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPTool("filesystem-server", "read_file"),
			[]attribute.KeyValue{
				attribute.String("tool.input", "/workspace/README.md"),
				attribute.String("tool.output", "# Project README\n\nThis project implements..."),
				attribute.String("agentpulse.mcp.input_schema", `{"path": {"type": "string"}}`),
			},
		)...),
	)
	time.Sleep(jitterD(50 * time.Millisecond))
	readFile.End()

	// Search files via MCP
	_, searchFiles := tracer.Start(rootCtx, "mcp.search_files",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPTool("filesystem-server", "search_files"),
			[]attribute.KeyValue{
				attribute.String("tool.input", `{"query": "TODO", "path": "/workspace/src"}`),
				attribute.String("tool.output", `["/workspace/src/main.go:42", "/workspace/src/handler.go:17"]`),
			},
		)...),
	)
	time.Sleep(jitterD(80 * time.Millisecond))
	searchFiles.End()

	// Analyze findings with LLM
	in, out = randTokens(400, 800, 200, 400)
	_, analyzeSpan := tracer.Start(rootCtx, "llm.analyze",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withLLM("claude-sonnet-4-6", "openai", in, out),
		)...),
	)
	time.Sleep(jitterD(500 * time.Millisecond))
	analyzeSpan.End()

	// Search GitHub code via MCP
	_, searchCode := tracer.Start(rootCtx, "mcp.search_code",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPTool("github-server", "search_code"),
			[]attribute.KeyValue{
				attribute.String("tool.input", `{"q": "TODO authentication", "repo": "myorg/myapp"}`),
				attribute.String("tool.output", `{"total_count": 3, "items": [...]}`),
			},
		)...),
	)
	time.Sleep(jitterD(120 * time.Millisecond))
	searchCode.End()

	// Create GitHub issue via MCP
	_, createIssue := tracer.Start(rootCtx, "mcp.create_issue",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPTool("github-server", "create_issue"),
			[]attribute.KeyValue{
				attribute.String("tool.input", `{"title": "Fix authentication TODOs", "body": "Found 3 unresolved TODOs..."}`),
				attribute.String("tool.output", `{"number": 142, "url": "https://github.com/myorg/myapp/issues/142"}`),
			},
		)...),
	)
	time.Sleep(jitterD(200 * time.Millisecond))
	createIssue.End()

	// Final summary LLM call
	in, out = randTokens(300, 600, 150, 300)
	_, summarySpan := tracer.Start(rootCtx, "llm.summarize",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withLLM("claude-haiku-4-5", "openai", in, out),
		)...),
	)
	time.Sleep(jitterD(300 * time.Millisecond))
	summarySpan.End()

	return nil
}

// scenarioMCPGatewayWithError is like scenarioMCPGateway but one MCP tool call fails.
// This demonstrates error handling and span status propagation.
func scenarioMCPGatewayWithError(ctx context.Context, tracer trace.Tracer, projectID, userID string) error {
	runID := fmt.Sprintf("run-%d", time.Now().UnixMilli())
	user := optUserAttrs(userID)

	rootCtx, root := tracer.Start(ctx, "orchestrator-agent",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			[]attribute.KeyValue{
				attribute.String("agentpulse.span_kind", "agent.handoff"),
				attribute.String("agentpulse.handoff.target", "mcp-tools"),
			},
		)...),
	)
	defer root.End()

	// Discover filesystem tools
	_, fsDiscover := tracer.Start(rootCtx, "mcp.list_tools",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPListTools("filesystem-server", 4, "read_file,write_file,list_directory,search_files"),
		)...),
	)
	time.Sleep(jitterD(40 * time.Millisecond))
	fsDiscover.End()

	// Plan
	in, out := randTokens(150, 300, 80, 180)
	_, planSpan := tracer.Start(rootCtx, "llm.plan",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withLLM("claude-haiku-4-5", "openai", in, out),
		)...),
	)
	time.Sleep(jitterD(150 * time.Millisecond))
	planSpan.End()

	// Write file via MCP — this one fails (e.g., permission denied)
	shouldFail := rand.Float64() < 0.6 // 60% chance of failure to ensure demo shows errors
	_, writeFile := tracer.Start(rootCtx, "mcp.write_file",
		trace.WithAttributes(combine(
			baseAttrs(projectID, runID, "orchestrator-agent"),
			user,
			withMCPTool("filesystem-server", "write_file"),
			[]attribute.KeyValue{
				attribute.String("tool.input", `{"path": "/workspace/output.md", "content": "..."}`),
			},
		)...),
	)
	time.Sleep(jitterD(60 * time.Millisecond))
	if shouldFail {
		writeFile.SetStatus(codes.Error, "MCP tool call failed: permission denied: /workspace/output.md")
		writeFile.End()
		root.SetStatus(codes.Error, "orchestrator failed: MCP write_file error")
		return fmt.Errorf("mcp write_file: permission denied")
	}
	writeFile.SetAttributes(attribute.String("tool.output", "file written successfully"))
	writeFile.End()

	return nil
}
