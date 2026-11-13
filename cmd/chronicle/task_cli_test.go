package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/plan"
	"github.com/FelipeMalacarne/chronicle/internal/core/rules"
)

// taskPlanner serves the plan's next task and a run that ends at once;
// the proof comes on the second status read after the run ended.
type taskPlanner struct {
	mu      sync.Mutex
	posted  []map[string]any
	reads   int
	cancels int
}

func (p *taskPlanner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	loc := entities.MustParseRef("location:lumbridge")
	task := plan.TaskView{
		Key: "step:oziris-main::1.1.3#0", Item: "step:oziris-main::1.1.3", Type: "travel", Label: "Ir até Lumbridge",
		Until: "na área do local", State: rules.Unknown, Reason: "sem posição: ligue Send position", Risks: []string{},
		Executable: true, Task: content.Task{Type: content.TaskTravel, Location: &loc},
	}
	now := time.Now().UTC()
	run := &agent.Run{ID: "run-7", UID: "0123456789abcdef", ClassName: agent.ChronicleLinkClass, State: agent.RunDone, StartedAt: now, FinishedAt: &now,
		Task:   &agent.RunTask{Key: task.Key, Plan: "route:oziris-main", Type: "travel", Label: task.Label},
		Events: []agent.RunEvent{{At: now, State: agent.RunRunning, Text: "andando até Lumbridge"}}}
	switch {
	case r.URL.Path == "/api/me":
		_ = json.NewEncoder(w).Encode(client.Me{AuthMode: "dev", Accounts: []client.Account{{ID: "acc_x", RSN: "X"}}})
	case r.URL.Path == "/api/plan/next":
		_ = json.NewEncoder(w).Encode(client.PlanNext{Account: "acc_x", Result: plan.Result{
			Plan: plan.Ref{ID: "route:oziris-main", Kind: plan.KindRoute, Title: "Oziris"}, Status: plan.StatusTask,
			Item: &plan.ItemView{Kind: plan.ItemStep, Key: task.Item, Ref: "1.1.3", Title: "Walk to Lumbridge", Attribution: "Guia por OzirisLoL · ironman.guide"},
			Task: &task, Skipped: []plan.TaskView{},
		}})
	case r.URL.Path == "/api/microbot/task" && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		p.posted = append(p.posted, body)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": agent.RunStatus{Run: run}, "task": task})
	case r.URL.Path == "/api/microbot/task":
		p.reads++
		proof := task
		st := client.TaskStatus{Run: run, Proof: &proof}
		if p.reads > 1 {
			st.Proof.State, st.Proof.Reason, st.Proven = rules.Met, "", true
		}
		_ = json.NewEncoder(w).Encode(st)
	case r.URL.Path == "/api/microbot/run/cancel":
		p.cancels++
		_, _ = w.Write([]byte(`{"ok":true}`))
	default:
		http.NotFound(w, r)
	}
}

func TestTaskCLI(t *testing.T) {
	p := &taskPlanner{}
	srv := httptest.NewServer(p)
	defer srv.Close()

	out, err := runCLI(t, "task", "--url", srv.URL)
	if err != nil || !strings.Contains(out, "→ Ir até Lumbridge") || !strings.Contains(out, "OzirisLoL") || !strings.Contains(out, "task run --key") {
		t.Fatalf("next = %q %v", out, err)
	}
	// It acts in game: never without --yes.
	if out, err = runCLI(t, "task", "run", "--url", srv.URL); err == nil || !strings.Contains(err.Error(), "--yes") || len(p.posted) != 0 {
		t.Fatalf("run without --yes = %q %v (%d posts)", out, err, len(p.posted))
	}
	out, err = runCLI(t, "task", "run", "--url", srv.URL, "--yes", "--wait", "10s")
	if err != nil || !strings.Contains(out, "run-7 começou") || !strings.Contains(out, "andando até Lumbridge") || !strings.Contains(out, "provado pelo ingest") {
		t.Fatalf("run = %q %v", out, err)
	}
	p.mu.Lock()
	if len(p.posted) != 1 || p.posted[0]["key"] != "step:oziris-main::1.1.3#0" || p.posted[0]["supervised"] != false {
		t.Errorf("posted = %v", p.posted)
	}
	p.mu.Unlock()
	if out, err = runCLI(t, "task", "cancel", "--url", srv.URL); err != nil || !strings.Contains(out, "cancelado") || p.cancels != 1 {
		t.Fatalf("cancel = %q %v", out, err)
	}
	if out, err = runCLI(t, "task", "status", "--url", srv.URL); err != nil || !strings.Contains(out, "Prova:") {
		t.Fatalf("status = %q %v", out, err)
	}
}
