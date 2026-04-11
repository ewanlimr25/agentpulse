package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── Mock handlers ────────────────────────────────────────────────────────────

func anthropicHandler(text string, inputTok, outputTok int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
			"usage": map[string]int{
				"input_tokens":  inputTok,
				"output_tokens": outputTok,
			},
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func openAIHandler(text string, promptTok, completionTok int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]string{"content": text},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     promptTok,
				"completion_tokens": completionTok,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func googleHandler(text string, promptTok, candidateTok int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]string{{"text": text}},
					},
				},
			},
			"usageMetadata": map[string]int{
				"promptTokenCount":     promptTok,
				"candidatesTokenCount": candidateTok,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func errorHandler(statusCode int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	})
}

// newTestClient creates a client with URL overrides pointing to test servers.
func newTestClient(keys ProviderKeys, providerMap map[string]string, anthropicSrv, openAISrv, googleSrv *httptest.Server) *client {
	c := &client{
		keys:        keys,
		providerMap: providerMap,
		httpClient:  http.DefaultClient,
	}
	if anthropicSrv != nil {
		c.anthropicURL = anthropicSrv.URL
	}
	if openAISrv != nil {
		c.openAIURL = openAISrv.URL
	}
	if googleSrv != nil {
		c.googleURL = googleSrv.URL
	}
	return c
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestAnthropicCompletion(t *testing.T) {
	srv := httptest.NewServer(anthropicHandler("Hello from Claude!", 42, 15))
	defer srv.Close()

	temp := float32(0.7)
	c := newTestClient(
		ProviderKeys{Anthropic: "test-key"},
		map[string]string{"claude-haiku-4-5": "anthropic"},
		srv, nil, nil,
	)

	resp, err := c.Complete(context.Background(), CompletionRequest{
		Model:       "claude-haiku-4-5",
		System:      "You are a helpful assistant.",
		Messages:    []Message{{Role: "user", Content: "Hi"}},
		Temperature: &temp,
		MaxTokens:   1024,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Hello from Claude!" {
		t.Errorf("text: got %q, want %q", resp.Text, "Hello from Claude!")
	}
	if resp.InputTokens != 42 {
		t.Errorf("input tokens: got %d, want 42", resp.InputTokens)
	}
	if resp.OutputTokens != 15 {
		t.Errorf("output tokens: got %d, want 15", resp.OutputTokens)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("finish reason: got %q, want %q", resp.FinishReason, "end_turn")
	}
	if resp.LatencyMS < 0 {
		t.Errorf("latency should be non-negative, got %d", resp.LatencyMS)
	}
}

func TestAnthropicRequestShape(t *testing.T) {
	var gotBody anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		// Return a valid response.
		resp := map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 1},
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	temp := float32(0.5)
	c := newTestClient(
		ProviderKeys{Anthropic: "key"},
		map[string]string{"claude-sonnet-4-20250514": "anthropic"},
		srv, nil, nil,
	)

	_, err := c.Complete(context.Background(), CompletionRequest{
		Model:       "claude-sonnet-4-20250514",
		System:      "Be helpful",
		Messages:    []Message{{Role: "user", Content: "Hello"}},
		Temperature: &temp,
		MaxTokens:   512,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody.System != "Be helpful" {
		t.Errorf("system: got %q, want %q", gotBody.System, "Be helpful")
	}
	if gotBody.MaxTokens != 512 {
		t.Errorf("max_tokens: got %d, want 512", gotBody.MaxTokens)
	}
	if gotBody.Temperature == nil || *gotBody.Temperature != 0.5 {
		t.Errorf("temperature: got %v, want 0.5", gotBody.Temperature)
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Errorf("messages: expected 1 user message, got %+v", gotBody.Messages)
	}
}

func TestOpenAICompletion(t *testing.T) {
	srv := httptest.NewServer(openAIHandler("Hello from GPT!", 30, 10))
	defer srv.Close()

	c := newTestClient(
		ProviderKeys{OpenAI: "test-key"},
		map[string]string{"gpt-4o": "openai"},
		nil, srv, nil,
	)

	resp, err := c.Complete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		System:   "You are a helpful assistant.",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Hello from GPT!" {
		t.Errorf("text: got %q, want %q", resp.Text, "Hello from GPT!")
	}
	if resp.InputTokens != 30 {
		t.Errorf("input tokens: got %d, want 30", resp.InputTokens)
	}
	if resp.OutputTokens != 10 {
		t.Errorf("output tokens: got %d, want 10", resp.OutputTokens)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish reason: got %q, want %q", resp.FinishReason, "stop")
	}
}

func TestOpenAIRequestShape(t *testing.T) {
	var gotBody openAIRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(
		ProviderKeys{OpenAI: "key"},
		map[string]string{"gpt-4o": "openai"},
		nil, srv, nil,
	)

	_, err := c.Complete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		System:   "Be concise",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// System prompt should be sent as the first message with role "system".
	if len(gotBody.Messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(gotBody.Messages))
	}
	if gotBody.Messages[0].Role != "system" || gotBody.Messages[0].Content != "Be concise" {
		t.Errorf("first message: got %+v, want system message", gotBody.Messages[0])
	}
	if gotBody.Messages[1].Role != "user" || gotBody.Messages[1].Content != "Hello" {
		t.Errorf("second message: got %+v, want user message", gotBody.Messages[1])
	}
}

func TestGoogleCompletion(t *testing.T) {
	srv := httptest.NewServer(googleHandler("Hello from Gemini!", 25, 12))
	defer srv.Close()

	c := newTestClient(
		ProviderKeys{Google: "test-key"},
		map[string]string{"gemini-2.0-flash": "google"},
		nil, nil, srv,
	)

	resp, err := c.Complete(context.Background(), CompletionRequest{
		Model:    "gemini-2.0-flash",
		System:   "You are a helpful assistant.",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Hello from Gemini!" {
		t.Errorf("text: got %q, want %q", resp.Text, "Hello from Gemini!")
	}
	if resp.InputTokens != 25 {
		t.Errorf("input tokens: got %d, want 25", resp.InputTokens)
	}
	if resp.OutputTokens != 12 {
		t.Errorf("output tokens: got %d, want 12", resp.OutputTokens)
	}
}

func TestUnknownModelError(t *testing.T) {
	c := newTestClient(
		ProviderKeys{Anthropic: "key", OpenAI: "key", Google: "key"},
		map[string]string{},
		nil, nil, nil,
	)

	_, err := c.Complete(context.Background(), CompletionRequest{
		Model:    "unknown-model-xyz",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown model, got nil")
	}
}

func TestProviderKeyMissingError(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"anthropic key missing", "claude-haiku-4-5"},
		{"openai key missing", "gpt-4o"},
		{"google key missing", "gemini-2.0-flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// All keys empty.
			c := newTestClient(
				ProviderKeys{},
				map[string]string{
					"claude-haiku-4-5": "anthropic",
					"gpt-4o":           "openai",
					"gemini-2.0-flash": "google",
				},
				nil, nil, nil,
			)

			_, err := c.Complete(context.Background(), CompletionRequest{
				Model:    tt.model,
				Messages: []Message{{Role: "user", Content: "Hi"}},
			})
			if err == nil {
				t.Fatalf("expected error for missing %s key, got nil", tt.name)
			}
		})
	}
}

