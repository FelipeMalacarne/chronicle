package ingest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)

func pointer[T any](value T) *T { return &value }

func unavailableObservation() *ItemObservation {
	return &ItemObservation{
		State:      ItemUnavailable,
		ObservedAt: testTime.Add(-time.Second),
	}
}

func restoredObservation(quantities QuantityMap) *ItemObservation {
	return &ItemObservation{
		State:      ItemRestored,
		ObservedAt: testTime.Add(-time.Minute),
		Revision:   pointer[int64](4),
		Quantities: &quantities,
	}
}

func liveObservation(quantities QuantityMap) *ItemObservation {
	return &ItemObservation{
		State:      ItemLive,
		ObservedAt: testTime.Add(-time.Second),
		Revision:   pointer[int64](9),
		LiveEpoch:  "login-2026-11-04T11:59:00Z",
		Quantities: &quantities,
	}
}

func fullPush() Push {
	emptyVarbits := IntMap{}
	emptyVarps := IntMap{}
	levels := IntMap{"attack": 80}
	xp := IntMap{"attack": 2_000_000}
	boosted := IntMap{}
	return Push{
		ContractVersion: ContractVersion,
		Username:        "Example Iron",
		Profile:         "STANDARD",
		AccountType:     "IRONMAN",
		ManifestVersion: 7,
		ClientSeq:       12,
		Kind:            PushFull,
		SentAt:          testTime,
		Data: PushData{
			Varbits: &emptyVarbits,
			Varps:   &emptyVarps,
			Levels:  &levels,
			XP:      &xp,
			Boosted: &boosted,
			Items: &ItemContainers{
				Bank:      unavailableObservation(),
				Inventory: unavailableObservation(),
				Equipment: unavailableObservation(),
			},
		},
	}
}

func deltaPush(observation *ItemObservation) Push {
	return Push{
		ContractVersion: ContractVersion,
		Username:        "Example Iron",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       13,
		Kind:            PushDelta,
		SentAt:          testTime,
		Data: PushData{
			Items: &ItemContainers{Bank: observation},
		},
	}
}

