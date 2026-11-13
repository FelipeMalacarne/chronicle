package propose

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	"github.com/FelipeMalacarne/chronicle/internal/core/recording"
)

var t0 = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// stretch builds events of one recording one second apart, from a short
// notation: each step sets the kind and fields on top of the last tile.
type stretch struct {
	events []recording.Event
	x, y   int
	plane  int
	world  int
}

func newStretch() *stretch { return &stretch{x: 3222, y: 3218, world: 330} }

func (s *stretch) at(x, y int) *stretch {
	s.x, s.y = x, y
	return s
}

func (s *stretch) add(kind string, edit func(*recording.Event)) *stretch {
	e := recording.Event{
		Recording: "rec-test", Seq: int64(len(s.events) + 1), At: t0.Add(time.Duration(len(s.events)) * time.Second),
		Kind: kind, World: s.world, X: s.x, Y: s.y, Plane: s.plane,
	}
	if edit != nil {
		edit(&e)
	}
	s.events = append(s.events, e)
	return s
}

func (s *stretch) move() *stretch { return s.add(recording.KindMove, nil) }

func (s *stretch) npc(id int, option string) *stretch {
	return s.add(recording.KindNPC, func(e *recording.Event) { e.NPC, e.Option = recording.Int(id), option })
}

func (s *stretch) object(id int, option string) *stretch {
	return s.add(recording.KindObject, func(e *recording.Event) { e.Object, e.Option = recording.Int(id), option })
}

func (s *stretch) say(text string, npc int) *stretch {
	return s.add(recording.KindDialogue, func(e *recording.Event) { e.Option, e.NPC = text, recording.Int(npc) })
}

func (s *stretch) items(kind string, id, qty int, npc *int) *stretch {
	return s.add(kind, func(e *recording.Event) { e.Item, e.Qty, e.NPC = recording.Int(id), recording.Int(qty), npc })
}

var cooksAssistant = content.TaskContext{Goal: []entities.ObjectiveKey{entities.MustParseObjectiveKey("quest:cooks_assistant@started")}}

func describe(t Task) string {
	b, _ := json.Marshal(t.Task)
	return string(b)
}

