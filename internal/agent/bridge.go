package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// bridgeTimeout bounds the version read of a status: the bridge is a local
// socket, and the cockpit polls the status every few seconds.
const bridgeTimeout = 2 * time.Second

// bridgeVersion is the Chronicle Link version its control bridge reports
// (GET /bridge/version, Chronicle Link 1.12+), "" when no bridge answers: none
// configured, the client down, the plugin off or older than the route. The
// hello cannot carry it: in a worker it goes out before the client and the
// bridge come up (docs/11 section 14.3).
func (e *Executor) bridgeVersion(ctx context.Context) string {
	if e.Bridge == nil {
		return ""
	}
	raw, err := e.Bridge.JSON(ctx, http.MethodGet, "/bridge/version", nil, nil, bridgeTimeout)
	if err != nil {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return strings.TrimSpace(v.Version)
}

// bridgeBreak is the client's break state from GET /bridge/break, nil when
// no bridge answers it or the answer does not read as one.
func (e *Executor) bridgeBreak(ctx context.Context) *BreakState {
	if e.Bridge == nil {
		return nil
	}
	raw, err := e.Bridge.JSON(ctx, http.MethodGet, "/bridge/break", nil, nil, bridgeTimeout)
	if err != nil {
		return nil
	}
	return parseBreak(raw)
}

// parseBreak reads the bridge's {"active", "in_s", "left_s"}: active is
// required, a negative or absent time is unknown (nil).
func parseBreak(raw []byte) *BreakState {
	var v struct {
		Active *bool `json:"active"`
		InS    *int  `json:"in_s"`
		LeftS  *int  `json:"left_s"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Active == nil {
		return nil
	}
	known := func(s *int) *int {
		if s == nil || *s < 0 {
			return nil
		}
		return s
	}
	return &BreakState{Active: *v.Active, InS: known(v.InS), LeftS: known(v.LeftS)}
}
