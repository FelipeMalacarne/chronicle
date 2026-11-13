// Package propose turns a recorded stretch of a demonstration into proposed
// tasks for one route step (docs/10 section 11, phase 2). It is pure and
// deterministic: the events, the step's task context and the catalogs come
// in; typed tasks with catalog refs, the events behind each one and what
// could not be resolved come out. Every proposal is an inference (confidence
// medium): it enters content only through the verifier with the owner's
// confirmation (CLAUDE.md rule 9). An id that maps to several catalog
// entries, or none, is reported, never guessed.
package propose

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

// TeleportTiles is how far a move after an item option must land for the
// item to count as the travel's teleport (via).
const TeleportTiles = 30

// Input is what Propose reads.
type Input struct {
	// Events are the recorded stretch; Propose orders them.
	Events []recording.Event
	// Context is the step's task context (content.Guide.StepTaskContext):
	// the objectives it completes, for the talk and interact termination,
	// and whether a bank task without items has a loadout.
	Context  content.TaskContext
	Registry *entities.Registry
}

// Proposal is what a stretch suggests for a step.
type Proposal struct {
	Tasks  []Task  `json:"tasks"`
	Issues []Issue `json:"issues"`
	// Events and Used count the stretch's events and those behind a task.
	Events int `json:"events"`
	Used   int `json:"used"`
}

// Task is one proposed task: the typed task, the names of its refs, the
// events behind it (the evidence to submit) and what the verifier says of
// it now. Problems is empty when it would pass as it is; otherwise the owner
// or the agent fixes it (an until to choose, a parameter to add) before
// submitting.
type Task struct {
	Task       content.Task             `json:"task"`
	Names      map[string]string        `json:"names,omitempty"`
	Confidence string                   `json:"confidence"`
	Evidence   content.RecordedEvidence `json:"evidence"`
	Problems   []string                 `json:"problems,omitempty"`
	Note       string                   `json:"note,omitempty"`
}

// Issue kinds.
const (
	// IssueAmbiguous: a game id is on several catalog pages (or a tile in
	// several location areas); the candidates are listed.
	IssueAmbiguous = "ambiguous"
	// IssueUnresolved: a game id or a tile is in no catalog entry.
	IssueUnresolved = "unresolved"
	// IssueUnmapped: an event no task type covers yet.
	IssueUnmapped = "unmapped"
)

// Issue is what could not become a task, with the events concerned.
type Issue struct {
	Kind       string   `json:"kind"`
	Message    string   `json:"message"`
	Seqs       []string `json:"seqs"`
	Candidates []string `json:"candidates,omitempty"`
}

// proposer walks the events once.
type proposer struct {
	reg *entities.Registry
	ctx content.TaskContext
	out Proposal
	// used marks the events behind a task.
	used map[string]bool
	// lastTravel is the location of the last travel proposed.
	lastTravel *entities.Ref
	// since are the moves and item options after the last task: a travel's
	// evidence and its possible teleport.
	since []recording.Event
	// opener is the booth, chest or banker clicked right before the bank
	// opened: part of the bank visit's evidence.
	opener *recording.Event
}

// Propose turns the events into tasks, in the order they happened.
func Propose(in Input) Proposal {
	events := slices.Clone(in.Events)
	slices.SortStableFunc(events, recording.Compare)
	p := &proposer{reg: in.Registry, ctx: in.Context, used: map[string]bool{}, out: Proposal{Tasks: []Task{}, Issues: []Issue{}, Events: len(events)}}
	for i := 0; i < len(events); i++ {
		e := events[i]
		switch e.Kind {
		case recording.KindMove, recording.KindItemOp:
			p.since = append(p.since, e)
		case recording.KindNPC:
			i = p.npc(events, i)
		case recording.KindObject:
			if bankNext(events, i) {
				// a booth or chest opened the bank: the bank visit is the task
				p.opener = &events[i]
				continue
			}
			p.travelTo(e)
			p.interact(e)
		case recording.KindBankOpen, recording.KindBankWithdraw, recording.KindBankDeposit:
			i = p.bank(events, i)
		case recording.KindShopBuy:
			i = p.buys(events, i, e.NPC)
		case recording.KindItemUse:
			p.travelTo(e)
			p.itemUse(e)
		case recording.KindDialogue:
			p.issue(IssueUnmapped, "a dialogue option without an NPC talked to before it", nil, e)
		}
	}
	p.travelTo(recording.Event{}) // a stretch that ends somewhere new
	p.out.Used = len(p.used)
	return p.out
}

