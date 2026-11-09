package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The dev client's log and account (docs/08, "logs e conta").
const (
	CmdClientLogs  = "client_logs"  // the tail of the client's log, secrets masked
	CmdAccountShow = "account_show" // the character the dev client launches with, and the launcher's list
	CmdAccountUse  = "account_use"  // pick a character by its number in the list (client stopped)
	CmdWorldHop    = "world_hop"    // logout and login on another world
)

// LogsParams are the parameters of CmdClientLogs.
type LogsParams struct {
	Lines int `json:"lines"`
}

// AccountUseParams are the parameters of CmdAccountUse: the 1-based number
// of the character in `mb-account list`.
type AccountUseParams struct {
	Index int `json:"index"`
}

// WorldParams are the parameters of CmdWorldHop.
type WorldParams struct {
	World int `json:"world"`
}

// ClientLogs is the answer to CmdClientLogs.
type ClientLogs struct {
	File  string   `json:"file"`
	Lines []string `json:"lines"`
}

// Account is the answer to CmdAccountShow: the character names only (the
// sessions in the launcher's file never leave the machine).
type Account struct {
	Current    string   `json:"current"`
	Characters []string `json:"characters"`
	Error      string   `json:"error,omitempty"`
}

func validateClient(command string, raw json.RawMessage) (any, error) {
	switch command {
	case CmdClientLogs:
		var p LogsParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.Lines == 0 {
			p.Lines = 200
		}
		if p.Lines < 1 || p.Lines > 2000 {
			return nil, errors.New("logs: between 1 and 2000 lines")
		}
		return p, nil
	case CmdAccountUse:
		var p AccountUseParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.Index < 1 || p.Index > 50 {
			return nil, errors.New("account: the character's number in the list (1 to 50)")
		}
		return p, nil
	case CmdWorldHop:
		var p WorldParams
		if err := decodeParams(raw, &p); err != nil {
			return nil, err
		}
		if p.World < 301 || p.World > 999 {
			return nil, errors.New("world: between 301 and 999")
		}
		return p, nil
	case CmdAccountShow:
		return nil, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownCommand, command)
}

// Secrets a log line may carry: key=value pairs named like secrets, JWTs and
// long opaque tokens.
var (
	reSecretPair = regexp.MustCompile(`(?i)\b(token|session[_-]?id|secret|password|passwd|jx_session_id|authorization)(["']?\s*[=:]\s*["']?)([^\s"',;]+)`)
	reJWT        = regexp.MustCompile(`eyJ[\w-]{8,}\.[\w-]{8,}\.[\w-]{8,}`)
	reOpaque     = regexp.MustCompile(`\b[A-Za-z0-9_\-]{32,}\b`)
)

// maskSecrets hides what looks like a credential in a log line.
func maskSecrets(line string) string {
	line = reSecretPair.ReplaceAllString(line, "$1$2***")
	line = reJWT.ReplaceAllString(line, "***")
	return reOpaque.ReplaceAllString(line, "***")
}

// clientLogs reads the tail of the client's log (or of its stdout before the
// log exists), like `mb-client logs`.
func (e *Executor) clientLogs(lines int) (ClientLogs, error) {
	for _, f := range []string{
		filepath.Join(e.Home, ".runelite", "logs", "client.log"),
		filepath.Join(e.Home, "run", "client.out"),
	} {
		tail, err := tailFile(f, lines)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ClientLogs{}, err
		}
		for i := range tail {
			tail[i] = maskSecrets(tail[i])
		}
		return ClientLogs{File: filepath.Base(f), Lines: tail}, nil
	}
	return ClientLogs{Lines: []string{}}, nil
}

// tailFile returns the last n lines of a file, reading at most its last 2 MB.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // a fixed file of the dev home
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	const window = 2 << 20
	if st, err := f.Stat(); err == nil && st.Size() > window {
		if _, err := f.Seek(st.Size()-window, 0); err != nil {
			return nil, err
		}
	}
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
		if len(out) > n {
			out = out[1:]
		}
	}
	return out, sc.Err()
}

// account runs `mb-account show` and `list` and keeps only the names.
func (e *Executor) account(ctx context.Context) Account {
	var a Account
	cur, err := e.runAccount(ctx, "show")
	if err != nil {
		a.Error = err.Error()
		return a
	}
	a.Current = strings.TrimSpace(cur)
	list, err := e.runAccount(ctx, "list")
	if err != nil {
		a.Error = err.Error()
		return a
	}
	a.Characters = []string{}
	for line := range strings.SplitSeq(list, "\n") {
		// "nl" numbers the names: " 1  Name".
		f := strings.Fields(line)
		if len(f) >= 2 && strings.Trim(f[0], "0123456789") == "" {
			a.Characters = append(a.Characters, strings.Join(f[1:], " "))
		}
	}
	return a
}

func (e *Executor) runAccount(ctx context.Context, args ...string) (string, error) {
	if len(e.AccountCmd) == 0 {
		return "", errors.New("no mb-account command configured (MICROBOT_HARNESS_DIR or MICROBOT_ACCOUNT_CMD)")
	}
	return runArgv(ctx, e.AccountCmd, time.Minute, args...)
}

// client runs the log, account and world commands.
func (e *Executor) client(ctx context.Context, command string, params json.RawMessage) (any, error) {
	p, err := validateClient(command, params)
	if err != nil {
		return nil, err
	}
	switch pp := p.(type) {
	case LogsParams:
		return e.clientLogs(pp.Lines)
	case AccountUseParams:
		if e.status(ctx).Running {
			return nil, errors.New("account: stop the client first (the character applies on the next start)")
		}
		out, err := e.runAccount(ctx, "use", fmt.Sprint(pp.Index))
		return ClientOutput{Output: out}, err
	case WorldParams:
		if _, err := e.Game.Logout(ctx); err != nil {
			return nil, fmt.Errorf("world hop: logout: %w", err)
		}
		return e.Game.Login(ctx, pp.World)
	case nil:
		return e.account(ctx), nil
	}
	return nil, fmt.Errorf("command %q is not in the agent's closed list", command)
}
