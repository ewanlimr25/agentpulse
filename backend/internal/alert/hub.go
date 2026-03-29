// Package alert manages WebSocket connections for real-time budget alerts.
package alert

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/agentpulse/agentpulse/backend/internal/authutil"
	"github.com/agentpulse/agentpulse/backend/internal/store"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for MVP; tighten in production.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Event is pushed to connected WebSocket clients for both budget and signal alerts.
// Use the Type field to distinguish: "budget.alert" | "signal.alert".
type Event struct {
	Type      string  `json:"type"`
	ProjectID string  `json:"project_id"`
	RunID     string  `json:"run_id,omitempty"`
	RuleID    string  `json:"rule_id"`
	RuleName  string  `json:"rule_name"`
	Action    string  `json:"action"` // "notify" | "halt"

	// Budget-specific fields (present when Type == "budget.alert")
	CostUSD  float64 `json:"cost_usd,omitempty"`
	LimitUSD float64 `json:"limit_usd,omitempty"`

	// Signal-specific fields (present when Type == "signal.alert")
	SignalType   string  `json:"signal_type,omitempty"`
	CurrentValue float64 `json:"current_value,omitempty"`
	Threshold    float64 `json:"threshold,omitempty"`
}

// Hub manages active WebSocket clients and broadcasts events.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	events   chan Event
	projects store.ProjectStore
}

type client struct {
	projectID string
	conn      *websocket.Conn
	send      chan []byte
}

func NewHub(projects store.ProjectStore) *Hub {
	return &Hub{
		clients:  make(map[*client]struct{}),
		events:   make(chan Event, 256),
		projects: projects,
	}
}

// Run starts the hub's event loop. Call in a goroutine.
func (h *Hub) Run() {
	for evt := range h.events {
		data, err := json.Marshal(evt)
		if err != nil {
			slog.Error("alert hub marshal", "error", err)
			continue
		}
		h.broadcast(evt.ProjectID, data)
	}
}

// Publish sends an event to all subscribers of the event's project.
func (h *Hub) Publish(evt Event) {
	select {
	case h.events <- evt:
	default:
		slog.Warn("alert hub event channel full, dropping event", "project_id", evt.ProjectID)
	}
}

// ServeWS upgrades the HTTP connection and registers the client.
// Clients must supply a valid Bearer token and a ?project_id= query param.
// The token is verified to belong to the requested project before upgrading.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	token, ok := authutil.ExtractBearer(r)
	if !ok {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	hash := authutil.HashToken(token)
	project, err := h.projects.GetByAPIKeyHash(r.Context(), hash)
	if err != nil {
		http.Error(w, "invalid API key", http.StatusUnauthorized)
		return
	}

	if project.ID != projectID {
		http.Error(w, "API key does not belong to this project", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "error", err)
		return
	}

	c := &client{
		projectID: projectID,
		conn:      conn,
		send:      make(chan []byte, 64),
	}

	h.register(c)
	go c.writePump()
	c.readPump(h) // blocks until disconnect
}

func (h *Hub) register(c *client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *Hub) broadcast(projectID string, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.projectID != projectID {
			continue
		}
		select {
		case c.send <- data:
		default:
			slog.Warn("alert hub client send buffer full", "project_id", projectID)
		}
	}
}

// readPump drains incoming messages (we don't expect any from clients in MVP).
func (c *client) readPump(h *Hub) {
	defer func() {
		h.unregister(c)
		c.conn.Close()
	}()
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}