func TestPropose(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		events []recording.Event
		ctx    content.TaskContext
		want   []string // each proposed task's JSON, in order
		issues []string // a substring of each issue, in order
	}{
		{
			name: "walk into the castle and talk to the Duke",
			events: newStretch().at(3190, 3220).move().at(3222, 3218).move().npc(815, "Talk-to").
				say("Have you any quests for me?", 815).say("Sure, no problem.", 815).events,
			ctx: cooksAssistant,
			// Lumbridge was on the way: the travel goes where the interaction is.
			want: []string{
				`{"type":"travel","location":"location:lumbridge_castle"}`,
				`{"type":"talk","npc":"npc:duke_horacio","options":["Have you any quests for me?","Sure, no problem."],"until":{"state":"goal_met"}}`,
			},
		},
		{
			name: "a booth opens the bank: withdraw what is carried away, deposits cancel",
			events: newStretch().object(10583, "Bank").add(recording.KindBankOpen, func(e *recording.Event) { e.Object = recording.Int(10583) }).
				items(recording.KindBankWithdraw, 995, 250, nil).items(recording.KindBankWithdraw, 1512, 5, nil).
				items(recording.KindBankDeposit, 1511, 27, nil).items(recording.KindBankWithdraw, 2347, 1, nil).items(recording.KindBankDeposit, 2347, 1, nil).events,
			want: []string{
				`{"type":"travel","location":"location:lumbridge_castle"}`,
				`{"type":"bank","items":[{"name":"Coins","item_id":995,"qty":250},{"name":"Logs","item_id":1511,"qty":5,"noted":true}]}`,
			},
		},
		{
			name: "a trade with Bob is a buy at his shop, one per item",
			events: newStretch().at(3230, 3203).npc(10619, "Trade").
				items(recording.KindShopBuy, 1351, 1, recording.Int(10619)).items(recording.KindShopBuy, 1265, 1, recording.Int(10619)).
				items(recording.KindShopBuy, 1351, 1, recording.Int(10619)).events,
			// Bob's shop is in no location's area: no travel, said once.
			want: []string{
				`{"type":"buy","shop":"shop:bobs_brilliant_axes","item":"item:1351","qty":2}`,
				`{"type":"buy","shop":"shop:bobs_brilliant_axes","item":"item:1265","qty":1}`,
			},
			issues: []string{"(3230, 3203, 0) is in no location's area"},
		},
		{
			name:   "an object option, with the step's goal",
			events: newStretch().object(114, "Cook").events,
			ctx:    cooksAssistant,
			want: []string{
				`{"type":"travel","location":"location:lumbridge_castle"}`,
				`{"type":"interact","object":"object:cooking_range_lumbridge_castle","action":"Cook","until":{"state":"goal_met"}}`,
			},
		},
		{
			name: "a teleport tab is the travel's via",
			events: newStretch().add(recording.KindItemOp, func(e *recording.Event) { e.Item, e.Option = recording.Int(8007), "Break" }).
				at(3210, 3448).move().npc(3105, "Talk-to").events,
			want: []string{
				`{"type":"travel","location":"location:varrock","via":"item:8007"}`,
				`{"type":"talk","npc":"npc:hans"}`,
			},
		},
		{
			name:   "an item used on an object is a manual task for now",
			events: newStretch().add(recording.KindItemUse, func(e *recording.Event) { e.Item, e.Object, e.Name = recording.Int(1925), recording.Int(14868), "Sink" }).events,
			want: []string{
				`{"type":"travel","location":"location:lumbridge_castle"}`,
				`{"type":"manual","note":"Usar Bucket em Sink","until":{"state":"owner_mark"}}`,
			},
		},
		{
			name:   "an id on several pages is reported, never guessed",
			events: newStretch().npc(3908, "Talk-to").events,
			want:   []string{`{"type":"travel","location":"location:lumbridge_castle"}`},
			issues: []string{"npc id 3908 () is on 2 catalog pages"},
		},
		{
			name:   "an id in no page is reported",
			events: newStretch().npc(999999, "Talk-to").object(999998, "Open").events,
			want:   []string{`{"type":"travel","location":"location:lumbridge_castle"}`},
			issues: []string{"npc id 999999", "object id 999998"},
		},
		{
			name:   "what has no task type is reported",
			events: newStretch().npc(3105, "Pickpocket").npc(10619, "Trade").say("Hello", 1).events,
			want:   []string{`{"type":"travel","location":"location:lumbridge_castle"}`},
			issues: []string{`"Pickpocket" on an NPC has no task type`, "a trade with no purchase", "a dialogue option without an NPC"},
		},
		{
			name:   "a purchase from a shopkeeper whose shops do not sell it",
			events: newStretch().npc(10619, "Trade").items(recording.KindShopBuy, 1511, 1, recording.Int(10619)).events,
			want:   []string{`{"type":"travel","location":"location:lumbridge_castle"}`},
			issues: []string{"no shop of npc 10619 sells item 1511"},
		},
		{
			name:   "the stretch ends somewhere new",
			events: newStretch().npc(3105, "Talk-to").at(3105, 3265).move().events,
			want: []string{
				`{"type":"travel","location":"location:lumbridge_castle"}`,
				`{"type":"talk","npc":"npc:hans"}`,
				`{"type":"travel","location":"location:draynor_village"}`,
			},
		},
		{
			name:   "nowhere known",
			events: newStretch().at(2000, 2000).move().events,
			want:   []string{},
			issues: []string{"in no location's area"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Propose(Input{Events: c.events, Context: c.ctx, Registry: reg})
			if len(got.Tasks) != len(c.want) {
				var all []string
				for _, x := range got.Tasks {
					all = append(all, describe(x))
				}
				t.Fatalf("tasks:\n%s\nwant %d: %v\nissues %+v", strings.Join(all, "\n"), len(c.want), c.want, got.Issues)
			}
			for i, w := range c.want {
				if g := describe(got.Tasks[i]); g != w {
					t.Errorf("task %d:\n got %s\nwant %s", i, g, w)
				}
				if got.Tasks[i].Confidence != content.ConfidenceMedium || len(got.Tasks[i].Evidence.Events) == 0 {
					t.Errorf("task %d: confidence %q, %d events", i, got.Tasks[i].Confidence, len(got.Tasks[i].Evidence.Events))
				}
			}
			if len(got.Issues) != len(c.issues) {
				t.Fatalf("issues = %+v, want %v", got.Issues, c.issues)
			}
			for i, w := range c.issues {
				if !strings.Contains(got.Issues[i].Message, w) {
					t.Errorf("issue %d = %q, want %q", i, got.Issues[i].Message, w)
				}
			}
		})
	}
}

// Every proposed task carries its events and passes the verifier as it is,
// except what the owner must decide (a talk without the step's goal needs
// an until).
func TestProposalsPassTheVerifier(t *testing.T) {
	reg, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	events := newStretch().at(3190, 3220).move().at(3222, 3218).npc(815, "Talk-to").say("Have you any quests for me?", 815).
		at(3230, 3203).npc(10619, "Trade").items(recording.KindShopBuy, 1351, 1, recording.Int(10619)).
		object(10583, "Bank").add(recording.KindBankOpen, nil).items(recording.KindBankWithdraw, 995, 100, nil).events
	got := Propose(Input{Events: events, Context: cooksAssistant, Registry: reg})
	// travel, talk, buy (Bob's is unmapped: no travel there) and the bank,
	// with the booth that opened it; the move through Lumbridge is unused.
	if len(got.Tasks) != 4 || got.Used != len(events)-1 {
		t.Fatalf("tasks %d, used %d of %d: %+v", len(got.Tasks), got.Used, len(events), got)
	}
	for _, task := range got.Tasks {
		if len(task.Problems) > 0 {
			t.Errorf("%s: %v", describe(task), task.Problems)
		}
		if task.Evidence.Recording != "rec-test" {
			t.Errorf("%s: evidence recording %q", describe(task), task.Evidence.Recording)
		}
	}
	if talk := got.Tasks[1]; talk.Names["npc"] != "Duke Horacio" || len(talk.Evidence.Events) != 2 {
		t.Errorf("talk = %+v", talk)
	}

	noGoal := Propose(Input{Events: newStretch().npc(815, "Talk-to").events, Registry: reg})
	if talk := noGoal.Tasks[1]; !strings.Contains(strings.Join(talk.Problems, ";"), "talk needs until") {
		t.Errorf("a talk without the step's goal: problems %v", talk.Problems)
	}
}
