package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ── Google request/response types ────────────────────────────────────────────

type googleRequest struct {
	Contents          []googleContent        `json:"contents"`
	GenerationConfig  googleGenerationConfig `json:"generationConfig"`
	SystemInstruction *googleContent         `json:"systemInstruction,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenerationConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float32 `json:"temperature,omitempty"`
}

type googleResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// callGoogle sends a completion request to the Google Gemini generateContent API.
func (c *client) callGoogle(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	gr := googleRequest{
		Contents: []googleContent{},
	}

	// System prompt sent as systemInstruction.
	if req.System != "" {
		gr.SystemInstruction = &googleContent{
			Parts: []googlePart{{Text: req.System}},
		}
	}

	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		gr.Contents = append(gr.Contents, googleContent{
			Role:  role,
			Parts: []googlePart{{Text: m.Content}},
		})
	}

	if req.MaxTokens > 0 {
		gr.GenerationConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		gr.GenerationConfig.Temperature = req.Temperature
	}

	body, err := json.Marshal(gr)
	if err != nil {
		return nil, fmt.Errorf("llmclient google: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", c.googleURL, req.Model, c.keys.Google)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llmclient google: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient google: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("llmclient google: api error %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llmclient google: read body: %w", err)
	}

	var gResp googleResponse
	if err := json.Unmarshal(raw, &gResp); err != nil || len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("llmclient google: parse response: %w", err)
	}

	return &CompletionResponse{
		Text:         gResp.Candidates[0].Content.Parts[0].Text,
		InputTokens:  gResp.UsageMetadata.PromptTokenCount,
		OutputTokens: gResp.UsageMetadata.CandidatesTokenCount,
		FinishReason: "stop",
	}, nil
}
