package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// CmdTimers reads the clocks of RuneLite's Time Tracking through the control
// bridge (Chronicle Link 1.6+): one per farming tab, the bird houses and the
// farming contract, the numbers the client's own panel shows. Read-only;
// docs/08 phase 4, delivery 1.
const CmdTimers = "timers"

// Timers is the answer to CmdTimers.
type Timers struct {
	// Now is the game machine's clock (epoch seconds) when it read them.
	Now int64 `json:"now"`
	// Enabled is false when the Time Tracking plugin is off in the client:
	// then there are no timers.
	Enabled bool    `json:"enabled"`
	Timers  []Timer `json:"timers"`
}

// Timer is one clock of the Time Tracking panel.
type Timer struct {
	Key   string `json:"key"`   // HERB, TREE, BIRD_HOUSE, FARMING_CONTRACT, ...
	Label string `json:"label"` // the panel's name ("Herbs", "Bird Houses")
	// State is the tab's summary: COMPLETED, IN_PROGRESS, OCCUPIED, EMPTY or
	// UNKNOWN (the patches were never seen on this profile).
	State string `json:"state"`
	// ReadyAt is when it is (or was) ready, epoch seconds; only while growing
	// or done.
	ReadyAt *int64 `json:"readyAt,omitempty"`
}

func (e *Executor) timers(ctx context.Context) (Timers, error) {
	if e.Bridge == nil {
		return Timers{}, errors.New("timers: no bridge configured")
	}
	raw, err := e.Bridge.JSON(ctx, http.MethodGet, "/bridge/timers", nil, nil, 10*time.Second)
	if err != nil {
		return Timers{}, fmt.Errorf("timers: %w (the Chronicle Link plugin 1.6+ serves the clocks in the dev client)", err)
	}
	var t Timers
	if err = json.Unmarshal(raw, &t); err != nil {
		return Timers{}, fmt.Errorf("timers: %w", err)
	}
	if t.Timers == nil {
		t.Timers = []Timer{}
	}
	return t, nil
}
