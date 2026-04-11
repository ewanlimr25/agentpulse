package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicVersion = "2023-06-01"

// ── Anthropic request/response types ─────────────────────────────────────────

type anthropicRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float32           `json:"temperature,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

// callAnthropic sends a completion request to the Anthropic Messages API.
func (c *client) callAnthropic(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	ar := anthropicRequest{
		Model:     req.Model,
		System:    req.System,
		MaxTokens: maxTokens,
		Messages:  msgs,
	}
	if req.Temperature != nil {
		ar.Temperature = req.Temperature
	}

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, fmt.Errorf("llmclient anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.anthropicURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.keys.Anthropic)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("llmclient anthropic: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llmclient anthropic: read body: %w", err)
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(raw, &aResp); err != nil || len(aResp.Content) == 0 {
		return nil, fmt.Errorf("llmclient anthropic: parse response: %w", err)
	}

	return &CompletionResponse{
		Text:         aResp.Content[0].Text,
		InputTokens:  aResp.Usage.InputTokens,
		OutputTokens: aResp.Usage.OutputTokens,
		FinishReason: aResp.StopReason,
	}, nil
}
