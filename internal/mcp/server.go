// Package mcp is a minimal Model Context Protocol server over stdio: JSON-RPC
// 2.0, one message per line, tools only. It is shared by `chronicle content mcp`
// (offline extraction over content/) and, later, `chronicle mcp` (account
// progress over the API). It knows nothing about either.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
)

// ProtocolVersions are the MCP revisions the server speaks, newest first.
var ProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is one callable tool. Handler receives the raw arguments and returns
// a value that is sent back as JSON text; an error is a tool error the model
// sees (isError), not a protocol error.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     func(ctx context.Context, args json.RawMessage) (any, error)
}

// Server serves tools to one client over a reader/writer pair.
type Server struct {
	Name         string
	Version      string
	Instructions string
	Tools        []Tool
	Log          *slog.Logger

	mu sync.Mutex
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve reads requests from r until EOF or ctx is done and writes responses
// to w. Requests are handled one at a time, in order.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	if s.Log == nil {
		s.Log = slog.New(slog.DiscardHandler)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if resp := s.handle(ctx, line); resp != nil {
			// Encode adds the newline that ends the message.
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// handle answers one message; notifications get no response.
func (s *Server) handle(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParse, Message: err.Error()}}
	}
	if len(req.ID) == 0 {
		s.Log.Debug("mcp notification", "method", req.Method)
		return nil
	}
	resp := &response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{Code: codeInvalidRequest, Message: "not a JSON-RPC 2.0 request"}
		return resp
	}
	result, rerr := s.dispatch(ctx, req)
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := ProtocolVersions[0]
		if slices.Contains(ProtocolVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
			"instructions":    s.Instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		tools := make([]map[string]any, len(s.Tools))
		for i, t := range s.Tools {
			tools[i] = map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema}
		}
		return map[string]any{"tools": tools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		i := slices.IndexFunc(s.Tools, func(t Tool) bool { return t.Name == p.Name })
		if i < 0 {
			return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("unknown tool %q", p.Name)}
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		return s.call(ctx, s.Tools[i], p.Arguments), nil
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: fmt.Sprintf("method %q not found", req.Method)}
}

// call runs a tool; handler errors and panics become tool errors.
func (s *Server) call(ctx context.Context, t Tool, args json.RawMessage) (result map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			s.Log.Error("mcp tool panicked", "tool", t.Name, "panic", r)
			result = toolResult(fmt.Sprintf("internal error: %v", r), true)
		}
	}()
	out, err := t.Handler(ctx, args)
	if err != nil {
		s.Log.Info("mcp tool error", "tool", t.Name, "err", err)
		return toolResult(err.Error(), true)
	}
	if text, ok := out.(string); ok {
		return toolResult(text, false)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		return toolResult("encode result: "+err.Error(), true)
	}
	return toolResult(buf.String(), false)
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

// Decode unmarshals tool arguments strictly: unknown fields are an error the
// model sees, so a misspelled field is never silently dropped.
func Decode(args json.RawMessage, into any) error {
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if dec.More() {
		return errors.New("invalid arguments: trailing data")
	}
	return nil
}
