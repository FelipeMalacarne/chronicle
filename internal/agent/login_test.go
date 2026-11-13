package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/microbot"
)

func TestLoginRefusal(t *testing.T) {
	// What reaches the server: the agent's error text (microbot.APIError of
	// the Agent Server's answer) inside a CommandError.
	over := func(command string, err error) error { return &CommandError{Command: command, Message: err.Error()} }
	refused := &microbot.APIError{Path: "/login", Status: 401, Message: `{"success":false,"message":"Login failed: Invalid credentials","currentWorld":330,"loginIndex":4,"loginError":"Invalid credentials"}`}
	timedOut := &microbot.APIError{Path: "/login", Status: 401, Message: `{"success":false,"message":"Login timed out after 60s","currentWorld":330}`}
	tests := []struct {
		name   string
		err    error
		reason string
		ok     bool
	}{
		{"refused", over(CmdLogin, refused), "Invalid credentials", true},
		{"timed out", over(CmdLogin, timedOut), "Login timed out after 60s", true},
		{"not json", over(CmdLogin, &microbot.APIError{Path: "/login", Status: 401, Message: "nope"}), "nope", true},
		{"wrapped", fmt.Errorf("call: %w", over(CmdLogin, refused)), "Invalid credentials", true},
		{"in progress", over(CmdLogin, &microbot.APIError{Path: "/login", Status: 409, Message: "Login attempt already in progress"}), "", false},
		{"cooldown", over(CmdLogin, &microbot.APIError{Path: "/login", Status: 429, Message: `{"success":false}`}), "", false},
		{"client down", over(CmdLogin, microbot.ErrNotRunning), "", false},
		{"another command", over(CmdLogout, refused), "", false},
		{"no agent", ErrNoAgent, "", false},
		{"nil", nil, "", false},
	}
	for _, tt := range tests {
		reason, ok := LoginRefusal(tt.err)
		if reason != tt.reason || ok != tt.ok {
			t.Errorf("%s: LoginRefusal = %q, %v; want %q, %v", tt.name, reason, ok, tt.reason, tt.ok)
		}
	}
	long := over(CmdLogin, &microbot.APIError{Path: "/login", Status: 401, Message: strings.Repeat("é", 300)})
	if reason, ok := LoginRefusal(long); !ok || len(reason) > 204 || !strings.HasSuffix(reason, "…") {
		t.Fatalf("long reason: %q", reason)
	}
}
