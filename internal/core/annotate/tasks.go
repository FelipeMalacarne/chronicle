package annotate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording/propose"
)

// Tasks of a step (docs/10 sections 7 and 11, phase 2): the agent proposes
// them from the step's text (a quote) or from a recorded demonstration
// (propose_tasks), the owner confirms what is inferred, and submit or
// submit_tasks resolve the names and verify them with the loader's checks.

// NameRef names a catalog entry for a task parameter, as resolve_entity
// takes it: Name (for an npc, shop or object the wiki page title picks one
// page) and ID to choose among the candidates of an ambiguous name. Kind is
// only for via: item or unlock.
type NameRef struct {
	Kind string `json:"kind,omitempty"`
	Name string `json:"name"`
	ID   string `json:"id,omitempty"`
}

// TaskItemProposal is an item of a bank task, resolved like an item
// requirement (ItemProposal).
type TaskItemProposal struct {
	Name     string   `json:"name"`
	ID       string   `json:"id,omitempty"`
	AnyOf    []string `json:"any_of,omitempty"`
	Family   string   `json:"family,omitempty"`
	Qty      int      `json:"qty,omitempty"`
	Noted    bool     `json:"noted,omitempty"`
	Optional bool     `json:"optional,omitempty"`
}

// UntilProposal is a task's termination, like content.TaskUntil: a state
// predicate (at_location, loadout_ready, goal_met, owner_mark) or an
// objective target named like an objective proposal's.
type UntilProposal struct {
	State     string          `json:"state,omitempty"`
	Objective *TargetProposal `json:"objective,omitempty"`
}

// RecordedRef points at the events of a saved recording that support a
// task: the file `chronicle recording -o` wrote, the recording's id and the
// events' seqs. Submit copies the events from the file into the sidecar; the
// agent never writes events itself.
type RecordedRef struct {
	File      string  `json:"file"`
	Recording string  `json:"recording"`
	Seqs      []int64 `json:"seqs"`
}

// TaskProposal is one task the agent proposes for a step: the type, names
// for the refs (never refs), the game's text (options, action), numbers,
// the termination, and the evidence: a quote of the step (Field, default
// /text, and Quote), the recorded events (Recorded), or both. A task from a
// recording always needs the owner (confirmed_by owner).
type TaskProposal struct {
	Type     string             `json:"type"`
	Location *NameRef           `json:"location,omitempty"`
	Via      *NameRef           `json:"via,omitempty"`
	Items    []TaskItemProposal `json:"items,omitempty"`
	Shop     *NameRef           `json:"shop,omitempty"`
	Item     *NameRef           `json:"item,omitempty"`
	Qty      int                `json:"qty,omitempty"`
	NPC      *NameRef           `json:"npc,omitempty"`
	Options  []string           `json:"options,omitempty"`
	Object   *NameRef           `json:"object,omitempty"`
	Action   string             `json:"action,omitempty"`
	Skill    *NameRef           `json:"skill,omitempty"`
	Note     string             `json:"note,omitempty"`
	Until    *UntilProposal     `json:"until,omitempty"`
	Field    string             `json:"field,omitempty"`
	Quote    string             `json:"quote,omitempty"`
	Recorded *RecordedRef       `json:"recorded,omitempty"`
	Source
}

// RecordingFile is a saved recording: the answer of GET
// /api/account/recording as `chronicle recording -o` writes it.
type RecordingFile struct {
	Account   string            `json:"account"`
	Since     time.Time         `json:"since"`
	Until     time.Time         `json:"until"`
	Events    []recording.Event `json:"events"`
	Truncated bool              `json:"truncated"`
}

// recordingFiles reads each recording file once per call.
type recordingFiles map[string]*RecordingFile

func (rf recordingFiles) read(path string) (*RecordingFile, error) {
	path = filepath.Clean(path)
	if f, ok := rf[path]; ok {
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("recording file: %w", err)
	}
	var f RecordingFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("recording file %s: %w", path, err)
	}
	if msgs := recording.Validate("events", f.Events, time.Time{}); len(msgs) > 0 {
		if len(msgs) > 3 {
			msgs = append(msgs[:3], fmt.Sprintf("and %d more", len(msgs)-3))
		}
		return nil, fmt.Errorf("recording file %s: %s", path, strings.Join(msgs, "; "))
	}
	slices.SortStableFunc(f.Events, recording.Compare)
	rf[path] = &f
	return &f, nil
}

