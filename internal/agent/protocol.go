// Package agent is the Microbot control link (docs/08): the protocol between
// `ironman serve` and `ironman agent`, and the agent itself. The agent runs on
// the machine with the game, opens an OUTBOUND websocket to the server
// (/api/microbot/agent, machine token) and executes only the commands of a
// closed list; it never relays raw requests to the Agent Server.
package agent

import (
	"encoding/json"
	"slices"
)

// Commands is the closed list the agent executes (docs/08, Segurança item
// 4). Anything else is refused on both ends.
const (
	CmdStatus      = "status"
	CmdClientStart = "client_start"
	CmdClientStop  = "client_stop"
	CmdLogin       = "login"
	CmdLogout      = "logout"
	CmdScreenshot  = "screenshot"
)

// Commands lists every command of the protocol, in display order.
var Commands = []string{CmdStatus, CmdClientStart, CmdClientStop, CmdLogin, CmdLogout, CmdScreenshot}

// Known reports whether name is in the closed list.
func Known(name string) bool { return slices.Contains(Commands, name) }

// Message types on the websocket (JSON text frames).
const (
	// TypeHello: agent → server, first frame: who the agent is.
	TypeHello = "hello"
	// TypeCommand: server → agent, one command with an id.
	TypeCommand = "command"
	// TypeResult: agent → server, the answer to a command id.
	TypeResult = "result"
)

// Message is one frame of the link.
type Message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Command string          `json:"command,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Hello   *Hello          `json:"hello,omitempty"`
}

// Hello describes the agent that connected.
type Hello struct {
	Host     string   `json:"host"`
	Version  string   `json:"version"`
	Commands []string `json:"commands"`
}

// LoginParams are the parameters of CmdLogin: world 0 keeps the profile's.
type LoginParams struct {
	World int `json:"world,omitempty"`
}

// Status is the answer to CmdStatus.
type Status struct {
	// Running: the Agent Server answered /state.
	Running bool `json:"running"`
	// Headless: the harness's Xvfb display is up (mb-client --headless).
	Headless bool `json:"headless"`
	// VNC: the harness's x11vnc is up (127.0.0.1 only).
	VNC bool `json:"vnc"`
	// State and Login are /state and /login of the Agent Server as is.
	State json.RawMessage `json:"state,omitempty"`
	Login json.RawMessage `json:"login,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Screenshot is the answer to CmdScreenshot: the frame re-encoded as JPEG
// (a PNG of the client is megabytes), with its media type.
type Screenshot struct {
	Image []byte `json:"image"`
	Type  string `json:"type"`
}

// ClientOutput is the answer to CmdClientStart and CmdClientStop: the tail of
// the harness command's output.
type ClientOutput struct {
	Output string `json:"output"`
}