// bankNext reports whether the bank opens right after event i (the next
// event that is not a move).
func bankNext(events []recording.Event, i int) bool {
	for _, e := range events[i+1:] {
		if e.Kind == recording.KindMove {
			continue
		}
		return e.Kind == recording.KindBankOpen
	}
	return false
}

func key(e recording.Event) string { return fmt.Sprintf("%s#%d", e.Recording, e.Seq) }

// travelTo proposes a travel to where the next interaction e happens, when
// that is a new place. The zero event closes the stretch: its last move.
func (p *proposer) travelTo(e recording.Event) {
	at := e
	if e.Kind == "" {
		if len(p.since) == 0 {
			return
		}
		at = p.since[len(p.since)-1]
		if at.Kind != recording.KindMove {
			return
		}
	}
	since := p.since
	p.since = nil
	locs := p.reg.LocationsAt(at.X, at.Y, at.Plane)
	switch {
	case len(locs) == 0:
		if e.Kind == "" {
			p.issue(IssueUnresolved, fmt.Sprintf("the stretch ends at (%d, %d, %d), in no location's area", at.X, at.Y, at.Plane), nil, at)
		} else {
			p.issue(IssueUnresolved, fmt.Sprintf("(%d, %d, %d) is in no location's area: no travel proposed (a location with a wiki map would make one)", at.X, at.Y, at.Plane), nil, at)
		}
		return
	case len(locs) > 1:
		p.issue(IssueAmbiguous, fmt.Sprintf("(%d, %d, %d) is in the area of %d locations: choose the travel's destination", at.X, at.Y, at.Plane, len(locs)), locs, at)
		return
	}
	loc := locs[0]
	if p.lastTravel != nil && *p.lastTravel == loc {
		return
	}
	p.lastTravel = &loc
	area, _ := p.reg.LocationArea(loc.ID)
	var evidence []recording.Event
	var via *entities.Ref
	var viaName string
	for i, s := range since {
		if s.Kind == recording.KindMove && area.Contains(s.X, s.Y, s.Plane) {
			evidence = append(evidence, s)
		}
		if s.Kind == recording.KindItemOp && s.Item != nil && teleported(s, since[i+1:], at) {
			if form, ok := p.reg.Item(*s.Item); ok {
				r := entities.ItemRef(form.ID)
				via, viaName = &r, form.Name
				evidence = append(evidence, s)
			}
		}
	}
	if !slices.ContainsFunc(evidence, func(x recording.Event) bool { return area.Contains(x.X, x.Y, x.Plane) }) && e.Kind != "" {
		evidence = append(evidence, e)
	}
	slices.SortStableFunc(evidence, recording.Compare)
	t := content.Task{Type: content.TaskTravel, Location: &loc, Via: via}
	names := p.names(map[string]*entities.Ref{"location": &loc})
	if via != nil {
		names["via"] = viaName
	}
	p.add(t, names, evidence, "")
}

