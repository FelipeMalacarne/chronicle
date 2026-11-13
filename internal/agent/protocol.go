// Package agent is the Microbot control link (docs/08): the protocol between
// `chronicle serve` and `chronicle agent`, and the agent itself. The agent runs on
// the machine with the game, opens an OUTBOUND websocket to the server
// (/api/microbot/agent, authenticated per mode) and executes only the commands of a
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
	// CmdVNCOpen: open one live VNC session (docs/08 phase 2). The agent
	// connects to the harness's x11vnc on loopback and opens a second
	// outbound websocket to the server for that session's bytes.
	CmdVNCOpen = "vnc_open"
)

// Commands lists every command of the protocol, in display order.
var Commands = []string{
	CmdStatus, CmdClientStart, CmdClientStop, CmdLogin, CmdLogout, CmdScreenshot, CmdVNCOpen,
	CmdPlugins, CmdHubSearch, CmdPluginInstall, CmdPluginRemove, CmdScriptStart, CmdScriptStop,
	CmdScriptsPause, CmdScriptsResume, CmdScriptDetail, CmdConfigGet, CmdConfigSet,
	CmdGameView, CmdWalk, CmdInventoryUse, CmdInventoryDrop, CmdBankOpen, CmdBankClose, CmdBankDeposit,
	CmdBankWithdraw, CmdNPCInteract, CmdObjectInteract, CmdGroundPickup, CmdDialogueContinue, CmdDialogueSelect,
	CmdClientLogs, CmdAccountShow, CmdAccountUse, CmdWorldHop, CmdConfigSchema,
	CmdEquipmentUse, CmdPrayerToggle, CmdSpellCast, CmdBankDepositX, CmdItemUseOn, CmdTimers,
	CmdRunStart, CmdRunStatus, CmdRunHistory, CmdRunCancel, CmdTriggerGet, CmdTriggerSet,
	CmdWorkerBootstrap, CmdRoutinesSync, CmdRecurringGet, CmdTaskRun, CmdRunUntil,
}

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
	// TypeEvent: agent → server, unasked: something the agent did by itself
	// (docs/11 section 4.4). No answer; the server applies it idempotently,
	// and the agent sends its history again on every connection.
	TypeEvent = "event"
)

// Event is the payload of a TypeEvent frame.
type Event struct {
	// Run is a run as it stands now, with its whole trail: sent at every
	// change of the run and, for the runs of the history, on connecting.
	Run *Run `json:"run,omitempty"`
}

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
	Event   *Event          `json:"event,omitempty"`
	// Account names the game client of an agent that drives several
	// (Hello.Clients): on a command, the client that runs it; on an event,
	// the client it comes from. Empty on the link of an agent with one client.
	Account string `json:"account,omitempty"`
}

// FeaturesHeader is the header of the server's answer to the agent's
// websocket upgrade listing what its link understands beyond the first
// protocol; an older server sends none, and the agent then keeps to it.
const FeaturesHeader = "Chronicle-Link-Features"

// FeatureClients: the server routes the clients of an agent that drives
// several (Hello.Clients, Message.Account; docs/10 section 10).
const FeatureClients = "clients"

// MaxClients bounds the game clients one agent drives.
const MaxClients = 16

// ClientHello is one game client an agent declares in its hello.
type ClientHello struct {
	// Account is the Chronicle account the client serves.
	Account string `json:"account"`
}

// ValidAccount reports whether id has the shape of a Chronicle account id
// (acc_<normalized rsn>).
func ValidAccount(id string) bool { return reAccountID.MatchString(id) }

// Hello describes the agent that connected.
type Hello struct {
	Host     string   `json:"host"`
	Version  string   `json:"version"`
	Commands []string `json:"commands"`
	// Account is the Chronicle account the agent serves (MICROBOT_AGENT_ACCOUNT
	// for an attached agent, WORKER_ACCOUNT for a managed one); empty on an
	// agent that declares none (docs/11 section 2).
	Account string `json:"account,omitempty"`
	// Kind is KindAttached or KindManaged; empty means attached.
	Kind string `json:"kind,omitempty"`
	// Worker is the pod name of a managed agent.
	Worker string `json:"worker,omitempty"`
	// Bridge stays "" today: the hello goes out before the client (and the
	// Chronicle Link bridge) come up in a worker, so the version is read
	// while the client runs and reported in Status.Bridge instead.
	Bridge string `json:"bridge,omitempty"`
	// Clients are the game clients of an attached agent that drives several
	// (docs/10 section 10), each with its own account, executor and lock;
	// Account is then the first one's, for an older server, which links only
	// that one. Sent only to a server that announces FeatureClients; empty
	// on an agent with one client.
	Clients []ClientHello `json:"clients,omitempty"`
}

// LoginParams are the parameters of CmdLogin: world 0 keeps the profile's.
type LoginParams struct {
	World int `json:"world,omitempty"`
}

// VNCParams are the parameters of CmdVNCOpen: the server's id of the browser
// session waiting for the agent's stream.
type VNCParams struct {
	Session string `json:"session"`
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
	// Bridge is the Chronicle Link version its control bridge answered
	// (GET /bridge/version), read while the client runs; "" when no bridge
	// answered (plugin off, or older than 1.12).
	Bridge string `json:"bridge,omitempty"`
	// Break is the Microbot Break Handler's state the bridge answered
	// (GET /bridge/break); nil when the bridge did not answer it.
	Break *BreakState `json:"break,omitempty"`
	Error string      `json:"error,omitempty"`
}

// BreakState is the client's break, as the Chronicle Link bridge reads it
// from the Break Handler's static fields (Chronicle Link 1.12+).
type BreakState struct {
	// Active: a Break Handler break (any state but waiting), an antiban
	// micro break or a Break Handler V2 break, as the client's own
	// ClientAlertReporter counts them.
	Active bool `json:"active"`
	// InS is the seconds to the next Break Handler break, LeftS the seconds
	// left in the current one; nil when unknown (no Break Handler running,
	// its counter unset, or Break Handler V2, which keeps its times private).
	InS   *int `json:"in_s,omitempty"`
	LeftS *int `json:"left_s,omitempty"`
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
