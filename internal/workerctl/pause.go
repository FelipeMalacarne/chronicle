package workerctl

import (
	"encoding/json"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

// The automatic restart on a new image (docs/11 section 7.2, the owner's
// decision of 2026-11-13): a managed worker on an old image restarts by
// itself only at a pause, where a restart loses nothing.

// What a pause is, in the audit's params ("pause").
const (
	// PauseLoggedOut: the client is up, at the login screen.
	PauseLoggedOut = "logged_out"
	// PauseClientDown: the client is not running (a bootstrap that has not
	// brought it up yet, or one that failed): nobody is in the game.
	PauseClientDown = "client_down"
	// PauseBreak: logged out on a Break Handler break (Status.Break.Active).
	PauseBreak = "break"
)

// PauseView is what the reconciler read of a worker to decide whether it is
// paused.
type PauseView struct {
	// Status is the agent's CmdStatus answer; nil when it did not answer.
	Status *agent.Status
	// Run is the agent's CmdRunStatus answer; nil when it did not answer.
	// An agent without the command has no runs: an empty RunStatus.
	Run *agent.RunStatus
	// VNC: a browser has a live VNC session of the worker open on this
	// server (agent.Link.VNCOpen), watching or intervening.
	VNC bool
}

// Paused reports whether the worker is at a pause: nobody on its VNC, no run
// in progress, and the character out of the game (logged out, on a Break
// Handler break or not, or the client down). A break with the character still
// logged in (an antiban micro break) is not a pause: a restart would log out
// an account about to resume. When it is, the string is the pause
// (PauseLoggedOut, PauseClientDown, PauseBreak); when it is not, what holds
// the restart back, for the log. Anything unknown is not a pause: an agent
// that did not answer, a game state that does not read, a client between the
// login screen and the game. The owner decided on 2026-11-13 that a break
// counts too, although a restarted worker stays at the login screen (W3)
// instead of going back into the game when the break ends.
func Paused(v PauseView) (string, bool) {
	switch {
	case v.VNC:
		return "a VNC session is open", false
	case v.Run == nil:
		return "the run status is unknown", false
	case v.Run.Run != nil && !v.Run.Run.Final():
		return "a run is in progress (" + v.Run.Run.State + ")", false
	case v.Status == nil:
		return "the client status is unknown", false
	}
	st := v.Status
	onBreak := st.Break != nil && st.Break.Active
	if !st.Running {
		if st.Error != "" {
			return "the client status is unknown: " + st.Error, false
		}
		return PauseClientDown, true
	}
	var game struct {
		LoggedIn  *bool  `json:"loggedIn"`
		GameState string `json:"gameState"`
	}
	if len(st.State) == 0 || json.Unmarshal(st.State, &game) != nil || game.LoggedIn == nil {
		return "the game state does not read", false
	}
	if *game.LoggedIn {
		return "the character is in the game", false
	}
	switch game.GameState {
	case "LOGGING_IN", "LOADING", "HOPPING", "CONNECTION_LOST":
		// On its way into the game (a login, the Break Handler coming back,
		// a world hop) or about to reconnect.
		return "the client is between the login screen and the game (" + game.GameState + ")", false
	}
	if onBreak {
		return PauseBreak, true
	}
	return PauseLoggedOut, true
}
