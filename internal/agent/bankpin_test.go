package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// pinBridge is the Chronicle Link bridge of a worker: /bridge/version answers
// once up, and /bridge/bank-pin keeps what it was sent.
type pinBridge struct {
	mu   sync.Mutex
	up   bool
	sent []string
}

func (b *pinBridge) JSON(_ context.Context, method, path string, _ url.Values, body any, _ time.Duration) (json.RawMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.up {
		return nil, errors.New("dial unix bridge.sock: no such file")
	}
	switch method + " " + path {
	case "GET /bridge/version":
		return json.RawMessage(`{"version":"1.17.0"}`), nil
	case "POST /bridge/bank-pin":
		raw, _ := json.Marshal(body)
		b.sent = append(b.sent, string(raw))
		return json.RawMessage(`{"success":true}`), nil
	}
	return nil, errors.New("HTTP 404: Not Found")
}

func TestBootstrapWritesTheBankPin(t *testing.T) {
	e, _, _ := podExecutor(t)
	bridge := &pinBridge{up: true}
	e.Bridge = bridge
	ctx := context.Background()
	res, err := e.Execute(ctx, CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Session.BankPin = "0420" }))
	if err != nil {
		t.Fatal(err)
	}
	if r := res.(BootstrapResult); !r.Started || len(r.Problems) != 0 {
		t.Fatalf("bootstrap = %+v", r)
	}
	if len(bridge.sent) != 1 || bridge.sent[0] != `{"pin":"0420"}` {
		t.Fatalf("bridge got %q", bridge.sent)
	}
	// A reconnect while the client runs writes it again (it may have changed).
	if _, err = e.Execute(ctx, CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Session.BankPin = "1357" })); err != nil {
		t.Fatal(err)
	}
	if len(bridge.sent) != 2 || bridge.sent[1] != `{"pin":"1357"}` {
		t.Fatalf("bridge got %q", bridge.sent)
	}
}

func TestBootstrapWithoutABankPinLeavesTheProfileAlone(t *testing.T) {
	e, _, _ := podExecutor(t)
	bridge := &pinBridge{up: true}
	e.Bridge = bridge
	if _, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(nil)); err != nil {
		t.Fatal(err)
	}
	if len(bridge.sent) != 0 {
		t.Fatalf("bridge got %q", bridge.sent)
	}
}

func TestBootstrapRefusesABadBankPin(t *testing.T) {
	e, _, _ := podExecutor(t)
	for _, pin := range []string{"123", "12345", "12a4", " 123"} {
		_, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Session.BankPin = pin }))
		if err == nil || !strings.Contains(err.Error(), "bank_pin must be four digits") || strings.Contains(err.Error(), pin) {
			t.Fatalf("pin %q: %v", pin, err)
		}
	}
}

// Without the bridge the client still starts; the bootstrap says the PIN did
// not reach the profile, never which PIN.
func TestBankPinWithoutTheBridgeIsAProblem(t *testing.T) {
	e, _, _ := podExecutor(t)
	e.Bridge = &pinBridge{}
	e.Worker.BridgeWait = 50 * time.Millisecond
	res, err := e.Execute(context.Background(), CmdWorkerBootstrap, bootstrapParams(func(p *BootstrapParams) { p.Session.BankPin = "0420" }))
	if err != nil {
		t.Fatal(err)
	}
	r := res.(BootstrapResult)
	if !r.Started || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "bridge did not answer") || strings.Contains(r.Problems[0], "0420") {
		t.Fatalf("bootstrap = %+v", r)
	}
}
