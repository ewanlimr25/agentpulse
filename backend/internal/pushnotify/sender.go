// Package pushnotify delivers Web Push notifications to browser subscriptions.
package pushnotify

import (
	"context"
	"encoding/json"
	"log/slog"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/agentpulse/agentpulse/backend/internal/store"
)

// Sender delivers Web Push notifications to all subscriptions for a project.
type Sender struct {
	vapidPublicKey  string
	vapidPrivateKey string
	subject         string
	subs            store.PushSubscriptionStore
}

// NewSender returns a new Sender configured with VAPID credentials.
func NewSender(vapidPublicKey, vapidPrivateKey, subject string, subs store.PushSubscriptionStore) *Sender {
	return &Sender{
		vapidPublicKey:  vapidPublicKey,
		vapidPrivateKey: vapidPrivateKey,
		subject:         subject,
		subs:            subs,
	}
}

// Notify sends a push notification to all subscriptions for a project.
// Removes 410 Gone subscriptions automatically.
func (s *Sender) Notify(ctx context.Context, projectID, title, body string) {
	subs, err := s.subs.ListByProject(ctx, projectID)
	if err != nil {
		slog.Warn("pushnotify list_subscriptions", "project_id", projectID, "error", err)
		return
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	for _, sub := range subs {
		sub := sub
		go func() {
			ws := &webpush.Subscription{
				Endpoint: sub.Endpoint,
				Keys: webpush.Keys{
					P256dh: sub.P256DHKey,
					Auth:   sub.AuthKey,
				},
			}
			resp, err := webpush.SendNotification(payload, ws, &webpush.Options{
				VAPIDPublicKey:  s.vapidPublicKey,
				VAPIDPrivateKey: s.vapidPrivateKey,
				Subscriber:      s.subject,
				TTL:             30,
			})
			if err != nil {
				slog.Warn("pushnotify send", "endpoint", sub.Endpoint, "error", err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 410 || resp.StatusCode == 404 {
				// Subscription expired/revoked — clean it up.
				if delErr := s.subs.Delete(context.Background(), sub.ProjectID, sub.Endpoint); delErr != nil {
					slog.Warn("pushnotify delete_stale_sub", "error", delErr)
				}
			} else if resp.StatusCode >= 400 {
				slog.Warn("pushnotify non-2xx", "status", resp.StatusCode, "endpoint", sub.Endpoint)
			}
		}()
	}
}
