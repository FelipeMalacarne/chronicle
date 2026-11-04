package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestServe(t *testing.T) {
	s := &Server{Name: "t", Version: "1", Tools: []Tool{
		{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var a struct{ Say string }
			if err := Decode(args, &a); err != nil {
				return nil, err
			}
			return map[string]string{"said": a.Say + " >= ok"}, nil
		}},
		{Name: "fail", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("nope") }},
		{Name: "boom", Handler: func(context.Context, json.RawMessage) (any, error) { panic("kaboom") }},
	}}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		``,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"Say":"hi"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":{"typo":"hi"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"fail"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"boom"}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"missing"}}`,
		`{"jsonrpc":"2.0","id":8,"method":"resources/list"}`,
		`not json`,
	}, "\n")
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		`"protocolVersion":"2025-06-18"`, // unknown version: the server offers its newest
		`"id":2,"result":{}`,
		`\"said\": \"hi >= ok\"`,
		`unknown field \"typo\"`,
		`"text":"nope","type":"text"}],"isError":true`,
		`internal error: kaboom`,
		`"code":-32602,"message":"unknown tool \"missing\""`,
		`"code":-32601`,
		`"id":null,"error":{"code":-32700`,
	}
	if len(lines) != len(want) {
		t.Fatalf("%d responses (the notification gets none), want %d:\n%s", len(lines), len(want), out.String())
	}
	for i, w := range want {
		if !strings.Contains(lines[i], w) {
			t.Errorf("response %d = %s\nwant it to contain %s", i, lines[i], w)
		}
	}
}
