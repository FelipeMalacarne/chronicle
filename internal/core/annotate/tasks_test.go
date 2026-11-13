package annotate

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/content/reference"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

// withLocations puts the real location catalog back in the workspace (the
// test workspace starts with none), for tasks that travel.
func withLocations(t *testing.T, w *Workspace, dir string) {
	t.Helper()
	data, err := fs.ReadFile(reference.FS, "locations.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ReferenceSubdir, "locations.json")
	if err = os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err = os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if err = w.SyncRegistry(); err != nil {
		t.Fatal(err)
	}
}

func stepTasks(t *testing.T, w *Workspace, stepID string) []content.AnnotatedTask {
	t.Helper()
	gs := w.guides["t"]
	st, _ := gs.guide.Step(stepID)
	sa := gs.annotation(st)
	if sa == nil {
		t.Fatalf("step %s has no annotation", stepID)
	}
	return sa.Tasks
}

// writeRecording saves a recording file like `chronicle recording -o`.
func writeRecording(t *testing.T, events []recording.Event) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rec.json")
	data, _ := json.Marshal(RecordingFile{Account: "acc_test", Events: events})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSubmitTasksFromTheText(t *testing.T) {
	w, _ := workspace(t)
	spade := TaskProposal{Type: "buy", Shop: &NameRef{Name: "Lumbridge General Store"}, Item: &NameRef{Name: "Spade"}, Qty: 1, Quote: "Buy a spade", Source: hi}
	tasks := []TaskProposal{spade}
	res := submit(t, w, "1.1", StepProposal{StepID: "a", Tasks: &tasks})
	if len(res.Accepted) != 1 || len(res.Accepted[0].Tasks) != 1 || !strings.HasPrefix(res.Accepted[0].Tasks[0], "buy shop:lumbridge_general_store item:952 x1") {
		t.Fatalf("submit = %+v", res)
	}
	got := stepTasks(t, w, "a")
	if got[0].Evidence.Quote != "Buy a spade" || got[0].Source.Method != content.ProvenanceLLM || got[0].Shop.ID != "lumbridge_general_store" {
		t.Fatalf("task = %+v", got[0])
	}

	// Resubmitting the objectives keeps the tasks; [] removes them.
	submit(t, w, "1.1", StepProposal{StepID: "a", Objectives: []ObjectiveProposal{objective("starts", "quest", "X Marks the Spot", "started", 0, "start X marks the spot quest", hi)}})
	if got = stepTasks(t, w, "a"); len(got) != 1 {
		t.Fatalf("after resubmitting the objectives: %d tasks", len(got))
	}
	none := []TaskProposal{}
	submit(t, w, "1.1", StepProposal{StepID: "a", Tasks: &none})
	if got = stepTasks(t, w, "a"); len(got) != 0 {
		t.Fatalf("after tasks []: %d tasks", len(got))
	}

	// Names resolve like the objectives', and the verifier runs.
	bad := []TaskProposal{
		{Type: "buy", Shop: &NameRef{Name: "Bob's Brilliant Axes."}, Item: &NameRef{Name: "Spade"}, Qty: 1, Quote: "Buy a spade", Source: hi},
		{Type: "talk", NPC: &NameRef{Name: "Cook"}, Quote: "Buy a spade", Source: hi},
		{Type: "manual", Source: hi},
	}
	why := reasons(submit(t, w, "1.1", StepProposal{StepID: "a", Tasks: &bad}), "a")
	for _, want := range []string{"tasks[1]: npc: npc \"Cook\" is ambiguous", "tasks[2]: a task needs a quote"} {
		if !strings.Contains(why, want) {
			t.Errorf("reasons %q lack %q", why, want)
		}
	}
	bad = bad[:1]
	if why = reasons(submit(t, w, "1.1", StepProposal{StepID: "a", Tasks: &bad}), "a"); !strings.Contains(why, "does not sell") {
		t.Errorf("a shop that does not sell the item: %q", why)
	}
}

func TestProposeAndSubmitTasksFromARecording(t *testing.T) {
	w, dir := workspace(t)
	withLocations(t, w, dir)
	at := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	ev := func(seq int64, kind string, edit func(*recording.Event)) recording.Event {
		e := recording.Event{Recording: "rec-test", Seq: seq, At: at.Add(time.Duration(seq) * time.Second), Kind: kind, World: 330, X: 3212, Y: 3246}
		edit(&e)
		return e
	}
	file := writeRecording(t, []recording.Event{
		ev(1, recording.KindNPC, func(e *recording.Event) { e.NPC, e.Option = recording.Int(2813), "Trade" }),
		ev(2, recording.KindShopBuy, func(e *recording.Event) {
			e.Item, e.Qty, e.NPC = recording.Int(952), recording.Int(1), recording.Int(2813)
		}),
		ev(3, recording.KindNPC, func(e *recording.Event) { e.X, e.Y, e.NPC, e.Option = 3222, 3218, recording.Int(8484), "Talk-to" }),
		ev(4, recording.KindDialogue, func(e *recording.Event) {
			e.X, e.Y, e.Option, e.NPC = 3222, 3218, "I'm looking for a quest.", recording.Int(8484)
		}),
	})

	// Without the step's objectives annotated, submit_tasks refuses it.
	got, err := w.ProposeTasks("t", "a", file, ProposeFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Annotated || len(got.Goal) != 1 || got.Goal[0] != "quest:x_marks_the_spot@started" {
		t.Fatalf("step view = %+v", got)
	}
	if len(got.Proposals) != 3 {
		t.Fatalf("proposals = %+v\nissues %+v", got.Proposals, got.Issues)
	}
	buy, travel, talk := got.Proposals[0], got.Proposals[1], got.Proposals[2]
	if buy.Submit.Type != "buy" || buy.Submit.Shop.ID != "shop:lumbridge_general_store" || buy.Submit.Item.ID != "item:952" ||
		buy.Submit.Recorded == nil || len(buy.Submit.Recorded.Seqs) != 2 || buy.Submit.Confidence != content.ConfidenceMedium || buy.Submit.ConfirmedBy != "" {
		t.Fatalf("buy = %+v", buy)
	}
	if travel.Submit.Type != "travel" || travel.Submit.Location.ID != "location:lumbridge_castle" {
		t.Fatalf("travel = %+v", travel)
	}
	if talk.Submit.NPC.Name != "Veos" || len(talk.Submit.Options) != 1 || talk.Submit.Until.State != content.UntilGoalMet || len(talk.Problems) != 0 {
		t.Fatalf("talk = %+v", talk)
	}
	if len(got.Issues) != 1 || !strings.Contains(got.Issues[0].Message, "(3212, 3246, 0) is in no location's area") {
		t.Fatalf("issues = %+v", got.Issues)
	}
	proposals := []TaskProposal{buy.Submit, travel.Submit, talk.Submit}
	res, err := w.SubmitTasks("t", "1.1", "annotate-guide@3", []StepTasksProposal{{StepID: "a", Tasks: proposals}})
	if err != nil {
		t.Fatal(err)
	}
	if why := reasons(res, "a"); !strings.Contains(why, "no current annotation") {
		t.Fatalf("not annotated: %q", why)
	}
	submit(t, w, "1.1", StepProposal{StepID: "a", Objectives: []ObjectiveProposal{objective("starts", "quest", "X Marks the Spot", "started", 0, "start X marks the spot quest", hi)}})

	// A task from a recording is the owner's call.
	res, _ = w.SubmitTasks("t", "1.1", "annotate-guide@3", []StepTasksProposal{{StepID: "a", Tasks: proposals}})
	if why := reasons(res, "a"); !strings.Contains(why, "a task from a recording needs confirmed_by owner") {
		t.Fatalf("without the owner: %q", why)
	}
	for i := range proposals {
		proposals[i].ConfirmedBy = content.ConfirmedByOwner
	}
	res, err = w.SubmitTasks("t", "1.1", "annotate-guide@3", []StepTasksProposal{{StepID: "a", Tasks: proposals}})
	if err != nil || len(res.Accepted) != 1 {
		t.Fatalf("confirmed = %+v, %v", res, err)
	}
	tasks := stepTasks(t, w, "a")
	if len(tasks) != 3 || tasks[0].Source.Method != content.ProvenanceRecording || tasks[0].Evidence.Recorded == nil ||
		len(tasks[0].Evidence.Recorded.Events) != 2 || tasks[0].Evidence.Quote != "" {
		t.Fatalf("tasks = %+v", tasks)
	}
	gs := w.guides["t"]
	st, _ := gs.guide.Step("a")
	if sa := gs.annotation(st); len(sa.Objectives) != 1 {
		t.Fatalf("submit_tasks kept the objectives? %+v", sa.Objectives)
	}
	// The sidecar loads as content: the loader verifies the same way.
	if issues, _ := gs.guide.CheckStepAnnotation(gs.annotation(st), gs.decisions, w.reg); len(issues) != 0 {
		t.Fatalf("loader checks: %v", issues)
	}

	// The events come from the file by seq; one that is not there fails.
	missing := talk.Submit
	missing.ConfirmedBy = content.ConfirmedByOwner
	missing.Recorded = &RecordedRef{File: file, Recording: "rec-test", Seqs: []int64{3, 99}}
	res, _ = w.SubmitTasks("t", "1.1", "annotate-guide@3", []StepTasksProposal{{StepID: "a", Tasks: []TaskProposal{missing}}})
	if why := reasons(res, "a"); !strings.Contains(why, "has no event rec-test#99") {
		t.Fatalf("missing event: %q", why)
	}

	// A filter selects the stretch.
	only, err := w.ProposeTasks("t", "a", file, ProposeFilter{FromSeq: 3})
	if err != nil || len(only.Proposals) != 2 || only.Recording.Events != 2 || len(only.Current) != 3 {
		t.Fatalf("filtered = %+v, %v", only, err)
	}
}
