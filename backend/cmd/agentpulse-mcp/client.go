package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// apiClient is a thin wrapper around the AgentPulse REST API. We keep it
// JSON-only and depend on no AgentPulse internals so this binary stays small
// (a single command-line user installs it, points it at any AgentPulse
// deployment, and gets an MCP server).
type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
	logger  *slog.Logger
}

func newAPIClient(cfg *Config, logger *slog.Logger) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.IngestToken,
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// get performs an authenticated GET against the AgentPulse API and decodes the
// JSON response into out. Empty body is allowed.
func (c *apiClient) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MB cap

	if resp.StatusCode >= 400 {
		return fmt.Errorf("api error %d: %s", resp.StatusCode, snippet(body))
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// post is provided in case future tools need to mutate; reserved.
func (c *apiClient) post(ctx context.Context, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("api error %d: %s", resp.StatusCode, snippet(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func snippet(b []byte) string {
	const max = 240
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