func TestValidItemObservationStates(t *testing.T) {
	tests := []struct {
		name string
		obs  *ItemObservation
	}{
		{name: "unavailable", obs: unavailableObservation()},
		{name: "restored mirror", obs: restoredObservation(QuantityMap{"4151": 1})},
		{name: "live client", obs: liveObservation(QuantityMap{"4151": 1})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			push := fullPush()
			push.Data.Items.Bank = tt.obs
			if err := push.Validate(); err != nil {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestInvalidFreshnessCombinations(t *testing.T) {
	tests := []struct {
		name string
		push func() Push
		want string
	}{
		{
			name: "unavailable has revision",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank.Revision = pointer[int64](1)
				return p
			},
			want: "revision must be absent",
		},
		{
			name: "unavailable has quantities",
			push: func() Push {
				p := fullPush()
				quantities := QuantityMap{}
				p.Data.Items.Bank.Quantities = &quantities
				return p
			},
			want: "quantities must be absent",
		},
		{
			name: "restored claims live epoch",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = restoredObservation(QuantityMap{"4151": 1})
				p.Data.Items.Bank.LiveEpoch = "not-really-live"
				return p
			},
			want: "live_epoch must be absent",
		},
		{
			name: "restored used as delta",
			push: func() Push {
				return deltaPush(restoredObservation(QuantityMap{"4151": 1}))
			},
			want: "valid only in a full push",
		},
		{
			name: "live lacks epoch",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = liveObservation(QuantityMap{"4151": 1})
				p.Data.Items.Bank.LiveEpoch = ""
				return p
			},
			want: "live_epoch is required",
		},
		{
			name: "live lacks revision",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = liveObservation(QuantityMap{"4151": 1})
				p.Data.Items.Bank.Revision = nil
				return p
			},
			want: "revision is required",
		},
		{
			name: "observation after send",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank.ObservedAt = p.SentAt.Add(time.Second)
				return p
			},
			want: "must not be after sent_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.push().Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestFullAndDeltaQuantitySemantics(t *testing.T) {
	tests := []struct {
		name    string
		push    func() Push
		wantErr string
	}{
		{
			name: "full positive quantity",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = liveObservation(QuantityMap{"4151": 1})
				return p
			},
		},
		{
			name: "full zero is ambiguous",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = liveObservation(QuantityMap{"4151": 0})
				return p
			},
			wantErr: "must be positive in a full push",
		},
		{
			name: "delta zero is tombstone",
			push: func() Push {
				return deltaPush(liveObservation(QuantityMap{"4151": 0}))
			},
		},
		{
			name: "delta positive is replacement quantity",
			push: func() Push {
				return deltaPush(liveObservation(QuantityMap{"4151": 3}))
			},
		},
		{
			name: "delta negative is invalid",
			push: func() Push {
				return deltaPush(liveObservation(QuantityMap{"4151": -1}))
			},
			wantErr: "must not be negative",
		},
		{
			name: "noncanonical item key is invalid",
			push: func() Push {
				return deltaPush(liveObservation(QuantityMap{"04151": 1}))
			},
			wantErr: "positive canonical item id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.push().Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFullAndDeltaPresenceSemantics(t *testing.T) {
	tests := []struct {
		name    string
		push    func() Push
		wantErr string
	}{
		{
			name: "full contains empty complete facets",
			push: fullPush,
		},
		{
			name: "full missing map",
			push: func() Push {
				p := fullPush()
				p.Data.Varbits = nil
				return p
			},
			wantErr: "data.varb is required",
		},
		{
			name: "full null map is not an empty object",
			push: func() Push {
				p := fullPush()
				var nullMap IntMap
				p.Data.Varbits = &nullMap
				return p
			},
			wantErr: "data.varb must be a JSON object",
		},
		{
			name: "full missing container",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = nil
				return p
			},
			wantErr: "data.items.bank is required",
		},
		{
			name: "live null quantities is not an empty container",
			push: func() Push {
				p := fullPush()
				p.Data.Items.Bank = liveObservation(QuantityMap{})
				var nullMap QuantityMap
				p.Data.Items.Bank.Quantities = &nullMap
				return p
			},
			wantErr: "quantities is required",
		},
		{
			name: "delta omits unchanged facets",
			push: func() Push {
				p := deltaPush(liveObservation(QuantityMap{"4151": 2}))
				p.Data.Varbits = nil
				p.Data.Items.Inventory = nil
				return p
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.push().Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestManifestScopesAndPrivacyDefaults(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion,
		ManifestVersion: 7,
		TokenScopes:     []TokenScope{ScopeStateWrite},
		Varbits:         []int64{},
		Varps:           []int64{},
		Collections:     []int64{},
		IntervalSeconds: 10,
	}
	if manifest.Items.Bank || manifest.Items.Inventory || manifest.Items.Equipment {
		t.Fatal("zero-value item selection must keep all private containers disabled")
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	manifest.Items.Bank = true
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), string(ScopeItemsBank)) {
		t.Fatalf("Validate() = %v, want missing bank scope", err)
	}
	manifest.TokenScopes = append(manifest.TokenScopes, ScopeItemsBank)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() with bank scope = %v", err)
	}

	manifest.Items.OnlyItemIDs = []int64{4151}
	push := fullPush()
	push.Data.Items.Bank = liveObservation(QuantityMap{"11840": 1})
	if err := push.ValidateAgainst(manifest); err == nil || !strings.Contains(err.Error(), "outside items.only_item_ids") {
		t.Fatalf("ValidateAgainst() = %v, want item allowlist error", err)
	}
}

func TestJSONRoundTrips(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion,
		ManifestVersion: 7,
		TokenScopes:     []TokenScope{ScopeStateWrite, ScopeItemsBank},
		Varbits:         []int64{0, 20},
		Varps:           []int64{},
		Collections:     []int64{4151},
		Skills:          true,
		Items:           ItemSelection{Bank: true, OnlyItemIDs: []int64{4151}},
		IntervalSeconds: 10,
		Resync:          true,
	}
	full := fullPush()
	full.Data.Items.Bank = restoredObservation(QuantityMap{"4151": 1})
	delta := deltaPush(liveObservation(QuantityMap{"4151": 0}))
	response := Response{
		ContractVersion: ContractVersion,
		OK:              true,
		AcceptedSeq:     13,
		ManifestVersion: 7,
		StepsChanged:    pointer[int64](2),
	}

	tests := []struct {
		name  string
		value any
		fresh func() any
	}{
		{name: "manifest", value: &manifest, fresh: func() any { return &Manifest{} }},
		{name: "full push", value: &full, fresh: func() any { return &Push{} }},
		{name: "delta push", value: &delta, fresh: func() any { return &Push{} }},
		{name: "response", value: &response, fresh: func() any { return &Response{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			decoded := tt.fresh()
			if err := json.Unmarshal(encoded, decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.value, decoded) {
				t.Fatalf("round trip mismatch\njson: %s\nwant: %#v\n got: %#v", encoded, tt.value, decoded)
			}
			if strings.Contains(string(encoded), "quantities\":[") {
				t.Fatalf("item quantities must encode as a JSON object: %s", encoded)
			}
		})
	}
}

func TestQuestFacetValidation(t *testing.T) {
	quests := func(m QuestMap) *QuestMap { return &m }
	tests := []struct {
		name    string
		push    func() Push
		wantErr string
	}{
		{
			name: "full without quests stays valid (plugin 1.0.0)",
			push: fullPush,
		},
		{
			name: "full with quests",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{"Cook's Assistant": QuestFinished, "X Marks the Spot": QuestInProgress, "Dragon Slayer I": QuestNotStarted})
				return p
			},
		},
		{
			name: "full with an empty quest list",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{})
				return p
			},
		},
		{
			name: "delta with one changed quest",
			push: func() Push {
				p := deltaPush(liveObservation(QuantityMap{"4151": 1}))
				p.Data.Quests = quests(QuestMap{"Rune Mysteries": QuestFinished})
				return p
			},
		},
		{
			name: "null quest map is not an empty object",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(nil)
				return p
			},
			wantErr: "data.quest must be a JSON object",
		},
		{
			name: "unknown state",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{"Cook's Assistant": "COMPLETED"})
				return p
			},
			wantErr: `data.quest["Cook's Assistant"] must be NOT_STARTED, IN_PROGRESS or FINISHED`,
		},
		{
			name: "lower-case state",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{"Cook's Assistant": "finished"})
				return p
			},
			wantErr: "must be NOT_STARTED, IN_PROGRESS or FINISHED",
		},
		{
			name: "blank name",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{" ": QuestFinished})
				return p
			},
			wantErr: "must be a non-empty trimmed quest name",
		},
		{
			name: "untrimmed name",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{"Cook's Assistant ": QuestFinished})
				return p
			},
			wantErr: "must be a non-empty trimmed quest name",
		},
		{
			name: "overlong name",
			push: func() Push {
				p := fullPush()
				p.Data.Quests = quests(QuestMap{strings.Repeat("a", MaxQuestNameLength+1): QuestFinished})
				return p
			},
			wantErr: "is longer than",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.push().Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestQuestFacetNeedsManifest(t *testing.T) {
	manifest := Manifest{
		ContractVersion: ContractVersion, ManifestVersion: 7, TokenScopes: []TokenScope{ScopeStateWrite},
		Varbits: []int64{}, Varps: []int64{}, Collections: []int64{}, IntervalSeconds: 10,
	}
	push := fullPush()
	push.Data.Quests = &QuestMap{"Cook's Assistant": QuestFinished}
	if err := push.ValidateAgainst(manifest); err == nil || !strings.Contains(err.Error(), "data.quest is not enabled") {
		t.Fatalf("ValidateAgainst() = %v, want quest gate", err)
	}
	manifest.Quests = true
	if err := push.ValidateAgainst(manifest); err != nil {
		t.Fatalf("ValidateAgainst() with quests = %v", err)
	}
	push.Data.Quests = nil
	if err := push.ValidateAgainst(manifest); err != nil {
		t.Fatalf("ValidateAgainst() without quests = %v", err)
	}
}

func TestQuestFacetWireShape(t *testing.T) {
	push := fullPush()
	push.Data.Quests = &QuestMap{"Cook's Assistant": QuestFinished}
	encoded, err := json.Marshal(push)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"quest":{"Cook's Assistant":"FINISHED"}`) {
		t.Fatalf("data.quest must encode as a JSON object of names: %s", encoded)
	}
	var decoded Push
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(decoded.Data.Quests, push.Data.Quests) {
		t.Fatalf("round trip = %v, %v", decoded.Data.Quests, err)
	}
	manifest, _ := json.Marshal(Manifest{Quests: true})
	if !strings.Contains(string(manifest), `"quests":true`) {
		t.Fatalf("manifest must announce quests: %s", manifest)
	}
}
