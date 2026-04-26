// Command agentpulse-mcp is a Model Context Protocol server that exposes
// AgentPulse trace and run data to IDE-side agents (Claude Code, Cursor,
// Windsurf, etc.) via JSON-RPC 2.0 over stdio.
//
// Usage:
//
//	agentpulse-mcp
//
// Configuration is via environment:
//
//	AGENTPULSE_BASE_URL     default: http://localhost:8080
//	AGENTPULSE_INGEST_TOKEN required — Bearer token from `apctl ingest-tokens create`
//	AGENTPULSE_PROJECT_ID   required — default project for all tools
//	AGENTPULSE_TIMEOUT      optional — HTTP timeout in seconds (default 15)
//
// Add to ~/.claude.json (Claude Code):
//
//	{
//	  "mcpServers": {
//	    "agentpulse": {
//	      "command": "agentpulse-mcp",
//	      "env": {
//	        "AGENTPULSE_BASE_URL": "http://localhost:8080",
//	        "AGENTPULSE_INGEST_TOKEN": "ap_live_xxx",
//	        "AGENTPULSE_PROJECT_ID": "00000000-0000-0000-0000-000000000000"
//	      }
//	    }
//	  }
//	}
package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentpulse-mcp:", err)
		os.Exit(2)
	}

	// Logs go to stderr; stdout is reserved for JSON-RPC frames.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := newAPIClient(cfg, logger)
	server := newServer(client, cfg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("agentpulse-mcp starting",
		"base_url", cfg.BaseURL,
		"project_id", cfg.ProjectID,
	)
	if err := server.Serve(ctx, bufio.NewReader(os.Stdin), os.Stdout); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// Config holds the runtime config for the MCP server.
type Config struct {
	BaseURL     string
	IngestToken string
	ProjectID   string
	Timeout     time.Duration
}

func loadConfig() (*Config, error) {
	baseURL := getEnv("AGENTPULSE_BASE_URL", "http://localhost:8080")
	token := os.Getenv("AGENTPULSE_INGEST_TOKEN")
	projectID := os.Getenv("AGENTPULSE_PROJECT_ID")
	if token == "" {
		return nil, fmt.Errorf("AGENTPULSE_INGEST_TOKEN is required")
	}
	if projectID == "" {
		return nil, fmt.Errorf("AGENTPULSE_PROJECT_ID is required")
	}

	timeout := 15 * time.Second
	if v := os.Getenv("AGENTPULSE_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}

	return &Config{
		BaseURL:     baseURL,
		IngestToken: token,
		ProjectID:   projectID,
		Timeout:     timeout,
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