// evidence copies the events a RecordedRef names out of its file.
func (rf recordingFiles) evidence(ref RecordedRef) (content.RecordedEvidence, error) {
	f, err := rf.read(ref.File)
	if err != nil {
		return content.RecordedEvidence{}, err
	}
	if len(ref.Seqs) == 0 {
		return content.RecordedEvidence{}, fmt.Errorf("recorded: name the events by their seqs")
	}
	out := content.RecordedEvidence{Recording: ref.Recording}
	for _, seq := range ref.Seqs {
		i := slices.IndexFunc(f.Events, func(e recording.Event) bool { return e.Recording == ref.Recording && e.Seq == seq })
		if i < 0 {
			return content.RecordedEvidence{}, fmt.Errorf("recorded: %s has no event %s#%d", ref.File, ref.Recording, seq)
		}
		if !slices.ContainsFunc(out.Events, func(e recording.Event) bool { return e.Seq == seq }) {
			out.Events = append(out.Events, f.Events[i])
		}
	}
	slices.SortStableFunc(out.Events, recording.Compare)
	return out, nil
}

// buildTasks resolves task proposals into annotated tasks; every problem is
// returned, prefixed with the task's place.
func buildTasks(reg *entities.Registry, st *content.Step, tps []TaskProposal, skillVersion string, files recordingFiles) ([]content.AnnotatedTask, []string) {
	out := []content.AnnotatedTask{}
	var errs []string
	for i, tp := range tps {
		t, terrs := buildTask(reg, st, tp, skillVersion, files)
		for _, e := range terrs {
			errs = append(errs, fmt.Sprintf("tasks[%d]: %s", i, e))
		}
		out = append(out, t)
	}
	return out, errs
}

func buildTask(reg *entities.Registry, st *content.Step, tp TaskProposal, skillVersion string, files recordingFiles) (content.AnnotatedTask, []string) {
	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	t := content.Task{Type: tp.Type, Qty: tp.Qty, Options: tp.Options, Action: tp.Action, Note: strings.TrimSpace(tp.Note)}
	resolveRef := func(param string, nr *NameRef, kind entities.Kind) *entities.Ref {
		if nr == nil {
			return nil
		}
		if param == "via" {
			switch nr.Kind {
			case string(entities.KindItem), string(entities.KindUnlock):
				kind = entities.Kind(nr.Kind)
			default:
				fail("via: kind must be item or unlock, got %q", nr.Kind)
				return nil
			}
		}
		res, noted := resolveName(reg, string(kind), nr.Name, "")
		if noted {
			fail("%s: a task names the item, not its noted form: drop %q", param, "noted")
			return nil
		}
		ref, err := pick(reg, res, string(kind), nr.Name, nr.ID)
		if err != nil {
			fail("%s: %v", param, err)
			return nil
		}
		return &ref
	}
	t.Location = resolveRef("location", tp.Location, entities.KindLocation)
	t.Via = resolveRef("via", tp.Via, "")
	t.Shop = resolveRef("shop", tp.Shop, entities.KindShop)
	t.Item = resolveRef("item", tp.Item, entities.KindItem)
	t.NPC = resolveRef("npc", tp.NPC, entities.KindNPC)
	t.Object = resolveRef("object", tp.Object, entities.KindObject)
	t.Skill = resolveRef("skill", tp.Skill, entities.KindSkill)
	for i, it := range tp.Items {
		req, err := itemRequirement(reg, ItemProposal{Name: it.Name, ID: it.ID, AnyOf: it.AnyOf, Family: it.Family, Qty: it.Qty, Noted: it.Noted, Optional: it.Optional})
		if err != nil {
			fail("items[%d]: %v", i, err)
			continue
		}
		t.Items = append(t.Items, req)
	}
	if u := tp.Until; u != nil {
		switch {
		case (u.State == "") == (u.Objective == nil):
			fail("until: a state or an objective, exactly one")
		case u.State != "":
			t.Until = &content.TaskUntil{State: u.State}
		default:
			k, err := objectiveKey(reg, *u.Objective)
			if err != nil {
				fail("until: %v", err)
			} else {
				t.Until = &content.TaskUntil{Objective: &k}
			}
		}
	}

	var ev content.TaskEvidence
	switch {
	case strings.TrimSpace(tp.Quote) != "":
		field := tp.Field
		if field == "" {
			field = "/text"
		}
		span, err := content.QuoteSpan(st, field, tp.Quote)
		if err != nil {
			fail("%v", err)
		}
		ev.Field, ev.Quote, ev.Span = field, tp.Quote, span
	case tp.Field != "":
		fail("field %s without a quote", tp.Field)
	}
	if tp.Recorded != nil {
		rec, err := files.evidence(*tp.Recorded)
		if err != nil {
			fail("%v", err)
		} else {
			ev.Recorded = &rec
		}
	}
	if ev.Quote == "" && tp.Recorded == nil {
		fail("a task needs a quote of the step or the recorded events it came from")
	}
	switch tp.Confidence {
	case content.ConfidenceHigh, content.ConfidenceMedium:
	default:
		fail("confidence must be high (the text says it) or medium (inferred, or from a recording), got %q", tp.Confidence)
	}
	if tp.ConfirmedBy != "" && tp.ConfirmedBy != content.ConfirmedByOwner {
		fail("confirmed_by can only be %q", content.ConfirmedByOwner)
	}
	src := content.ObjectiveSource{Method: content.ProvenanceLLM, Confidence: tp.Confidence, SkillVersion: skillVersion,
		ConfirmedBy: tp.ConfirmedBy, Decision: tp.Decision}
	if tp.Recorded != nil {
		src.Method = content.ProvenanceRecording
	}
	if tp.Decision != "" {
		src.ConfirmedBy = content.ConfirmedByOwner
	}
	return content.AnnotatedTask{Task: t, Evidence: ev, Source: src}, errs
}