// teleported reports whether the character landed far from where it used
// the item: the next position after it (a move or the interaction) is
// TeleportTiles away or on another plane or world.
func teleported(op recording.Event, after []recording.Event, interaction recording.Event) bool {
	next := interaction
	for _, a := range after {
		if a.Kind == recording.KindMove {
			next = a
			break
		}
	}
	if next.Kind == "" {
		return false
	}
	return next.World != op.World || next.Plane != op.Plane ||
		max(abs(next.X-op.X), abs(next.Y-op.Y)) >= TeleportTiles
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// npc handles an NPC click at i: a talk (with the dialogue options that
// follow), a trade (the purchases that follow) or a banker (the bank visit).
// It returns the index of the last event it consumed.
func (p *proposer) npc(events []recording.Event, i int) int {
	e := events[i]
	switch {
	case e.Option == "Talk-to":
		p.travelTo(e)
		ref, ok := p.one(entities.KindNPC, *e.NPC, e)
		evidence := []recording.Event{e}
		var options []string
		// The dialogue follows the click, maybe after a walk to the NPC;
		// moves after the last option are the main loop's.
		last := run(events, i, func(d recording.Event) bool {
			return d.Kind == recording.KindDialogue && (d.NPC == nil || *d.NPC == *e.NPC)
		})
		for _, d := range events[i+1 : last+1] {
			if d.Kind == recording.KindDialogue {
				options = append(options, d.Option)
				evidence = append(evidence, d)
			}
		}
		if ok {
			t := content.Task{Type: content.TaskTalk, NPC: &ref, Options: options}
			p.add(t, p.names(map[string]*entities.Ref{"npc": &ref}), evidence, "")
		}
		return last
	case e.Option == "Trade":
		p.travelTo(e)
		j := i + 1
		for j < len(events) && events[j].Kind == recording.KindMove {
			j++
		}
		if j < len(events) && events[j].Kind == recording.KindShopBuy {
			return p.buys(events, j, e.NPC, e)
		}
		p.issue(IssueUnmapped, "a trade with no purchase after it", nil, e)
		return i
	case bankNext(events, i):
		// a banker opened the bank: the bank visit is the task
		p.opener = &events[i]
		return i
	}
	p.travelTo(e)
	p.issue(IssueUnmapped, fmt.Sprintf("%q on an NPC has no task type (talk is Talk-to, buy is Trade)", e.Option), nil, e)
	return i
}

// buys groups the purchases from i on (one shop visit) into one buy per
// item. shopkeeper is the NPC traded with, when known; trade its event.
func (p *proposer) buys(events []recording.Event, i int, shopkeeper *int, trade ...recording.Event) int {
	if len(trade) == 0 {
		p.travelTo(events[i])
	}
	type line struct {
		item   int
		qty    int
		events []recording.Event
	}
	var lines []*line
	last := run(events, i-1, func(e recording.Event) bool { return e.Kind == recording.KindShopBuy })
	for _, e := range events[i : last+1] {
		if e.Kind != recording.KindShopBuy {
			continue
		}
		if shopkeeper == nil {
			shopkeeper = e.NPC
		}
		item := *e.Item
		if form, ok := p.reg.Item(item); ok {
			item = form.BaseID
		}
		k := slices.IndexFunc(lines, func(l *line) bool { return l.item == item })
		if k < 0 {
			lines = append(lines, &line{item: item})
			k = len(lines) - 1
		}
		lines[k].qty += *e.Qty
		lines[k].events = append(lines[k].events, e)
	}
	for _, l := range lines {
		evidence := append(slices.Clone(trade), l.events...)
		item := entities.ItemRef(l.item)
		shop, ok := p.shopFor(shopkeeper, l.item, evidence)
		if !ok {
			continue
		}
		t := content.Task{Type: content.TaskBuy, Shop: &shop, Item: &item, Qty: l.qty}
		p.add(t, p.names(map[string]*entities.Ref{"shop": &shop, "item": &item}), evidence,
			"qty is what was bought; the task ends when the account owns that many (until item:<id>x<qty>)")
	}
	return last
}

// shopFor finds the shop of a purchase: among the shops the shopkeeper's
// pages own (all of them when the id is on several pages), those that sell
// the item. One is the shop; several or none are reported.
func (p *proposer) shopFor(shopkeeper *int, item int, evidence []recording.Event) (entities.Ref, bool) {
	if shopkeeper == nil {
		p.issue(IssueUnresolved, fmt.Sprintf("a purchase of item %d with no shopkeeper recorded: name the shop", item), nil, evidence...)
		return entities.Ref{}, false
	}
	var shops []entities.Ref
	for _, npc := range p.reg.NPCsByGameID(*shopkeeper) {
		for _, s := range p.reg.ShopsOwnedBy(npc.ID) {
			if _, sells := p.reg.ShopStockOf(s.ID, item); sells && !slices.Contains(shops, s) {
				shops = append(shops, s)
			}
		}
	}
	switch len(shops) {
	case 1:
		return shops[0], true
	case 0:
		p.issue(IssueUnresolved, fmt.Sprintf("no shop of npc %d sells item %d in the catalog: name the shop", *shopkeeper, item), nil, evidence...)
	default:
		p.issue(IssueAmbiguous, fmt.Sprintf("several shops of npc %d sell item %d", *shopkeeper, item), shops, evidence...)
	}
	return entities.Ref{}, false
}

// bank groups a bank visit from i on (the opening, the withdrawals and the
// deposits) into one bank task whose items are what was withdrawn.
func (p *proposer) bank(events []recording.Event, i int) int {
	var evidence []recording.Event
	if p.opener != nil {
		p.travelTo(*p.opener)
		evidence = append(evidence, *p.opener)
		p.opener = nil
	} else {
		p.travelTo(events[i])
	}
	opened := false
	type line struct {
		id    int
		qty   int
		noted bool
	}
	var lines []*line
	last := run(events, i-1, func(e recording.Event) bool {
		switch e.Kind {
		case recording.KindBankOpen:
			// a second opening is the next visit
			was := opened
			opened = true
			return !was
		case recording.KindBankWithdraw, recording.KindBankDeposit:
			opened = true
			return true
		}
		return false
	})
	for _, e := range events[i : last+1] {
		if e.Kind == recording.KindMove {
			continue
		}
		evidence = append(evidence, e)
		if e.Kind != recording.KindBankWithdraw {
			continue
		}
		id, noted := *e.Item, false
		if form, ok := p.reg.Item(id); ok {
			id, noted = form.BaseID, form.Noted
		}
		k := slices.IndexFunc(lines, func(l *line) bool { return l.id == id && l.noted == noted })
		if k < 0 {
			lines = append(lines, &line{id: id, noted: noted})
			k = len(lines) - 1
		}
		lines[k].qty += *e.Qty
	}
	// A deposit of what was withdrawn in the same visit cancels it.
	for _, e := range evidence {
		if e.Kind != recording.KindBankDeposit {
			continue
		}
		id, noted := *e.Item, false
		if form, ok := p.reg.Item(id); ok {
			id, noted = form.BaseID, form.Noted
		}
		if k := slices.IndexFunc(lines, func(l *line) bool { return l.id == id && l.noted == noted }); k >= 0 {
			lines[k].qty -= *e.Qty
		}
	}
	t := content.Task{Type: content.TaskBank}
	for _, l := range lines {
		if l.qty <= 0 {
			continue
		}
		id, qty := l.id, l.qty
		name := fmt.Sprintf("item %d", id)
		if form, ok := p.reg.Item(id); ok {
			name = form.Name
		}
		t.Items = append(t.Items, content.ItemRequirement{Name: name, ItemID: &id, Qty: &qty, Noted: l.noted})
	}
	note := "items are what was withdrawn; deposits are in the evidence"
	if len(t.Items) == 0 {
		note = "nothing was withdrawn: the task banks for the segment's loadout"
	}
	p.add(t, map[string]string{}, evidence, note)
	return last
}

// run returns the index of the last event of the run that follows i: the
// events after i that match, with moves between them; moves after the last
// match stay for the main loop.
func run(events []recording.Event, i int, match func(recording.Event) bool) int {
	last := i
	for j := i + 1; j < len(events); j++ {
		switch e := events[j]; {
		case e.Kind == recording.KindMove:
			continue
		case match(e):
			last = j
		default:
			return last
		}
	}
	return last
}

// interact proposes the object option e.
func (p *proposer) interact(e recording.Event) {
	ref, ok := p.one(entities.KindObject, *e.Object, e)
	if !ok {
		return
	}
	t := content.Task{Type: content.TaskInteract, Object: &ref, Action: e.Option}
	p.add(t, p.names(map[string]*entities.Ref{"object": &ref}), []recording.Event{e}, "")
}

// itemUse has no task type of its own yet: it becomes a manual task with a
// note for the owner, which the verifier accepts on a step.
func (p *proposer) itemUse(e recording.Event) {
	used := fmt.Sprintf("item %d", *e.Item)
	if form, ok := p.reg.Item(*e.Item); ok {
		used = form.Name
	}
	target := e.Name
	if target == "" && e.TargetItem != nil {
		target = fmt.Sprintf("item %d", *e.TargetItem)
		if form, ok := p.reg.Item(*e.TargetItem); ok {
			target = form.Name
		}
	}
	t := content.Task{Type: content.TaskManual, Note: fmt.Sprintf("Usar %s em %s", used, target)}
	p.add(t, map[string]string{}, []recording.Event{e}, "item on something has no task type yet (docs/10 section 7): a manual task carries it")
}

// one resolves a game id of an npc or an object to its one catalog page.
func (p *proposer) one(kind entities.Kind, id int, e recording.Event) (entities.Ref, bool) {
	var refs []entities.Ref
	if kind == entities.KindNPC {
		refs = p.reg.NPCsByGameID(id)
	} else {
		refs = p.reg.ObjectsByGameID(id)
	}
	switch len(refs) {
	case 1:
		return refs[0], true
	case 0:
		p.issue(IssueUnresolved, fmt.Sprintf("%s id %d (%s) is in no catalog page", kind, id, e.Name), nil, e)
	default:
		p.issue(IssueAmbiguous, fmt.Sprintf("%s id %d (%s) is on %d catalog pages: choose one", kind, id, e.Name, len(refs)), refs, e)
	}
	return entities.Ref{}, false
}

// add finishes a task: the termination talk and interact need (the step's
// goal, when it has one), the verifier's opinion and the evidence.
func (p *proposer) add(t content.Task, names map[string]string, evidence []recording.Event, note string) {
	if (t.Type == content.TaskTalk || t.Type == content.TaskInteract || t.Type == content.TaskManual) && len(p.ctx.Goal) > 0 {
		t.Until = &content.TaskUntil{State: content.UntilGoalMet}
	} else if t.Type == content.TaskManual {
		t.Until = &content.TaskUntil{State: content.UntilOwnerMark}
	}
	ev := content.RecordedEvidence{Recording: evidence[0].Recording}
	for _, e := range evidence {
		if e.Recording == ev.Recording {
			ev.Events = append(ev.Events, e)
			p.used[key(e)] = true
		}
	}
	problems := append(t.Check(p.reg, p.ctx), content.CheckRecorded(t, ev, p.reg)...)
	p.out.Tasks = append(p.out.Tasks, Task{Task: t, Names: names, Confidence: content.ConfidenceMedium, Evidence: ev, Problems: problems, Note: note})
}

func (p *proposer) issue(kind, msg string, candidates []entities.Ref, events ...recording.Event) {
	if n := len(p.out.Issues); n > 0 && p.out.Issues[n-1].Kind == kind && p.out.Issues[n-1].Message == msg {
		// the same issue again (interactions on one unmapped tile): one entry
		for _, e := range events {
			p.out.Issues[n-1].Seqs = append(p.out.Issues[n-1].Seqs, key(e))
		}
		return
	}
	is := Issue{Kind: kind, Message: msg, Seqs: []string{}}
	for _, e := range events {
		is.Seqs = append(is.Seqs, key(e))
	}
	for _, c := range candidates {
		s := c.String()
		if name, ok := p.reg.Display(c); ok {
			s += " (" + name + ")"
		}
		is.Candidates = append(is.Candidates, s)
	}
	p.out.Issues = append(p.out.Issues, is)
}

// names are the display names of the refs, for the owner's question.
func (p *proposer) names(refs map[string]*entities.Ref) map[string]string {
	out := map[string]string{}
	for k, r := range refs {
		if r == nil {
			continue
		}
		if name, ok := p.reg.Display(*r); ok {
			out[k] = name
		} else {
			out[k] = strings.TrimPrefix(r.String(), string(r.Kind)+":")
		}
	}
	return out
}
