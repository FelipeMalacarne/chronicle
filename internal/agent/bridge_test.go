package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

// versionBridge answers /bridge/version and /bridge/break like Chronicle Link
// 1.12 and records every path it was asked for.
type versionBridge struct {
	mu        sync.Mutex
	version   string
	breakJSON string
	paths     []string
}

func (b *versionBridge) JSON(_ context.Context, method, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paths = append(b.paths, method+" "+path)
	switch path {
	case "/bridge/version":
		return json.Marshal(map[string]string{"version": b.version})
	case "/bridge/break":
		return json.RawMessage(b.breakJSON), nil
	}
	return nil, errors.New("HTTP 404: Not Found")
}

// The status reads the bridge's version and the break only while the client
// runs, and leaves them out when no bridge answers.
func TestStatusReadsTheBridgeVersion(t *testing.T) {
	ctx := context.Background()
	game := &fakeGame{}
	bridge := &versionBridge{version: "1.12.0", breakJSON: `{"active":false,"in_s":300}`}
	e := &Executor{Game: game, Bridge: bridge, Home: t.TempDir()}

	st := e.status(ctx)
	if st.Running || st.Bridge != "" || st.Break != nil || len(bridge.paths) != 0 {
		t.Fatalf("client down: status %+v, bridge asked %v", st, bridge.paths)
	}

	game.running = true
	st = e.status(ctx)
	if !st.Running || st.Bridge != "1.12.0" || len(bridge.paths) != 2 || bridge.paths[0] != "GET /bridge/version" || bridge.paths[1] != "GET /bridge/break" {
		t.Fatalf("client up: status %+v, bridge asked %v", st, bridge.paths)
	}
	if st.Break == nil || st.Break.Active || st.Break.InS == nil || *st.Break.InS != 300 || st.Break.LeftS != nil {
		t.Fatalf("break = %+v", st.Break)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil || wire["bridge"] != "1.12.0" ||
		!reflect.DeepEqual(wire["break"], map[string]any{"active": false, "in_s": float64(300)}) {
		t.Fatalf("wire status = %s, %v", raw, err)
	}

	for name, b := range map[string]Requester{
		"no bridge configured":      nil,
		"bridge down or plugin off": failing{},
		"not the version shape":     answering(`["1.12.0"]`),
		"plugin older than 1.12":    answering(`{"error":"Not Found"}`),
	} {
		e := &Executor{Game: game, Bridge: b, Home: t.TempDir()}
		if st := e.status(ctx); !st.Running || st.Bridge != "" || st.Break != nil || st.Error != "" {
			t.Fatalf("%s: status = %+v", name, st)
		}
	}
}

func TestParseBreak(t *testing.T) {
	n := func(v int) *int { return &v }
	for _, tc := range []struct {
		name string
		raw  string
		want *BreakState
	}{
		{"waiting", `{"active":false,"in_s":300}`, &BreakState{InS: n(300)}},
		{"on a break", `{"active":true,"left_s":120}`, &BreakState{Active: true, LeftS: n(120)}},
		{"due now", `{"active":false,"in_s":0}`, &BreakState{InS: n(0)}},
		{"times unknown", `{"active":true}`, &BreakState{Active: true}},
		{"explicit nulls", `{"active":false,"in_s":null,"left_s":null}`, &BreakState{}},
		{"unset counters", `{"active":false,"in_s":-1,"left_s":-1}`, &BreakState{}},
		{"no active", `{"in_s":300}`, nil},
		{"not an object", `[1]`, nil},
		{"not json", `Not Found`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBreak([]byte(tc.raw)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseBreak(%s) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
	// A bridge without the route (plugin older than 1.12) leaves the break out.
	if got := (&Executor{Game: &fakeGame{}, Bridge: failing{}}).bridgeBreak(context.Background()); got != nil {
		t.Fatalf("no route = %+v", got)
	}
}
