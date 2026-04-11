package llmclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CompletionRequest is the provider-agnostic request.
type CompletionRequest struct {
	Model       string
	System      string     // system prompt (sent natively per provider)
	Messages    []Message  // user/assistant messages
	Temperature *float32   // nil = provider default
	MaxTokens   int        // 0 = provider default
}

// Message represents a single chat message.
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// CompletionResponse is the provider-agnostic response.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
	FinishReason string
	LatencyMS    int64
}

// Client is the interface for making LLM completions.
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// ProviderKeys holds API keys for each supported provider.
type ProviderKeys struct {
	Anthropic string
	OpenAI    string
	Google    string
}

// client is the concrete implementation of Client.
type client struct {
	keys        ProviderKeys
	providerMap map[string]string
	httpClient  *http.Client

	// URL overrides for testing.
	anthropicURL string
	openAIURL    string
	googleURL    string
}

// New creates a Client that routes requests to the correct provider based on
// the providerMap (model ID -> provider name: "anthropic", "openai", "google").
func New(keys ProviderKeys, providerMap map[string]string) Client {
	return &client{
		keys:         keys,
		providerMap:  providerMap,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		anthropicURL: "https://api.anthropic.com/v1/messages",
		openAIURL:    "https://api.openai.com/v1/chat/completions",
		googleURL:    "https://generativelanguage.googleapis.com/v1beta/models",
	}
}

// Complete dispatches the request to the appropriate provider.
func (c *client) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	provider, err := c.resolveProvider(req.Model)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	var resp *CompletionResponse
	switch provider {
	case "anthropic":
		if c.keys.Anthropic == "" {
			return nil, fmt.Errorf("llmclient: anthropic API key is empty")
		}
		resp, err = c.callAnthropic(ctx, req)
	case "openai":
		if c.keys.OpenAI == "" {
			return nil, fmt.Errorf("llmclient: openai API key is empty")
		}
		resp, err = c.callOpenAI(ctx, req)
	case "google":
		if c.keys.Google == "" {
			return nil, fmt.Errorf("llmclient: google API key is empty")
		}
		resp, err = c.callGoogle(ctx, req)
	default:
		return nil, fmt.Errorf("llmclient: unknown provider %q for model %q", provider, req.Model)
	}

	if err != nil {
		return nil, err
	}

	resp.LatencyMS = time.Since(start).Milliseconds()
	return resp, nil
}

// resolveProvider determines the provider for a model ID. It first checks the
// explicit providerMap, then falls back to prefix-based inference.
func (c *client) resolveProvider(model string) (string, error) {
	if provider, ok := c.providerMap[model]; ok {
		return provider, nil
	}

	lower := strings.ToLower(model)
	switch {
	case strings.HasPrefix(lower, "claude"):
		return "anthropic", nil
	case strings.HasPrefix(lower, "gpt"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "o4"):
		return "openai", nil
	case strings.HasPrefix(lower, "gemini"):
		return "google", nil
	}

	return "", fmt.Errorf("llmclient: cannot resolve provider for model %q", model)
}
