// Package hiscores is the adapter of the official OSRS hiscores (docs/02
// section 1): it fetches index_lite.json for an account and normalizes it into
// player_state facts. Only this package knows the external shape.
package hiscores

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the hiscores host; tests point the client elsewhere.
const DefaultBaseURL = "https://secure.runescape.com"

// AdapterVersion is the normalizer version stored with each snapshot.
const AdapterVersion = 1

// maxBody caps the response; a full JSON response is about 10 KB.
const maxBody = 512 << 10

// endpoints maps accounts.mode to the hiscores table. A group ironman has no
// individual table of its own and every account is on the normal one.
var endpoints = map[string]string{
	"ironman": "hiscore_oldschool_ironman",
	"hcim":    "hiscore_oldschool_hardcore_ironman",
	"uim":     "hiscore_oldschool_ultimate",
	"gim":     "hiscore_oldschool",
	"":        "hiscore_oldschool",
}

const normal = "hiscore_oldschool"

// Errors. A *StatusError wraps ErrNotFound or ErrRateLimited when it applies.
var (
	ErrNotFound    = errors.New("account not found on the hiscores")
	ErrRateLimited = errors.New("hiscores rate limited")
	ErrUpstream    = errors.New("hiscores unavailable")
)

// StatusError is a non-200 response.
type StatusError struct {
	Status int
	Err    error
}

func (e *StatusError) Error() string { return fmt.Sprintf("hiscores HTTP %d: %v", e.Status, e.Err) }
func (e *StatusError) Unwrap() error { return e.Err }

// Client fetches the hiscores.
type Client struct {
	BaseURL   string
	HTTP      *http.Client
	UserAgent string
}

// New returns a client for the real hiscores.
func New() *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: "ironman-planner (personal, github FelipeMalacarne)",
	}
}

// Result is a successful fetch.
type Result struct {
	Raw []byte
	// Table is the hiscores table that answered (the mode's or the normal one).
	Table string
}

// Fetch reads the account from the table of its mode; a 404 there falls back
// to the normal table, where every account is listed (an HCIM that died, for
// instance, is no longer on the hardcore one).
func (c *Client) Fetch(ctx context.Context, rsn, mode string) (Result, error) {
	table, ok := endpoints[mode]
	if !ok {
		table = normal
	}
	raw, err := c.get(ctx, table, rsn)
	if errors.Is(err, ErrNotFound) && table != normal {
		table = normal
		raw, err = c.get(ctx, table, rsn)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Raw: raw, Table: table}, nil
}

func (c *Client) get(ctx context.Context, table, rsn string) ([]byte, error) {
	u := fmt.Sprintf("%s/m=%s/index_lite.json?player=%s", strings.TrimRight(c.BaseURL, "/"), table, url.QueryEscape(rsn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, &StatusError{Status: resp.StatusCode, Err: ErrNotFound}
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return nil, &StatusError{Status: resp.StatusCode, Err: ErrRateLimited}
	default:
		return nil, &StatusError{Status: resp.StatusCode, Err: ErrUpstream}
	}
}

// Snapshot is the parsed index_lite.json.
type Snapshot struct {
	Name       string     `json:"name"`
	Skills     []Skill    `json:"skills"`
	Activities []Activity `json:"activities"`
}

// Skill is one skill row; Rank -1 means not ranked.
type Skill struct {
	Name  string `json:"name"`
	Rank  int64  `json:"rank"`
	Level int64  `json:"level"`
	XP    int64  `json:"xp"`
}

// Activity is one boss or activity row; Rank -1 means not ranked.
type Activity struct {
	Name  string `json:"name"`
	Rank  int64  `json:"rank"`
	Score int64  `json:"score"`
}

// Parse decodes a response body.
func Parse(raw []byte) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return Snapshot{}, fmt.Errorf("parse hiscores JSON: %w", err)
	}
	if len(s.Skills) == 0 {
		return Snapshot{}, errors.New("parse hiscores JSON: no skills")
	}
	return s, nil
}

// Fact is one player_state row produced by the normalizer.
type Fact struct {
	Facet string
	Key   string
	Value map[string]int64
}

// Normalize turns a snapshot into facts, mapped by name (the hiscores insert
// new bosses in the middle of the list, so positions are not stable):
//
//   - skills: facet "skill", key the lower-case skill id ("runecraft"),
//     value {level, xp}; Overall is not a skill;
//   - activities and bosses: facet "activity", key the hiscores name,
//     value {score, rank}.
//
// Unranked rows (rank -1) are left out: the hiscores show level 1 and 0 xp
// for an account below the ranking cut, which is not an observation
// (decision of the owner, 2026-11-04: unknown, never "level 1").
func Normalize(s Snapshot) []Fact {
	out := make([]Fact, 0, len(s.Skills)+len(s.Activities))
	for _, sk := range s.Skills {
		if sk.Rank < 0 || strings.EqualFold(sk.Name, "Overall") {
			continue
		}
		out = append(out, Fact{Facet: "skill", Key: strings.ToLower(sk.Name), Value: map[string]int64{"level": sk.Level, "xp": sk.XP}})
	}
	for _, a := range s.Activities {
		if a.Rank < 0 || a.Score < 0 {
			continue
		}
		out = append(out, Fact{Facet: "activity", Key: a.Name, Value: map[string]int64{"score": a.Score, "rank": a.Rank}})
	}
	return out
}

// Detection is the mode found for a name in the setup (docs/02 section 1:
// the table that answers tells the mode) and what that table says.
type Detection struct {
	// Mode is the accounts.mode of the most specific table that knows the
	// name ("hcim", "uim", "ironman"), or empty when only the normal table
	// does (a main account or a group ironman: the owner picks).
	Mode     string
	Found    bool
	Snapshot Snapshot
}

// detectOrder goes from the most specific table to the normal one: an HCIM
// or a UIM is also listed on the ironman table.
var detectOrder = []struct{ mode, table string }{
	{"hcim", "hiscore_oldschool_hardcore_ironman"},
	{"uim", "hiscore_oldschool_ultimate"},
	{"ironman", "hiscore_oldschool_ironman"},
	{"", normal},
}

// Detect probes the tables in order (at most four calls, only in the setup)
// and returns the first that knows the name.
func (c *Client) Detect(ctx context.Context, rsn string) (Detection, error) {
	for _, d := range detectOrder {
		raw, err := c.get(ctx, d.table, rsn)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return Detection{}, err
		}
		snap, err := Parse(raw)
		if err != nil {
			return Detection{}, err
		}
		return Detection{Mode: d.mode, Found: true, Snapshot: snap}, nil
	}
	return Detection{}, nil
}