func TestAPIErrorReturnsError(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		provider string
		status   int
	}{
		{"anthropic 401", "claude-haiku-4-5", "anthropic", http.StatusUnauthorized},
		{"openai 429", "gpt-4o", "openai", http.StatusTooManyRequests},
		{"google 403", "gemini-2.0-flash", "google", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(errorHandler(tt.status))
			defer srv.Close()

			keys := ProviderKeys{}
			providerMap := map[string]string{tt.model: tt.provider}
			var anthropicSrv, openAISrv, googleSrv *httptest.Server

			switch tt.provider {
			case "anthropic":
				keys.Anthropic = "key"
				anthropicSrv = srv
			case "openai":
				keys.OpenAI = "key"
				openAISrv = srv
			case "google":
				keys.Google = "key"
				googleSrv = srv
			}

			c := newTestClient(keys, providerMap, anthropicSrv, openAISrv, googleSrv)

			_, err := c.Complete(context.Background(), CompletionRequest{
				Model:    tt.model,
				Messages: []Message{{Role: "user", Content: "Hi"}},
			})
			if err == nil {
				t.Fatalf("expected error on %d status, got nil", tt.status)
			}
		})
	}
}

func TestPrefixInference(t *testing.T) {
	tests := []struct {
		model    string
		wantProv string
	}{
		{"claude-sonnet-4-20250514", "anthropic"},
		{"gpt-4-turbo", "openai"},
		{"o3-mini", "openai"},
		{"o4-mini", "openai"},
		{"gemini-1.5-pro", "google"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			c := &client{providerMap: map[string]string{}}
			got, err := c.resolveProvider(tt.model)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantProv {
				t.Errorf("provider: got %q, want %q", got, tt.wantProv)
			}
		})
	}
}
