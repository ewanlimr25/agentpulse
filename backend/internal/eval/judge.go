package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type judgeResponse struct {
	Score     float32 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

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

// callJudge sends a pre-built prompt to Claude and returns a parsed score.
func callJudge(ctx context.Context, apiKey, prompt string) (*judgeResponse, error) {
	body, _ := json.Marshal(anthropicRequest{
		Model:     judgeModel,
		MaxTokens: 256,
		// Prefill forces Claude to start with '{', preventing markdown fences.
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
			{Role: "assistant", Content: "{"},
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("judge: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("judge: http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("judge: api error %d: %s", resp.StatusCode, string(raw))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil || len(ar.Content) == 0 {
		return nil, fmt.Errorf("judge: parse anthropic response: %w", err)
	}

	// Reconstruct the full JSON: the prefill '{' is not echoed back by the API,
	// so we prepend it. Also strip any accidental markdown fences defensively.
	text := "{" + ar.Content[0].Text
	text = stripMarkdownFences(text)

	var jr judgeResponse
	if err := json.Unmarshal([]byte(text), &jr); err != nil {
		return nil, fmt.Errorf("judge: parse score json: %w", err)
	}

	// Clamp score to [0, 1]
	if jr.Score < 0 {
		jr.Score = 0
	}
	if jr.Score > 1 {
		jr.Score = 1
	}
	return &jr, nil
}
