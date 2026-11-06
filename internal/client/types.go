package client

import (
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/panel"
	coreprogress "github.com/FelipeMalacarne/ironman-planner/internal/core/progress"
)

// MarkAction is the kind of a mark write (manual_done, skipped, pinned_open,
// clear).
type MarkAction = coreprogress.MarkAction

// Mark actions.
const (
	ActionManualDone = coreprogress.ActionManualDone
	ActionSkipped    = coreprogress.ActionSkipped
	ActionPinnedOpen = coreprogress.ActionPinnedOpen
	ActionClear      = coreprogress.ActionClear
)

// Window is the active route from the cursor (GET /api/route).
type Window = panel.Window

// Graph is the requirement graph of a focus (GET /api/world/graph).
type Graph = coreprogress.Graph

// Account is one planner account.
type Account struct {
	ID      string `json:"id"`
	RSN     string `json:"rsn"`
	Mode    string `json:"mode"`
	GuideID string `json:"guide_id"`
	Layer   string `json:"layer"`
}

// Me is the user and their accounts.
type Me struct {
	AuthMode string    `json:"auth_mode"`
	Accounts []Account `json:"accounts"`
}

// Cursor is a guide's stored cursor.
type Cursor struct {
	StepID string `json:"step_id"`
	Cause  string `json:"cause"`
}

// Board is the account's progress in one guide.
type Board struct {
	Guide     string  `json:"guide"`
	Account   string  `json:"account"`
	Active    bool    `json:"active"`
	Cursor    *Cursor `json:"cursor"`
	CommandID string  `json:"command_id,omitempty"`
	coreprogress.Board
}

// RouteSummary is one route of GET /api/progress.
type RouteSummary struct {
	Guide         string                       `json:"guide"`
	Title         string                       `json:"title"`
	Version       string                       `json:"version,omitempty"`
	Source        *content.Source              `json:"source,omitempty"`
	Steps         int                          `json:"steps"`
	Active        bool                         `json:"active"`
	Cursor        *Cursor                      `json:"cursor"`
	CursorRef     string                       `json:"cursor_ref,omitempty"`
	CursorSection string                       `json:"cursor_section,omitempty"`
	Counts        coreprogress.Counts          `json:"counts"`
	Coverage      coreprogress.Coverage        `json:"coverage"`
	Sections      []coreprogress.SectionCounts `json:"sections"`
}

// Source is the newest fact one source reported.
type Source struct {
	Source         string    `json:"source"`
	LastObservedAt time.Time `json:"last_observed_at"`
	Facts          int64     `json:"facts"`
}

// Progress is GET /api/progress.
type Progress struct {
	Account     string         `json:"account"`
	ActiveGuide string         `json:"active_guide"`
	Routes      []RouteSummary `json:"routes"`
	Sources     []Source       `json:"sources"`
}

// Occurrence is where one route does something.
type Occurrence struct {
	Guide    string `json:"guide"`
	Step     string `json:"step"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Section  string `json:"section"`
	Position int    `json:"position,omitempty"`
	Total    int    `json:"total"`
	Key      string `json:"key"`
	Role     string `json:"role"`
}

// Goal is one pinned goal with its verdict and where each route completes it.
type Goal struct {
	ID         string       `json:"id"`
	Key        string       `json:"objective_key"`
	Label      string       `json:"label"`
	Name       string       `json:"name"`
	Ref        string       `json:"ref"`
	Kind       string       `json:"kind"`
	Priority   int          `json:"priority"`
	Status     string       `json:"status"`
	Verdict    string       `json:"verdict"`
	Regressed  bool         `json:"regressed,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	AchievedAt *time.Time   `json:"achieved_at,omitempty"`
	Routes     []Occurrence `json:"routes"`
}

// GoalPlanResponse is a goal and its plan.
type GoalPlanResponse struct {
	Account string                `json:"account"`
	Goal    Goal                  `json:"goal"`
	Plan    coreprogress.GoalPlan `json:"plan"`
}

// FactSource is where a fact came from and when.
type FactSource struct {
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}

// Skill is one skill of the account.
type Skill struct {
	Skill string `json:"skill"`
	Level int    `json:"level"`
	XP    int64  `json:"xp"`
	FactSource
}

// Quest is one quest the account reported.
type Quest struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	State string `json:"state"`
	FactSource
}

// Diary is one diary tier.
type Diary struct {
	Area     string `json:"area"`
	Tier     string `json:"tier"`
	Complete bool   `json:"complete"`
	FactSource
}

// Item is one item stack of a container.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int64  `json:"qty"`
	Slot string `json:"slot,omitempty"`
}

// Container is one item container with its freshness.
type Container struct {
	Container  string    `json:"container"`
	State      string    `json:"state"`
	ObservedAt time.Time `json:"observed_at"`
	Complete   bool      `json:"complete"`
	Items      []Item    `json:"items"`
}

// AccountState is GET /api/account/state.
type AccountState struct {
	Account     Account     `json:"account"`
	TotalLevel  int         `json:"total_level,omitempty"`
	QuestPoints *int64      `json:"quest_points,omitempty"`
	Skills      []Skill     `json:"skills"`
	Quests      []Quest     `json:"quests"`
	Diaries     []Diary     `json:"diaries"`
	Containers  []Container `json:"containers"`
}

// Link is a requirement of an entity, or an entity it unlocks.
type Link struct {
	Key         string `json:"key"`
	Ref         string `json:"ref,omitempty"`
	Label       string `json:"label"`
	Verdict     string `json:"verdict"`
	Boostable   bool   `json:"boostable,omitempty"`
	DuringQuest bool   `json:"during_quest,omitempty"`
}

// EntityObjective is one objective of an entity with the account's verdict.
type EntityObjective struct {
	Key       string `json:"key"`
	Verdict   string `json:"verdict"`
	Regressed bool   `json:"regressed,omitempty"`
}

// Entity is GET /api/world/entity.
type Entity struct {
	Ref         string            `json:"ref"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Wiki        string            `json:"wiki,omitempty"`
	Objectives  []EntityObjective `json:"objectives"`
	Routes      []Occurrence      `json:"routes"`
	Requires    []Link            `json:"requires"`
	CombatLevel int               `json:"combat_level,omitempty"`
	Unlocks     []Link            `json:"unlocks"`
}

// SyncRun is one hiscores run.
type SyncRun struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	Trigger       string     `json:"trigger"`
	Status        string     `json:"status"`
	HTTPStatus    int        `json:"http_status,omitempty"`
	Error         string     `json:"error,omitempty"`
	SnapshotID    string     `json:"snapshot_id,omitempty"`
	StepsChanged  int        `json:"steps_changed"`
	NextAllowedAt *time.Time `json:"next_allowed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// StepContent is a step of a guide (content only, no progress).
type StepContent struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Location string `json:"location,omitempty"`
	Section  string `json:"section"`
	Guide    string `json:"guide"`
	Ref      string `json:"ref"`
	Position int    `json:"position,omitempty"`
	Total    int    `json:"total"`
}
