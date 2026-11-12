package agent

// Managed workers (docs/11): the contract shared by the server, the agent in
// --worker mode and the reconciler. The protocol stays additive: an agent
// that sends none of the new hello fields is an attached agent with no
// declared account, as before.

// Agent kinds (Hello.Kind).
const (
	// KindAttached: an agent the server controls but never starts or stops
	// (the one on zaros, run by the owner).
	KindAttached = "attached"
	// KindManaged: an agent in a pod the server created (chronicle agent
	// --worker); it serves exactly one account and gets its session from the
	// server.
	KindManaged = "managed"
)

// CmdWorkerBootstrap: server → managed agent, once per link (and again after
// every reconnect while the client is not running): the account's game
// session, the Chronicle Link credential and the world. Only an agent in
// --worker mode executes it; the params carry secrets, so neither end logs
// or audits them.
const CmdWorkerBootstrap = "worker_bootstrap"

// GameSession is what the client needs to log in as one character without
// the Jagex launcher (docs/11 section 4.2). It is the plaintext of
// game_sessions.ciphertext and never leaves the server except inside
// BootstrapParams.
type GameSession struct {
	// Character is the character's display name (JX_DISPLAY_NAME).
	Character string `json:"character"`
	// SessionID and CharacterID are the launcher's JX_SESSION_ID and
	// JX_CHARACTER_ID.
	SessionID   string `json:"session_id"`
	CharacterID string `json:"character_id"`
	// DeviceIDs and RandomDats are the lines of the HWID agent's stores
	// (spoofed-device-ids.properties, spoofed-random-dats.properties) that
	// belong to this character, as "key=value" lines, so the character keeps
	// the same spoofed device identity on zaros and in the pod.
	DeviceIDs  string `json:"device_ids,omitempty"`
	RandomDats string `json:"random_dats,omitempty"`
}

// BootstrapParams are the parameters of CmdWorkerBootstrap.
type BootstrapParams struct {
	Account string      `json:"account"`
	Session GameSession `json:"session"`
	// PluginToken is the worker's own ipl_ token for the Chronicle Link
	// (scopes state:write and items:*), revoked when the worker stops.
	PluginToken string `json:"plugin_token"`
	// ServerURL is the planner as the Chronicle Link inside the pod reaches
	// it (the in-cluster Service, WORKER_SERVER_URL).
	ServerURL string `json:"server_url"`
	// World is the world to log in to later; 0 keeps the profile's. The
	// bootstrap never logs in (docs/11, W3).
	World int `json:"world,omitempty"`
}

// BootstrapResult is the answer to CmdWorkerBootstrap.
type BootstrapResult struct {
	// Started: the client is up and the Agent Server answers (login screen).
	Started bool   `json:"started"`
	Output  string `json:"output,omitempty"`
}
