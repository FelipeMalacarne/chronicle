package workerctl

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

func TestPaused(t *testing.T) {
	up := func(state string, brk *agent.BreakState) *agent.Status {
		return &agent.Status{Running: true, State: json.RawMessage(state), Break: brk}
	}
	loggedIn := `{"loggedIn":true,"gameState":"LOGGED_IN"}`
	loginScreen := `{"loggedIn":false,"gameState":"LOGIN_SCREEN"}`
	onBreak := &agent.BreakState{Active: true}
	waiting := &agent.BreakState{Active: false}
	noRun := &agent.RunStatus{}
	going := &agent.RunStatus{Run: &agent.Run{ID: "run-1", State: agent.RunRunning}}
	queued := &agent.RunStatus{Run: &agent.Run{ID: "run-2", State: agent.RunWaiting}}
	ended := &agent.RunStatus{Run: &agent.Run{ID: "run-3", State: agent.RunFailed}}
	for _, c := range []struct {
		name  string
		view  PauseView
		pause string // the pause, or a piece of what holds it back
		ok    bool
	}{
		{"logged out, no run", PauseView{Status: up(loginScreen, nil), Run: noRun}, PauseLoggedOut, true},
		{"logged out after a finished run", PauseView{Status: up(loginScreen, waiting), Run: ended}, PauseLoggedOut, true},
		{"logged out by the Break Handler", PauseView{Status: up(loginScreen, onBreak), Run: noRun}, PauseBreak, true},
		// A restart would log out an account about to resume.
		{"micro break, still logged in", PauseView{Status: up(loggedIn, onBreak), Run: noRun}, "the character is in the game", false},
		{"client not running", PauseView{Status: &agent.Status{}, Run: noRun}, PauseClientDown, true},
		// An old agent (no break in its status) still pauses when logged out.
		{"old agent, logged out", PauseView{Status: up(`{"loggedIn":false}`, nil), Run: noRun}, PauseLoggedOut, true},
		{"old agent, in the game", PauseView{Status: up(loggedIn, nil), Run: noRun}, "in the game", false},
		{"in the game, break not yet", PauseView{Status: up(loggedIn, waiting), Run: noRun}, "in the game", false},
		// Never during a run, whatever the game says.
		{"run going, logged out", PauseView{Status: up(loginScreen, nil), Run: going}, "run is in progress", false},
		{"run waiting for its safe point, on a break", PauseView{Status: up(loggedIn, onBreak), Run: queued}, "run is in progress", false},
		// Never under the owner's eyes.
		{"VNC open, logged out", PauseView{Status: up(loginScreen, nil), Run: noRun, VNC: true}, "VNC", false},
		{"VNC open, on a break", PauseView{Status: up(loggedIn, onBreak), Run: noRun, VNC: true}, "VNC", false},
		// Unknown is never a pause.
		{"no run status", PauseView{Status: up(loginScreen, nil)}, "run status is unknown", false},
		{"no client status", PauseView{Run: noRun}, "client status is unknown", false},
		{"client status failed", PauseView{Status: &agent.Status{Error: "dial unix: timeout"}, Run: noRun}, "timeout", false},
		{"state missing", PauseView{Status: &agent.Status{Running: true}, Run: noRun}, "does not read", false},
		{"state without loggedIn", PauseView{Status: up(`{"gameState":"LOGIN_SCREEN"}`, onBreak), Run: noRun}, "does not read", false},
		{"state garbled", PauseView{Status: up(`[`, nil), Run: noRun}, "does not read", false},
		// On the way into the game: the login, the Break Handler coming back.
		{"logging in", PauseView{Status: up(`{"loggedIn":false,"gameState":"LOGGING_IN"}`, nil), Run: noRun}, "LOGGING_IN", false},
		{"break ending, logging in", PauseView{Status: up(`{"loggedIn":false,"gameState":"LOADING"}`, onBreak), Run: noRun}, "LOADING", false},
		{"hopping", PauseView{Status: up(`{"loggedIn":false,"gameState":"HOPPING"}`, nil), Run: noRun}, "HOPPING", false},
		{"connection lost", PauseView{Status: up(`{"loggedIn":false,"gameState":"CONNECTION_LOST"}`, nil), Run: noRun}, "CONNECTION_LOST", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Paused(c.view)
			if ok != c.ok || (ok && got != c.pause) || (!ok && !strings.Contains(got, c.pause)) {
				t.Fatalf("Paused = %q, %v; want %q, %v", got, ok, c.pause, c.ok)
			}
		})
	}
}
