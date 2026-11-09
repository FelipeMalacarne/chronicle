package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// answering is a bridge that answers every request with the same JSON.
type answering string

func (a answering) JSON(context.Context, string, string, url.Values, any, time.Duration) (json.RawMessage, error) {
	return json.RawMessage(a), nil
}

func TestTimers(t *testing.T) {
	ctx := context.Background()
	e := &Executor{Game: &fakeGame{}, Bridge: answering(`{"now":1762720000,"enabled":true,"timers":[
		{"key":"HERB","label":"Herbs","state":"IN_PROGRESS","readyAt":1762721380},
		{"key":"BIRD_HOUSE","label":"Bird Houses","state":"UNKNOWN"}]}`)}
	out, err := e.Execute(ctx, CmdTimers, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := out.(Timers)
	if !ts.Enabled || len(ts.Timers) != 2 || ts.Timers[0].ReadyAt == nil || *ts.Timers[0].ReadyAt != 1762721380 || ts.Timers[1].ReadyAt != nil {
		t.Fatalf("timers = %+v", ts)
	}
	// Time Tracking off: no timers, and never a null list.
	out, err = (&Executor{Game: &fakeGame{}, Bridge: answering(`{"now":1,"enabled":false}`)}).Execute(ctx, CmdTimers, nil)
	if err != nil || out.(Timers).Timers == nil {
		t.Fatalf("timers off = %+v, %v", out, err)
	}
	if _, err = (&Executor{Game: &fakeGame{}, Bridge: failing{}}).Execute(ctx, CmdTimers, nil); err == nil || !strings.Contains(err.Error(), "1.6+") {
		t.Fatalf("old bridge = %v", err)
	}
	if _, err = ValidateParams(CmdTimers, nil); err != nil {
		t.Fatal(err)
	}
}
