package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/FelipeMalacarne/chronicle/internal/microbot"
)

// loginRefusedPrefix starts the error of a login the Agent Server waited for
// and that did not get in: its POST /login answers 401 with
// {"success":false,"message":...,"loginIndex":...,"loginError":...} when the
// game refused the credentials, the account cannot enter the world, or the
// login did not finish in time.
var loginRefusedPrefix = (&microbot.APIError{Path: "/login", Status: http.StatusUnauthorized}).Error()

// LoginRefusal tells whether a login command failed because the login itself
// did not get in (the Agent Server's 401), and returns the game's reason
// ("Invalid credentials", "Login timed out after 60s"). Any other failure (no
// agent, a login already in progress, the cooldown after too many attempts,
// the link timing out) says nothing about the session and returns false.
func LoginRefusal(err error) (string, bool) {
	var cmd *CommandError
	if !errors.As(err, &cmd) || cmd.Command != CmdLogin {
		return "", false
	}
	i := strings.Index(cmd.Message, loginRefusedPrefix)
	if i < 0 {
		return "", false
	}
	body := strings.TrimSpace(cmd.Message[i+len(loginRefusedPrefix):])
	reason := body
	var answer struct {
		Message    string `json:"message"`
		LoginError string `json:"loginError"`
	}
	if json.Unmarshal([]byte(body), &answer) == nil {
		switch {
		case answer.LoginError != "":
			reason = answer.LoginError
		case answer.Message != "":
			reason = answer.Message
		}
	}
	if reason == "" {
		reason = "login refused"
	}
	const most = 200
	if len(reason) > most {
		n := most
		for n > 0 && !utf8.RuneStart(reason[n]) {
			n--
		}
		reason = reason[:n] + "…"
	}
	return reason, true
}
