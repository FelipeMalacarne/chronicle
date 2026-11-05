package ingest

import "time"

// ContractVersion is the only wire-contract version understood by this package.
const ContractVersion = 1

// TokenScope is an effective permission attached to an ingest token.
type TokenScope string

const (
	ScopeStateWrite     TokenScope = "state:write"
	ScopeItemsBank      TokenScope = "items:bank"
	ScopeItemsInventory TokenScope = "items:inventory"
	ScopeItemsEquipment TokenScope = "items:equipment"
	// ScopeMarksWrite lets the plugin panel write the owner's marks (Feito,
	// Pular, Desfazer). Reading the panel needs any valid credential.
	ScopeMarksWrite TokenScope = "marks:write"
)

// Manifest tells a client which state it may read and submit. Item collection
// is disabled by the zero value of ItemSelection and therefore remains opt-in.
type Manifest struct {
	ContractVersion int          `json:"contract_version"`
	ManifestVersion int64        `json:"manifest_version"`
	TokenScopes     []TokenScope `json:"token_scopes"`
	Varbits         []int64      `json:"varbits"`
	Varps           []int64      `json:"varps"`
	Collections     []int64      `json:"collections"`
	Skills          bool         `json:"skills"`
	// Quests tells the client the server accepts data.quest. A client must
	// not send data.quest unless it is true (an older server rejects the
	// unknown field).
	Quests          bool          `json:"quests"`
	Items           ItemSelection `json:"items"`
	IntervalSeconds int           `json:"interval_seconds"`
	Resync          bool          `json:"resync"`
}

// ItemSelection is the server-side permission advertised to a client. A
// client must also have its corresponding local opt-in enabled before reading
// a container.
type ItemSelection struct {
	Bank        bool    `json:"bank"`
	Inventory   bool    `json:"inventory"`
	Equipment   bool    `json:"equipment"`
	OnlyItemIDs []int64 `json:"only_item_ids,omitempty"`
}

// PushKind controls whether maps are complete replacements or patches.
type PushKind string

const (
	PushFull  PushKind = "full"
	PushDelta PushKind = "delta"
)

// Push is one ordered client submission. ClientSeq is monotonic per token.
type Push struct {
	ContractVersion int       `json:"contract_version"`
	Username        string    `json:"username"`
	Profile         string    `json:"profile"`
	AccountType     string    `json:"account_type,omitempty"`
	ManifestVersion int64     `json:"manifest_version"`
	ClientSeq       int64     `json:"client_seq"`
	Kind            PushKind  `json:"kind"`
	SentAt          time.Time `json:"sent_at"`
	Data            PushData  `json:"data"`
}

// IntMap is a Java/Gson-friendly JSON object with string keys. Numeric IDs are
// represented as canonical base-10 strings on the wire.
type IntMap map[string]int64

// PushData uses pointers so JSON can distinguish an omitted delta facet from
// a present but empty full facet.
type PushData struct {
	Varbits                *IntMap         `json:"varb,omitempty"`
	Varps                  *IntMap         `json:"varp,omitempty"`
	Levels                 *IntMap         `json:"level,omitempty"`
	XP                     *IntMap         `json:"xp,omitempty"`
	Boosted                *IntMap         `json:"boosted,omitempty"`
	CollectionLogSlots     *string         `json:"collectionLogSlots,omitempty"`
	CollectionLogItemCount *int64          `json:"collectionLogItemCount,omitempty"`
	Items                  *ItemContainers `json:"items,omitempty"`
	// Quests maps the RuneLite quest name (Quest.getName()) to its
	// QuestState. Optional in every push for backward compatibility; when
	// present in a full push it is the complete quest list and replaces the
	// stored quests, in a delta it carries only the quests that changed.
	Quests *QuestMap `json:"quest,omitempty"`
}

// QuestMap is a JSON object RuneLite quest name -> NOT_STARTED |
// IN_PROGRESS | FINISHED, which maps to Map<String, String> in Java.
type QuestMap map[string]string

// Quest states, as RuneLite's QuestState enum names them.
const (
	QuestNotStarted = "NOT_STARTED"
	QuestInProgress = "IN_PROGRESS"
	QuestFinished   = "FINISHED"
)

// MaxQuestNameLength bounds a data.quest key.
const MaxQuestNameLength = 120

// ItemContainers holds independent observations. Full pushes include all
// three pointers; delta pushes include only containers whose state or contents
// changed.
type ItemContainers struct {
	Bank      *ItemObservation `json:"bank,omitempty"`
	Inventory *ItemObservation `json:"inventory,omitempty"`
	Equipment *ItemObservation `json:"equipment,omitempty"`
}

// ItemState distinguishes an unreadable container from a persisted mirror and
// a container observed from the current live client session.
type ItemState string

const (
	ItemUnavailable ItemState = "unavailable"
	ItemRestored    ItemState = "restored"
	ItemLive        ItemState = "live"
)

// QuantityMap is encoded as a JSON object (item_id string -> quantity), which
// maps directly to Map<String, Long> in Java. A pointer preserves the
// difference between an absent map and an observed empty container.
type QuantityMap map[string]int64

// ItemObservation carries freshness on the container, never on individual
// quantities. Revision is monotonic within its source; LiveEpoch identifies a
// single live client session and is present only for ItemLive.
type ItemObservation struct {
	State      ItemState    `json:"state"`
	ObservedAt time.Time    `json:"observed_at"`
	Revision   *int64       `json:"revision,omitempty"`
	LiveEpoch  string       `json:"live_epoch,omitempty"`
	Quantities *QuantityMap `json:"quantities,omitempty"`
}

// Response acknowledges the highest sequence accepted for this token. Resync
// directs the client to refresh the manifest and send a full push next.
type Response struct {
	ContractVersion int    `json:"contract_version"`
	OK              bool   `json:"ok"`
	AcceptedSeq     int64  `json:"accepted_seq"`
	Resync          bool   `json:"resync"`
	ManifestVersion int64  `json:"manifest_version"`
	StepsChanged    *int64 `json:"steps_changed,omitempty"`
	// Route is the account's active route after the reevaluation of this
	// push: its cursor and the steps whose status changed. It lets the plugin
	// panel decide whether to refetch. Absent when no reevaluation ran; old
	// clients ignore it.
	Route *RouteUpdate `json:"route,omitempty"`
}

// RouteUpdate is what one accepted push changed in the active route.
type RouteUpdate struct {
	Guide string `json:"guide"`
	// Cursor is the cursor step id after the push; empty when the route is
	// finished.
	Cursor      string `json:"cursor,omitempty"`
	CursorCause string `json:"cursor_cause,omitempty"`
	// Changed lists the steps of the active route whose status changed
	// (engine status changes and implied marks), in route order.
	Changed []string `json:"changed"`
}
