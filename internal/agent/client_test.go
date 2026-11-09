package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMaskSecrets(t *testing.T) {
	for in, want := range map[string]string{
		"Agent Server started on UDS":                           "Agent Server started on UDS",
		"token=abc123 rest":                                     "token=*** rest",
		`"sessionId": "d41d8cd98f00b204e9800998ecf8427e"`:       `"sessionId": "***"`,
		"JX_SESSION_ID=xyz":                                     "JX_SESSION_ID=***",
		"bearer eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.SflKxwRJSMeK": "bearer ***",
		"key 0123456789abcdef0123456789abcdef0123 end":          "key *** end",
		"walked to 3222, 3218 in 412 ms":                        "walked to 3222, 3218 in 412 ms",
	} {
		if got := maskSecrets(in); got != want {
			t.Errorf("maskSecrets(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientLogs(t *testing.T) {
	home := t.TempDir()
	e := &Executor{Home: home}
	if logs, err := e.clientLogs(10); err != nil || len(logs.Lines) != 0 {
		t.Fatalf("no log yet = %+v %v", logs, err)
	}
	dir := filepath.Join(home, ".runelite", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := range 50 {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%3))
		b.WriteString("\n")
	}
	b.WriteString("token=secret\n")
	if err := os.WriteFile(filepath.Join(dir, "client.log"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	logs, err := e.clientLogs(3)
	if err != nil || logs.File != "client.log" || len(logs.Lines) != 3 || logs.Lines[2] != "token=***" {
		t.Fatalf("logs = %+v %v", logs, err)
	}
	if _, err := ValidateParams(CmdClientLogs, json.RawMessage(`{"lines":5000}`)); err == nil {
		t.Error("5000 lines accepted")
	}
}

// fakeAccount stands in for mb-account: `show` and `list` like the harness,
// `use N` records N.
const fakeAccount = `case "$1" in
show) echo "Iron Felipe" ;;
list) printf ' 1  Iron Felipe\n 2  Not set\n' ;;
use) echo "dev client will use: #$2" ;;
esac`

func TestAccountAndWorld(t *testing.T) {
	ctx := context.Background()
	game := &fakeGame{}
	e := &Executor{Game: game, Home: t.TempDir(), AccountCmd: []string{"sh", "-c", fakeAccount, "mb-account"}}

	a, err := e.client(ctx, CmdAccountShow, nil)
	if acc, _ := a.(Account); err != nil || acc.Current != "Iron Felipe" || !slices.Equal(acc.Characters, []string{"Iron Felipe", "Not set"}) {
		t.Fatalf("account = %+v %v", a, err)
	}
	out, err := e.client(ctx, CmdAccountUse, json.RawMessage(`{"index":2}`))
	if co, _ := out.(ClientOutput); err != nil || !strings.Contains(co.Output, "#2") {
		t.Fatalf("account use = %+v %v", out, err)
	}
	if _, err = e.client(ctx, CmdAccountUse, json.RawMessage(`{"index":"2; rm -rf"}`)); err == nil {
		t.Fatal("a non-number index was accepted")
	}
	// The character applies on the next start: refused with the client up.
	game.running = true
	if _, err = e.client(ctx, CmdAccountUse, json.RawMessage(`{"index":1}`)); err == nil || !strings.Contains(err.Error(), "stop the client") {
		t.Fatalf("account use with the client running = %v", err)
	}
	if _, err = e.client(ctx, CmdWorldHop, json.RawMessage(`{"world":330}`)); err != nil || !slices.Equal(game.worlds, []int{330}) {
		t.Fatalf("world hop = %v, logins %v", err, game.worlds)
	}
	if _, err = e.client(ctx, CmdWorldHop, json.RawMessage(`{"world":42}`)); err == nil {
		t.Fatal("world 42 accepted")
	}
}
