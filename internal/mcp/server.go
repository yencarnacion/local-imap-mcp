package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"local-imap-mcp/internal/imapclient"
)

type Server struct {
	runner *ToolRunner
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func NewServer(runner *ToolRunner) *Server {
	return &Server{runner: runner}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"name": "local-imap-mcp", "ok": true})
			return
		}
		http.NotFound(w, r)
	})
	return mux
}

func (s *Server) MCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	resp, ok := s.handle(req)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeRPC(w, resp)
}

func (s *Server) handle(req request) (response, bool) {
	hasID := len(req.ID) > 0
	resp := response{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "local-imap-mcp",
				"version": "0.1.0",
			},
		}
	case "notifications/initialized":
		return resp, hasID
	case "tools/list":
		resp.Result = map[string]any{"tools": Tools()}
	case "tools/call":
		result, err := s.callTool(req.Params)
		if err != nil {
			resp.Error = rpcErr(err)
		} else {
			resp.Result = toolResult(result)
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
	}

	return resp, hasID
}

func (s *Server) callTool(raw json.RawMessage) (any, error) {
	var params callParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("tool name is required")
	}

	start := time.Now()
	result, err := s.runner.Call(params.Name, params.Arguments)
	log.Printf("tool_call name=%s duration=%s error=%t", params.Name, time.Since(start).Round(time.Millisecond), err != nil)
	return result, err
}

func toolResult(result any) any {
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		b = []byte(fmt.Sprintf("%v", result))
	}
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": string(b)},
		},
		"structuredContent": result,
		"isError":           false,
	}
}

func rpcErr(err error) *rpcError {
	if errors.Is(err, imapclient.ErrAuthFailed) {
		return &rpcError{Code: -32001, Message: imapclient.ErrAuthFailed.Error()}
	}
	if errors.Is(err, imapclient.ErrMailboxNotFound) {
		return &rpcError{Code: -32002, Message: imapclient.ErrMailboxNotFound.Error()}
	}
	if errors.Is(err, imapclient.ErrMessageNotFound) {
		return &rpcError{Code: -32003, Message: imapclient.ErrMessageNotFound.Error()}
	}
	return &rpcError{Code: -32602, Message: err.Error()}
}

func writeRPC(w http.ResponseWriter, resp response) {
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