// describeTask is a task in short form, for results and the report.
func describeTask(t content.Task) string {
	parts := []string{t.Type}
	for _, r := range []*entities.Ref{t.Location, t.NPC, t.Object, t.Shop, t.Item, t.Skill} {
		if r != nil {
			parts = append(parts, r.String())
		}
	}
	if t.Qty > 0 {
		parts = append(parts, fmt.Sprintf("x%d", t.Qty))
	}
	if t.Via != nil {
		parts = append(parts, "via "+t.Via.String())
	}
	if t.Action != "" {
		parts = append(parts, fmt.Sprintf("%q", t.Action))
	}
	if len(t.Options) > 0 {
		parts = append(parts, fmt.Sprintf("options %q", t.Options))
	}
	for _, it := range t.Items {
		parts = append(parts, describeItem(it))
	}
	if t.Note != "" {
		parts = append(parts, fmt.Sprintf("note %q", t.Note))
	}
	if u, ok := t.Ends(); ok {
		parts = append(parts, "until "+u.String())
	}
	return strings.Join(parts, " ")
}

// StepTasksProposal is the tasks of one step for submit_tasks: they replace
// the step's tasks and keep everything else of its annotation.
type StepTasksProposal struct {
	StepID  string         `json:"step_id"`
	Tasks   []TaskProposal `json:"tasks"`
	Comment string         `json:"comment,omitempty"`
}

// SubmitTasks verifies the tasks of steps of one section and writes them
// into their annotations, keeping the objectives, items, bank actions and
// location as they are. A step needs a current annotation first (submit):
// its tasks lean on what it completes. Each step is accepted whole or
// rejected with every reason, like Submit.
func (w *Workspace) SubmitTasks(guideID, sectionID, skillVersion string, steps []StepTasksProposal) (*SubmitResult, error) {
	gs, sec, err := w.section(guideID, sectionID)
	if err != nil {
		return nil, err
	}
	if skillVersion == "" {
		return nil, fmt.Errorf("skill_version is required (the annotate-guide skill's version, e.g. annotate-guide@3)")
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps to submit")
	}
	res := &SubmitResult{GuideID: guideID, SectionID: sec.ID, Accepted: []AcceptedStep{}, Rejected: []RejectedStep{}}
	candidates := map[string]content.StepAnnotation{}
	reasons := map[string][]string{}
	var order []string
	files := recordingFiles{}
	for _, p := range steps {
		if _, dup := reasons[p.StepID]; dup {
			return nil, fmt.Errorf("step %q is submitted twice", p.StepID)
		}
		reasons[p.StepID] = nil
		order = append(order, p.StepID)
		st, ok := gs.guide.Step(p.StepID)
		switch {
		case !ok:
			reasons[p.StepID] = []string{fmt.Sprintf("unknown step %q", p.StepID)}
			continue
		case st.SectionID != sec.ID:
			reasons[p.StepID] = []string{fmt.Sprintf("step %q belongs to section %q, not %q", p.StepID, st.SectionID, sec.ID)}
			continue
		case !gs.fresh(st):
			reasons[p.StepID] = []string{"the step has no current annotation: submit its objectives first (next_batch, submit)"}
			continue
		}
		sa := *gs.annotation(st)
		tasks, errs := buildTasks(w.reg, st, p.Tasks, skillVersion, files)
		sa.Tasks = tasks
		if len(errs) == 0 {
			issues, _ := gs.guide.CheckStepAnnotation(&sa, gs.decisions, w.reg)
			for _, is := range issues {
				errs = append(errs, is.Pointer+": "+is.Message)
			}
		}
		if len(errs) > 0 {
			reasons[p.StepID] = errs
			continue
		}
		candidates[p.StepID] = sa
	}
	if len(candidates) > 0 {
		file, err := gs.write(sec, skillVersion, candidates)
		if err != nil {
			return nil, err
		}
		res.File = file
	}
	comments := map[string]string{}
	for _, p := range steps {
		comments[p.StepID] = p.Comment
	}
	for _, id := range order {
		if sa, ok := candidates[id]; ok {
			res.Accepted = append(res.Accepted, accepted(sa))
		} else {
			res.Rejected = append(res.Rejected, RejectedStep{StepID: id, Reasons: reasons[id]})
		}
		if _, known := gs.index[id]; known {
			gs.report.log(id, sec.ID, reasons[id], comments[id])
		}
	}
	res.Pending = gs.coverage(sec).Pending + gs.coverage(sec).Stale
	if err := gs.writeReport(w.reg); err != nil {
		return nil, err
	}
	return res, nil
}

