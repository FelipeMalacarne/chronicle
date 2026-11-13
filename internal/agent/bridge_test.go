package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"
)

// versionBridge answers /bridge/version like Chronicle Link 1.12 and records
// every path it was asked for.
type versionBridge struct {
	mu      sync.Mutex
	version string
	paths   []string
}

func (b *versionBridge) JSON(_ context.Context, method, path string, _ url.Values, _ any, _ time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paths = append(b.paths, method+" "+path)
	if path != "/bridge/version" {
		return nil, errors.New("HTTP 404: Not Found")
	}
	return json.Marshal(map[string]string{"version": b.version})
}

// The status reads the bridge's version only while the client runs, and
// leaves it empty when no bridge answers it.
func TestStatusReadsTheBridgeVersion(t *testing.T) {
	ctx := context.Background()
	game := &fakeGame{}
	bridge := &versionBridge{version: "1.12.0"}
	e := &Executor{Game: game, Bridge: bridge, Home: t.TempDir()}

	st := e.status(ctx)
	if st.Running || st.Bridge != "" || len(bridge.paths) != 0 {
		t.Fatalf("client down: status %+v, bridge asked %v", st, bridge.paths)
	}

	game.running = true
	st = e.status(ctx)
	if !st.Running || st.Bridge != "1.12.0" || len(bridge.paths) != 1 || bridge.paths[0] != "GET /bridge/version" {
		t.Fatalf("client up: status %+v, bridge asked %v", st, bridge.paths)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil || wire["bridge"] != "1.12.0" {
		t.Fatalf("wire status = %s, %v", raw, err)
	}

	for name, b := range map[string]Requester{
		"no bridge configured":      nil,
		"bridge down or plugin off": failing{},
		"not the version shape":     answering(`["1.12.0"]`),
		"plugin older than 1.12":    answering(`{"error":"Not Found"}`),
	} {
		e := &Executor{Game: game, Bridge: b, Home: t.TempDir()}
		if st := e.status(ctx); !st.Running || st.Bridge != "" || st.Error != "" {
			t.Fatalf("%s: status = %+v", name, st)
		}
	}
}
