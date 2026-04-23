// Package emaildigest polls for due email digests and delivers them via the Resend API.
package emaildigest

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

	"github.com/agentpulse/agentpulse/backend/internal/domain"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

const (
	resendAPIURL   = "https://api.resend.com/emails"
	pollInterval   = 5 * time.Minute
	maxEventsLimit = 20
)

// Sender polls for due email digests every 5 minutes and delivers them via Resend.
// If RESEND_API_KEY is not configured, Run is a no-op.
type Sender struct {
	resendAPIKey string
	fromAddress  string
	digestStore  store.EmailDigestStore
	ruleStore    store.AlertRuleStore
	httpClient   *http.Client
}

// NewSender returns a new Sender. If resendAPIKey is empty, Run logs and returns.
func NewSender(resendAPIKey, fromAddress string, digestStore store.EmailDigestStore, ruleStore store.AlertRuleStore) *Sender {
	return &Sender{
		resendAPIKey: resendAPIKey,
		fromAddress:  fromAddress,
		digestStore:  digestStore,
		ruleStore:    ruleStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Run polls for due digests on a 5-minute interval until ctx is cancelled.
// If RESEND_API_KEY is not configured it logs an info message and returns immediately.
func (s *Sender) Run(ctx context.Context) {
	if s.resendAPIKey == "" {
		slog.Info("email digest disabled (RESEND_API_KEY not set)")
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.process(ctx)
		}
	}
}

func (s *Sender) process(ctx context.Context) {
	configs, err := s.digestStore.ListDue(ctx)
	if err != nil {
		slog.Warn("emaildigest list_due", "error", err)
		return
	}

	for _, cfg := range configs {
		events, err := s.ruleStore.ListEvents(ctx, cfg.ProjectID, maxEventsLimit)
		if err != nil {
			slog.Warn("emaildigest list_events", "project_id", cfg.ProjectID, "error", err)
			continue
		}

		if len(events) == 0 {
			if err := s.digestStore.UpdateLastSent(ctx, cfg.ProjectID); err != nil {
				slog.Warn("emaildigest update_last_sent (no events)", "project_id", cfg.ProjectID, "error", err)
			}
			continue
		}

		body := buildEmailBody(events)
		subject := fmt.Sprintf("AgentPulse digest: %d alerts in the last period", len(events))
		if err := s.sendEmail(cfg.RecipientEmail, subject, body); err != nil {
			slog.Warn("emaildigest send_failed", "project_id", cfg.ProjectID, "error", err)
			if updateErr := s.digestStore.UpdateLastError(ctx, cfg.ProjectID, err.Error()); updateErr != nil {
				slog.Warn("emaildigest update_last_error", "project_id", cfg.ProjectID, "error", updateErr)
			}
			continue
		}

		if err := s.digestStore.UpdateLastSent(ctx, cfg.ProjectID); err != nil {
			slog.Warn("emaildigest update_last_sent", "project_id", cfg.ProjectID, "error", err)
		}
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func (s *Sender) sendEmail(recipient, subject, body string) error {
	payload, err := json.Marshal(resendRequest{
		From:    s.fromAddress,
		To:      []string{recipient},
		Subject: subject,
		Text:    body,
	})
	if err != nil {
		return fmt.Errorf("emaildigest marshal_payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, resendAPIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("emaildigest build_request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.resendAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("emaildigest http_send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("emaildigest resend_api status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

// buildEmailBody produces a plain-text summary of recent alert events.
func buildEmailBody(events []*domain.AlertEvent) string {
	var sb strings.Builder
	sb.WriteString("AgentPulse — Recent Alert Events\n")
	sb.WriteString("=================================\n\n")
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("Rule:    %s\n", e.RuleID))
		sb.WriteString(fmt.Sprintf("Signal:  %s\n", string(e.SignalType)))
		sb.WriteString(fmt.Sprintf("Value:   %.4g (threshold: %.4g)\n", e.CurrentValue, e.Threshold))
		sb.WriteString(fmt.Sprintf("Time:    %s\n", e.TriggeredAt.UTC().Format(time.RFC3339)))
		sb.WriteString("\n")
	}
	return sb.String()
}