// TaskProposals is what propose_tasks answers for one step: the step as the
// tasks see it and, with a recording file, the tasks the recorded stretch
// suggests, each in the form submit_tasks takes.
type TaskProposals struct {
	GuideID string `json:"guide_id"`
	StepID  string `json:"step_id"`
	Text    string `json:"text"`
	// Annotated says the step has a current annotation, which submit_tasks
	// needs.
	Annotated bool `json:"annotated"`
	// Goal are the objectives the step completes: what until goal_met waits
	// for. Without any, a talk or an interact needs an objective in until.
	Goal []string `json:"goal"`
	// Loadout says a bank task without items has the segment's loadout.
	Loadout bool `json:"loadout"`
	// Current are the step's tasks now.
	Current   []string        `json:"current_tasks"`
	Recording *RecordingView  `json:"recording,omitempty"`
	Proposals []ProposedTask  `json:"proposals"`
	Issues    []propose.Issue `json:"issues"`
}

// RecordingView summarizes the stretch read.
type RecordingView struct {
	File   string    `json:"file"`
	Events int       `json:"events"`
	Used   int       `json:"used"`
	From   time.Time `json:"from,omitzero"`
	To     time.Time `json:"to,omitzero"`
}

// ProposedTask is one proposal: Submit is the task as submit_tasks takes
// it, with confidence medium and without confirmed_by, which only the
// owner's answer adds. Problems are what the verifier says of it now.
type ProposedTask struct {
	Summary  string            `json:"summary"`
	Names    map[string]string `json:"names,omitempty"`
	Events   []string          `json:"events"`
	Problems []string          `json:"problems,omitempty"`
	Note     string            `json:"note,omitempty"`
	Submit   TaskProposal      `json:"submit"`
}

// ProposeFilter selects the stretch of a recording file: one recording, a
// seq range of it, or a time range.
type ProposeFilter struct {
	Recording string    `json:"recording,omitempty"`
	FromSeq   int64     `json:"from_seq,omitempty"`
	ToSeq     int64     `json:"to_seq,omitempty"`
	Since     time.Time `json:"since,omitzero"`
	Until     time.Time `json:"until,omitzero"`
}

func (f ProposeFilter) keep(e recording.Event) bool {
	switch {
	case f.Recording != "" && e.Recording != f.Recording:
		return false
	case f.FromSeq > 0 && e.Seq < f.FromSeq, f.ToSeq > 0 && e.Seq > f.ToSeq:
		return false
	case !f.Since.IsZero() && e.At.Before(f.Since), !f.Until.IsZero() && !e.At.Before(f.Until):
		return false
	}
	return true
}

