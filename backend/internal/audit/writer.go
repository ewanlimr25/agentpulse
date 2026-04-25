package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Event struct {
	Timestamp  time.Time
	TokenHash  string
	IP         string
	Method     string
	Endpoint   string
	StatusCode int
	Outcome    string // "success" or "error"
}

// Writer asynchronously inserts audit events into ClickHouse.
// Drops events if the buffer is full (non-blocking request path).
type Writer struct {
	ch  driver.Conn
	buf chan Event
}

// NewWriter constructs an audit writer. When ch is nil (indie mode, no
// ClickHouse) Record is a no-op — indie mode prioritises operational
// simplicity over retained audit trails.
func NewWriter(ch driver.Conn) *Writer {
	w := &Writer{ch: ch, buf: make(chan Event, 4096)}
	if ch != nil {
		go w.run()
	}
	return w
}

func (w *Writer) Record(e Event) {
	if w.ch == nil {
		return
	}
	select {
	case w.buf <- e:
	default:
		slog.Warn("audit: buffer full, dropping event", "endpoint", e.Endpoint)
	}
}

func (w *Writer) run() {
	for e := range w.buf {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := w.ch.Exec(ctx,
			`INSERT INTO audit_events (timestamp, token_hash, ip, method, endpoint, status_code, outcome) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			e.Timestamp, e.TokenHash, e.IP, e.Method, e.Endpoint, uint16(e.StatusCode), e.Outcome,
		)
		cancel()
		if err != nil {
			slog.Warn("audit: insert failed", "error", err)
		}
	}
}
