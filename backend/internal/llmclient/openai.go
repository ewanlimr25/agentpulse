package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ── OpenAI request/response types ────────────────────────────────────────────

type openAIRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	Messages    []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// callOpenAI sends a completion request to the OpenAI Chat Completions API.
func (c *client) callOpenAI(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	msgs := make([]openAIMessage, 0, len(req.Messages)+1)

	// System prompt sent as a system message.
	if req.System != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, openAIMessage{Role: m.Role, Content: m.Content})
	}

	or := openAIRequest{
		Model:    req.Model,
		Messages: msgs,
	}
	if req.MaxTokens > 0 {
		or.MaxTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		or.Temperature = req.Temperature
	}

	body, err := json.Marshal(or)
	if err != nil {
		return nil, fmt.Errorf("llmclient openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.openAIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.keys.OpenAI)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient openai: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("llmclient openai: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llmclient openai: read body: %w", err)
	}

	var oResp openAIResponse
	if err := json.Unmarshal(raw, &oResp); err != nil || len(oResp.Choices) == 0 {
		return nil, fmt.Errorf("llmclient openai: parse response: %w", err)
	}

	return &CompletionResponse{
		Text:         oResp.Choices[0].Message.Content,
		InputTokens:  oResp.Usage.PromptTokens,
		OutputTokens: oResp.Usage.CompletionTokens,
		FinishReason: oResp.Choices[0].FinishReason,
	}, nil
}
