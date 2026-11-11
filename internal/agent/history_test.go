package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunHistoryOutlivesTheAgent(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "agent-runs.json")
	start := `{"className":"` + catSwap + `","stop":["` + fisher + `"]}`
	e := &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond, RunFile: file}
	for range 2 {
		if _, err := e.Execute(ctx, CmdRunStart, json.RawMessage(start)); err != nil {
			t.Fatal(err)
		}
		waitRun(t, e)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("history file: %v %v", fi, err)
	}

	// Another agent reads it: the last run is the current one and ids go on.
	e2 := &Executor{Game: newRunGame("done"), RunPoll: time.Millisecond, RunFile: file}
	out, err := e2.Execute(ctx, CmdRunStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := out.(RunStatus).Run; r == nil || r.ID != "run-2" || r.State != RunDone {
		t.Fatalf("status after restart = %+v", r)
	}
	out, _ = e2.Execute(ctx, CmdRunStart, json.RawMessage(start))
	if id := out.(RunStatus).Run.ID; id != "run-3" {
		t.Fatalf("new run id = %s", id)
	}
	waitRun(t, e2)
	out, _ = e2.Execute(ctx, CmdRunHistory, nil)
	h := out.(RunHistory)
	if len(h.Runs) != 3 || h.Runs[0].ID != "run-3" || h.Runs[2].ID != "run-1" || h.Error != "" {
		t.Fatalf("history = %+v", h)
	}
}

func TestRunHistoryAgentDiedMidRun(t *testing.T) {
	file := filepath.Join(t.TempDir(), "agent-runs.json")
	mid := []Run{{ID: "run-7", ClassName: catSwap, State: RunRunning, StartedAt: time.Now(), Events: []RunEvent{}}}
	if err := writeJSONFile(file, mid); err != nil {
		t.Fatal(err)
	}
	e := &Executor{Game: newRunGame("done"), RunFile: file}
	out, _ := e.Execute(context.Background(), CmdRunHistory, nil)
	h := out.(RunHistory)
	if len(h.Runs) != 1 || h.Runs[0].State != RunFailed || h.Runs[0].FinishedAt == nil || h.Runs[0].Error == "" {
		t.Fatalf("history = %+v", h)
	}
	// It is not "going": the agent takes other commands.
	if e.going() {
		t.Fatal("a dead run still drives the character")
	}
}

func TestRunHistoryKeepsTheLastRuns(t *testing.T) {
	var r runner
	for i := range runKeep + 5 {
		r.retire()
		r.run = &Run{ID: "run-" + string(rune('a'+i%26))}
	}
	if len(r.past) != runKeep-1 {
		t.Fatalf("past = %d runs", len(r.past))
	}
}
