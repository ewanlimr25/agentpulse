package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
)

// MCP protocol surface implemented:
//
//   initialize  -> capabilities + server_info handshake
//   tools/list  -> enumerate the AgentPulse tools
//   tools/call  -> invoke a single tool
//   ping        -> liveness probe
//   shutdown    -> graceful close (notification, no response)
//
// We deliberately don't implement resources, prompts, sampling, or roots —
// the AgentPulse surface for IDE-side use is tools-only.

type Server struct {
	cfg    *Config
	client *apiClient
	logger *slog.Logger
	tools  map[string]*toolDef
}

func newServer(client *apiClient, cfg *Config, logger *slog.Logger) *Server {
	s := &Server{cfg: cfg, client: client, logger: logger}
	s.tools = registerTools(client, cfg)
	return s
}

func (s *Server) Serve(ctx context.Context, in *bufio.Reader, out io.Writer) error {
	w := newRPCWriter(out)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := readNext(in)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			s.logger.Warn("readNext failed", "err", err)
			continue
		}
		s.dispatch(ctx, w, req)
	}
}

// dispatch routes a single request to the matching handler. Notifications
// (id == nil) get no response.
func (s *Server) dispatch(ctx context.Context, w *rpcWriter, req *rpcRequest) {
	isNotification := len(req.ID) == 0

	send := func(result any, rerr *rpcError) {
		if isNotification {
			return
		}
		if err := w.write(rpcResponse{ID: req.ID, Result: result, Error: rerr}); err != nil {
			s.logger.Warn("write response failed", "err", err)
		}
	}

	switch req.Method {
	case "initialize":
		send(s.handleInitialize(req.Params), nil)
	case "tools/list":
		send(s.handleToolsList(), nil)
	case "tools/call":
		result, rerr := s.handleToolsCall(ctx, req.Params)
		send(result, rerr)
	case "ping":
		send(map[string]any{}, nil)
	case "notifications/initialized", "shutdown", "exit":
		// Nothing to do — the host signals lifecycle but we have no state.
		send(map[string]any{}, nil)
	default:
		send(nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method})
	}
}

// ── initialize ────────────────────────────────────────────────────────────────

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      map[string]any `json:"serverInfo"`
	Instructions    string         `json:"instructions,omitempty"`
}

func (s *Server) handleInitialize(_ json.RawMessage) initializeResult {
	return initializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		ServerInfo: map[string]any{
			"name":    "agentpulse-mcp",
			"version": "0.1.0",
		},
		Instructions: "Use these tools to query AgentPulse for traces, runs, prompt diffs, replay bundles, and budget status. " +
			"All tools default to AGENTPULSE_PROJECT_ID; pass project_id to override.",
	}
}

// ── tools/list ────────────────────────────────────────────────────────────────

type toolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []toolDescriptor `json:"tools"`
}

func (s *Server) handleToolsList() toolsListResult {
	out := make([]toolDescriptor, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, toolDescriptor{
			Name:        t.name,
			Description: t.description,
			InputSchema: t.inputSchema,
		})
	}
	return toolsListResult{Tools: out}
}

// ── tools/call ────────────────────────────────────────────────────────────────

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolCallContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []toolCallContent `json:"content"`
	IsError bool              `json:"isError,omitempty"`
}

func (s *Server) handleToolsCall(ctx context.Context, raw json.RawMessage) (toolCallResult, *rpcError) {
	var p toolCallParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return toolCallResult{}, &rpcError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
	}
	tool, ok := s.tools[p.Name]
	if !ok {
		return toolCallResult{}, &rpcError{Code: codeMethodNotFound, Message: "unknown tool: " + p.Name}
	}

	out, err := tool.call(ctx, p.Arguments)
	if err != nil {
		s.logger.Warn("tool failed", "tool", p.Name, "err", err)
		return toolCallResult{
			Content: []toolCallContent{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil
	}

	// Tools return a JSON-encodable struct; render as a single text block so
	// the caller's LLM sees the structured output verbatim.
	body, mErr := json.MarshalIndent(out, "", "  ")
	if mErr != nil {
		return toolCallResult{}, &rpcError{Code: codeInternalError, Message: "marshal: " + mErr.Error()}
	}
	return toolCallResult{
		Content: []toolCallContent{{Type: "text", Text: string(body)}},
	}, nil
}