// ProposeTasks shows a step as its tasks see it and, when file is not
// empty, proposes tasks for it from the recorded stretch the filter selects
// (internal/core/recording/propose). Nothing is written: the owner confirms
// the proposals and submit_tasks verifies and writes them.
func (w *Workspace) ProposeTasks(guideID, stepID, file string, filter ProposeFilter) (*TaskProposals, error) {
	gs, err := w.guide(guideID)
	if err != nil {
		return nil, err
	}
	st, ok := gs.guide.Step(stepID)
	if !ok {
		return nil, fmt.Errorf("unknown step %q in %s", stepID, guideID)
	}
	var sa *content.StepAnnotation
	if gs.fresh(st) {
		sa = gs.annotation(st)
	}
	ctx := gs.guide.StepTaskContext(st, sa, w.reg)
	out := &TaskProposals{GuideID: guideID, StepID: st.ID, Text: st.Text, Annotated: sa != nil, Loadout: ctx.Loadout,
		Goal: []string{}, Current: []string{}, Proposals: []ProposedTask{}, Issues: []propose.Issue{}}
	for _, k := range ctx.Goal {
		out.Goal = append(out.Goal, k.String())
	}
	if sa != nil {
		for _, t := range sa.Tasks {
			out.Current = append(out.Current, describeTask(t.Task))
		}
	}
	if file == "" {
		return out, nil
	}
	f, err := recordingFiles{}.read(file)
	if err != nil {
		return nil, err
	}
	var events []recording.Event
	for _, e := range f.Events {
		if filter.keep(e) {
			events = append(events, e)
		}
	}
	got := propose.Propose(propose.Input{Events: events, Context: ctx, Registry: w.reg})
	out.Recording = &RecordingView{File: file, Events: got.Events, Used: got.Used}
	if len(events) > 0 {
		out.Recording.From, out.Recording.To = events[0].At, events[len(events)-1].At
	}
	out.Issues = got.Issues
	for _, p := range got.Tasks {
		out.Proposals = append(out.Proposals, w.proposed(p, file))
	}
	return out, nil
}

// proposed turns a proposal into the form submit_tasks takes: names that
// resolve to the proposal's refs (the wiki page title of an npc, shop or
// object) with the ref as id, and the events by seq.
func (w *Workspace) proposed(p propose.Task, file string) ProposedTask {
	t := p.Task
	nameOf := func(r *entities.Ref) *NameRef {
		if r == nil {
			return nil
		}
		name := r.ID
		if k, ok := w.reg.WikiKeyOf(*r); ok && (r.Kind == entities.KindNPC || r.Kind == entities.KindShop || r.Kind == entities.KindObject) {
			name = string(k)
		} else if d, ok := w.reg.Display(*r); ok {
			name = d
		}
		return &NameRef{Name: name, ID: r.String()}
	}
	tp := TaskProposal{
		Type: t.Type, Location: nameOf(t.Location), Shop: nameOf(t.Shop), Item: nameOf(t.Item), NPC: nameOf(t.NPC),
		Object: nameOf(t.Object), Skill: nameOf(t.Skill), Qty: t.Qty, Options: t.Options, Action: t.Action, Note: t.Note,
		Source: Source{Confidence: p.Confidence},
	}
	if t.Via != nil {
		tp.Via = nameOf(t.Via)
		tp.Via.Kind = string(t.Via.Kind)
	}
	for _, it := range t.Items {
		tip := TaskItemProposal{Name: it.Name, Noted: it.Noted, Optional: it.Optional}
		if it.ItemID != nil {
			tip.ID = fmt.Sprint(*it.ItemID)
			if form, ok := w.reg.Item(*it.ItemID); ok {
				tip.Name = form.Name
			}
		}
		if it.Qty != nil {
			tip.Qty = *it.Qty
		}
		tp.Items = append(tp.Items, tip)
	}
	if u := t.Until; u != nil {
		tp.Until = &UntilProposal{State: u.State}
		if u.Objective != nil {
			// an objective until is never proposed from a recording; the
			// agent fills it with the owner
			tp.Until = nil
		}
	}
	ref := &RecordedRef{File: file, Recording: p.Evidence.Recording}
	var events []string
	for _, e := range p.Evidence.Events {
		ref.Seqs = append(ref.Seqs, e.Seq)
		events = append(events, describeEvent(e))
	}
	tp.Recorded = ref
	return ProposedTask{Summary: describeTask(t), Names: p.Names, Events: events, Problems: p.Problems, Note: p.Note, Submit: tp}
}

// describeEvent is a recorded event in one line, for the owner's question.
func describeEvent(e recording.Event) string {
	s := fmt.Sprintf("#%d %s %s (%d, %d, %d)", e.Seq, e.At.Format(time.TimeOnly), e.Kind, e.X, e.Y, e.Plane)
	for _, part := range []struct {
		label string
		v     *int
	}{{"npc", e.NPC}, {"object", e.Object}, {"item", e.Item}, {"on item", e.TargetItem}, {"x", e.Qty}} {
		if part.v != nil {
			s += fmt.Sprintf(" %s %d", part.label, *part.v)
		}
	}
	if e.Option != "" {
		s += fmt.Sprintf(" %q", e.Option)
	}
	if e.Name != "" {
		s += " (" + e.Name + ")"
	}
	return s
}
