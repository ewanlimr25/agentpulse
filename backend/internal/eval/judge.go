package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicAPIURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	judgeModel       = "claude-haiku-4-5-20251001"
	evalVersion      = uint16(1)
)

// SupportedModels maps model ID -> provider name.
var SupportedModels = map[string]string{
	"claude-haiku-4-5": "anthropic",
	"gpt-4o-mini":      "openai",
	"gemini-2.0-flash": "google",
}

// ProviderKeys holds API keys for each supported judge provider.
type ProviderKeys struct {
	Anthropic string
	OpenAI    string
	Google    string
}

// stripMarkdownFences removes ```json ... ``` or ``` ... ``` wrappers if present.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```"} {
		if strings.HasPrefix(s, fence) {
			s = strings.TrimPrefix(s, fence)
			s = strings.TrimSuffix(s, "```")
			return strings.TrimSpace(s)
		}
	}
	return s
}

// JudgeResponse holds the score and reasoning returned by a judge model.
// It is exported so that callers outside this package (e.g. the dry-run handler)
// can read the result fields without reflection.
type JudgeResponse struct {
	Score     float32 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// judgeResponse is a package-internal alias kept for backward compatibility
// with existing unexported usages throughout this file.
type judgeResponse = JudgeResponse

// ── Anthropic ─────────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// callAnthropic sends the prompt to the Anthropic Messages API and returns a
// parsed judgeResponse. The prefill trick forces Claude to start with '{' so
// that the response is valid JSON without markdown fences.
func callAnthropic(ctx context.Context, apiKey, model, prompt string) (*judgeResponse, error) {
	return callAnthropicURL(ctx, apiKey, model, prompt, anthropicAPIURL)
}

// callAnthropicURL is the URL-overridable implementation used in tests.
func callAnthropicURL(ctx context.Context, apiKey, model, prompt, baseURL string) (*judgeResponse, error) {
	body, err := json.Marshal(anthropicRequest{
		Model:     model,
		MaxTokens: 256,
		// Prefill forces Claude to start with '{', preventing markdown fences.
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
			{Role: "assistant", Content: "{"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("judge anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("judge anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("judge anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain body to allow connection reuse, but do not log raw content.
		_, _ = io.Copy(io.Discard, resp.Body)
		slog.Warn("judge: api error", "provider", "anthropic", "status", resp.StatusCode)
		return nil, fmt.Errorf("judge anthropic: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("judge anthropic: read body: %w", err)
	}

	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil || len(ar.Content) == 0 {
		return nil, fmt.Errorf("judge anthropic: parse response: %w", err)
	}

	// Reconstruct the full JSON: the prefill '{' is not echoed back by the API,
	// so we prepend it. Also strip any accidental markdown fences defensively.
	text := "{" + ar.Content[0].Text
	text = stripMarkdownFences(text)

	return parseJudgeJSON(text, "anthropic")
}

// ── OpenAI ────────────────────────────────────────────────────────────────────

type openAIRequest struct {
	Model          string          `json:"model"`
	MaxTokens      int             `json:"max_tokens"`
	Messages       []openAIMessage `json:"messages"`
	ResponseFormat openAIFormat    `json:"response_format"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const openAIAPIURL = "https://api.openai.com/v1/chat/completions"

// callOpenAI sends the prompt to the OpenAI Chat Completions API with JSON mode
// enabled and returns a parsed judgeResponse.
func callOpenAI(ctx context.Context, apiKey, model, prompt string) (*judgeResponse, error) {
	return callOpenAIURL(ctx, apiKey, model, prompt, openAIAPIURL)
}

// callOpenAIURL is the URL-overridable implementation used in tests.
func callOpenAIURL(ctx context.Context, apiKey, model, prompt, baseURL string) (*judgeResponse, error) {
	body, err := json.Marshal(openAIRequest{
		Model:     model,
		MaxTokens: 256,
		Messages: []openAIMessage{
			{Role: "system", Content: "You are an eval judge. Respond with valid JSON only."},
			{Role: "user", Content: prompt},
		},
		ResponseFormat: openAIFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("judge openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("judge openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("judge openai: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		slog.Warn("judge: api error", "provider", "openai", "status", resp.StatusCode)
		return nil, fmt.Errorf("judge openai: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("judge openai: read body: %w", err)
	}

	var oResp openAIResponse
	if err := json.Unmarshal(raw, &oResp); err != nil || len(oResp.Choices) == 0 {
		return nil, fmt.Errorf("judge openai: parse response: %w", err)
	}

	text := stripMarkdownFences(oResp.Choices[0].Message.Content)
	return parseJudgeJSON(text, "openai")
}

// ── Google ────────────────────────────────────────────────────────────────────

type googleRequest struct {
	Contents         []googleContent        `json:"contents"`
	GenerationConfig googleGenerationConfig `json:"generationConfig"`
	SystemInstruction *googleContent        `json:"systemInstruction,omitempty"`
}

type googleContent struct {
	Role  string        `json:"role,omitempty"`
	Parts []googlePart  `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenerationConfig struct {
	MaxOutputTokens  int    `json:"maxOutputTokens"`
	ResponseMimeType string `json:"responseMimeType"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

const googleAPIBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// callGoogle sends the prompt to the Google Gemini generateContent API with
// JSON output mode enabled and returns a parsed judgeResponse.
func callGoogle(ctx context.Context, apiKey, model, prompt string) (*judgeResponse, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", googleAPIBaseURL, model, apiKey)
	return callGoogleURL(ctx, apiKey, model, prompt, url)
}

// callGoogleURL is the URL-overridable implementation used in tests.
func callGoogleURL(ctx context.Context, _ string, _ string, prompt, fullURL string) (*judgeResponse, error) {
	body, err := json.Marshal(googleRequest{
		SystemInstruction: &googleContent{
			Parts: []googlePart{{Text: "You are an eval judge. Respond with valid JSON only."}},
		},
		Contents: []googleContent{
			{Role: "user", Parts: []googlePart{{Text: prompt}}},
		},
		GenerationConfig: googleGenerationConfig{
			MaxOutputTokens:  256,
			ResponseMimeType: "application/json",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("judge google: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("judge google: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("judge google: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		slog.Warn("judge: api error", "provider", "google", "status", resp.StatusCode)
		return nil, fmt.Errorf("judge google: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("judge google: read body: %w", err)
	}

	var gr googleResponse
	if err := json.Unmarshal(raw, &gr); err != nil || len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("judge google: parse response: %w", err)
	}

	text := stripMarkdownFences(gr.Candidates[0].Content.Parts[0].Text)
	return parseJudgeJSON(text, "google")
}

// ── Router ────────────────────────────────────────────────────────────────────

// callJudgeModel is the package-internal entry point used by Worker and callJudge.
func callJudgeModel(ctx context.Context, keys ProviderKeys, model, prompt string) (*judgeResponse, error) {
	return CallJudgeModel(ctx, keys, model, prompt)
}

// CallJudgeModel routes to the correct provider based on the model ID.
// Returns an error if the model is not in SupportedModels or the required API key is empty.
// It is exported so that handlers (e.g. eval config dry-run) can invoke judges directly.
func CallJudgeModel(ctx context.Context, keys ProviderKeys, model, prompt string) (*judgeResponse, error) {
	provider, ok := SupportedModels[model]
	if !ok {
		return nil, fmt.Errorf("judge: unsupported model %q", model)
	}

	switch provider {
	case "anthropic":
		if keys.Anthropic == "" {
			return nil, fmt.Errorf("judge: anthropic API key is empty")
		}
		return callAnthropic(ctx, keys.Anthropic, model, prompt)
	case "openai":
		if keys.OpenAI == "" {
			return nil, fmt.Errorf("judge: openai API key is empty")
		}
		return callOpenAI(ctx, keys.OpenAI, model, prompt)
	case "google":
		if keys.Google == "" {
			return nil, fmt.Errorf("judge: google API key is empty")
		}
		return callGoogle(ctx, keys.Google, model, prompt)
	default:
		return nil, fmt.Errorf("judge: unknown provider %q for model %q", provider, model)
	}
}

// callJudge is the backward-compatible entry point used by Worker.
// It routes to the Anthropic provider using the hardcoded judge model.
func callJudge(ctx context.Context, apiKey, prompt string) (*judgeResponse, error) {
	return callJudgeModel(ctx, ProviderKeys{Anthropic: apiKey}, judgeModel, prompt)
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// parseJudgeJSON unmarshals a judge JSON string into a judgeResponse, clamping
// the score to [0, 1].
func parseJudgeJSON(text, provider string) (*judgeResponse, error) {
	var jr judgeResponse
	if err := json.Unmarshal([]byte(text), &jr); err != nil {
		return nil, fmt.Errorf("judge %s: parse score json: %w", provider, err)
	}
	if jr.Score < 0 {
		jr.Score = 0
	}
	if jr.Score > 1 {
		jr.Score = 1
	}
	return &jr, nil
}
