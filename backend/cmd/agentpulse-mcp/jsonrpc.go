package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// JSON-RPC 2.0 wire types. We deliberately keep these minimal — MCP is a thin
// layer on top of JSON-RPC and we only need a handful of fields.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// rpcWriter serializes responses on stdout. JSON-RPC over stdio is one JSON
// object per line ("ndjson"); MCP standardises on this framing.
type rpcWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newRPCWriter(w io.Writer) *rpcWriter {
	return &rpcWriter{w: w}
}

func (rw *rpcWriter) write(resp rpcResponse) error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	resp.JSONRPC = "2.0"
	buf, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	buf = append(buf, '\n')
	if _, err := rw.w.Write(buf); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}

// readNext reads a single JSON-RPC frame from the reader. Returns io.EOF at
// end of stream. Frames are line-delimited; oversized lines are rejected.
func readNext(r *bufio.Reader) (*rpcRequest, error) {
	for {
		line, err := r.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return nil, err
		}
		// Trim whitespace
		trimmed := trimSpaceBytes(line)
		if len(trimmed) == 0 {
			if err != nil {
				return nil, err
			}
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(trimmed, &req); err != nil {
			return nil, fmt.Errorf("invalid frame: %w", err)
		}
		if req.JSONRPC != "2.0" {
			return nil, errors.New("expected jsonrpc=2.0")
		}
		return &req, nil
	}
}

func trimSpaceBytes(b []byte) []byte {
	for len(b) > 0 && isSpace(b[0]) {
		b = b[1:]
	}
	for len(b) > 0 && isSpace(b[len(b)-1]) {
		b = b[:len(b)-1]
	}
	return b
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
