package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, baseURL string) *Server {
	t.Helper()
	cfg := &Config{
		BaseURL:     baseURL,
		IngestToken: "test-token",
		ProjectID:   "00000000-0000-0000-0000-000000000000",
		Timeout:     2 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := newAPIClient(cfg, logger)
	return newServer(client, cfg, logger)
}

// runFrames pipes the given JSON-RPC requests through Serve and returns the
// JSON-RPC responses (one per non-notification request, in order).
func runFrames(t *testing.T, srv *Server, reqs []string) []rpcResponse {
	t.Helper()
	in := strings.NewReader(strings.Join(reqs, "\n") + "\n")
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Serve(ctx, bufio.NewReader(in), &out)

	var responses []rpcResponse
	dec := json.NewDecoder(&out)
	for {
		var r rpcResponse
		if err := dec.Decode(&r); err != nil {
			break
		}
		responses = append(responses, r)
	}
	return responses
}

func TestInitialize(t *testing.T) {
	srv := newTestServer(t, "http://example.invalid")
	resps := runFrames(t, srv, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
	})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %+v", resps[0].Error)
	}
	body, _ := json.Marshal(resps[0].Result)
	if !strings.Contains(string(body), `"protocolVersion":"2024-11-05"`) {
		t.Errorf("missing protocolVersion: %s", body)
	}
	if !strings.Contains(string(body), `"name":"agentpulse-mcp"`) {
		t.Errorf("missing serverInfo.name: %s", body)
	}
}

func TestToolsList(t *testing.T) {
	srv := newTestServer(t, "http://example.invalid")
	resps := runFrames(t, srv, []string{
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	body, _ := json.Marshal(resps[0].Result)
	want := []string{
		"search_traces", "get_run_details", "compare_prompts", "replay_run", "current_budget_status",
	}
	for _, name := range want {
		if !strings.Contains(string(body), `"name":"`+name+`"`) {
			t.Errorf("missing tool %s in response: %s", name, body)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := newTestServer(t, "http://example.invalid")
	resps := runFrames(t, srv, []string{
		`{"jsonrpc":"2.0","id":3,"method":"does_not_exist"}`,
	})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil || resps[0].Error.Code != codeMethodNotFound {
		t.Errorf("expected MethodNotFound, got: %+v", resps[0].Error)
	}
}

func TestToolsCall_SearchTraces_HitsAPI(t *testing.T) {
	var capturedAuth, capturedPath, capturedQuery string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[],"total":0,"limit":20,"offset":0,"query":"hello"}`))
	}))
	defer api.Close()

	srv := newTestServer(t, api.URL)
	req := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_traces","arguments":{"q":"hello"}}}`
	resps := runFrames(t, srv, []string{req})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %+v", resps[0].Error)
	}
	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %q", capturedAuth)
	}
	if !strings.HasSuffix(capturedPath, "/search") {
		t.Errorf("expected /search path, got %q", capturedPath)
	}
	if !strings.Contains(capturedQuery, "q=hello") {
		t.Errorf("expected q=hello in query, got %q", capturedQuery)
	}
}

func TestToolsCall_SearchTraces_ShortQuery(t *testing.T) {
	srv := newTestServer(t, "http://example.invalid")
	req := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_traces","arguments":{"q":"hi"}}}`
	resps := runFrames(t, srv, []string{req})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected RPC error: %+v", resps[0].Error)
	}
	body, _ := json.Marshal(resps[0].Result)
	if !strings.Contains(string(body), "at least 3 characters") {
		t.Errorf("expected validation error in result, got %s", body)
	}
}

func TestToolsCall_ReplayRun_IncludesCommand(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"run_id":"abc","spans":[]}`))
	}))
	defer api.Close()

	srv := newTestServer(t, api.URL)
	req := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"replay_run","arguments":{"run_id":"abc"}}}`
	resps := runFrames(t, srv, []string{req})
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	body, _ := json.Marshal(resps[0].Result)
	if !strings.Contains(string(body), "apctl replay") {
		t.Errorf("expected replay_cmd hint, got %s", body)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	srv := newTestServer(t, "http://example.invalid")
	// id is omitted -> notification.
	req := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resps := runFrames(t, srv, []string{req})
	if len(resps) != 0 {
		t.Errorf("expected 0 responses for notification, got %d", len(resps))
	}
}
