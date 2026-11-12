package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

func crossGuide(id, stepID, original string, position int) *content.Guide {
	key := entities.MustParseObjectiveKey("quest:monks_friend@finished")
	loc := entities.Ref{Kind: entities.KindLocation, ID: "lumbridge"}
	steps := make([]*content.Step, 0, position)
	for i := 1; i < position; i++ {
		steps = append(steps, &content.Step{ID: id + "-filler-" + string(rune('a'+i)), Text: "walk"})
	}
	itemID := 952
	steps = append(steps, &content.Step{
		ID: stepID, Text: "Do Monk's Friend", Source: &content.Source{OriginalID: original}, LocationRef: &loc,
		Objectives:   []content.Objective{{Key: key, Role: content.RoleCompletes}},
		Requirements: []content.ItemRequirement{{Name: "spade", ItemID: &itemID}},
	})
	g := &content.Guide{ID: id, Title: id, Sections: []*content.Section{
		{ID: "s", Slug: "s", Steps: steps},
		{ID: "t", Slug: "t", Kind: content.KindTrack, Steps: []*content.Step{{ID: id + "-track", Text: "sail"}}},
	}}
	g.Index()
	return g
}

func TestStepCrossReferences(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cnt := &content.Content{Guides: []*content.Guide{crossGuide("oziris-main", "1.1.40", "", 3), crossGuide("bruhsailer-main", "bruh-1.1-x", "CH1.6", 2)}}
	server := httptest.NewServer(New(Options{Version: "test", Content: cnt, Catalog: catalog, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/api/guides/bruhsailer-main/steps/bruh-1.1-x")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Ref      string                  `json:"ref"`
		Position int                     `json:"position"`
		Total    int                     `json:"total"`
		Names    map[string]string       `json:"names"`
		Also     map[string][]occurrence `json:"also"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Ref != "CH1.6" || got.Position != 2 || got.Total != 2 {
		t.Fatalf("ref/position = %q %d/%d", got.Ref, got.Position, got.Total)
	}
	if got.Names["quest:monks_friend"] != "Monk's Friend" || got.Names["item:952"] != "Spade" {
		t.Fatalf("names = %v", got.Names)
	}
	also := got.Also["quest:monks_friend@finished"]
	if len(also) != 1 || also[0].Guide != "oziris-main" || also[0].Ref != "1.1.40" || also[0].Position != 3 || also[0].Total != 3 {
		t.Fatalf("also = %+v", got.Also)
	}

	resp, err = http.Get(server.URL + "/api/guides/oziris-main/sections/s")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sec struct {
		Steps []struct {
			ID   string                  `json:"id"`
			Also map[string][]occurrence `json:"also"`
		} `json:"steps"`
		Names map[string]string `json:"names"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&sec); err != nil {
		t.Fatal(err)
	}
	if len(sec.Steps) != 3 || sec.Steps[2].Also["quest:monks_friend@finished"][0].Ref != "CH1.6" || sec.Names["location:lumbridge"] == "" {
		t.Fatalf("section = %+v", sec)
	}
}

func TestStepNamesOfItemSetKeys(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	st := &content.Step{Objectives: []content.Objective{
		{Key: entities.MustParseObjectiveKey("item:family/graceful_hoodx1"), Role: content.RoleUses},
		{Key: entities.MustParseObjectiveKey("item:any(1355x2|1357x1)"), Role: content.RoleUses},
	}}
	names := map[string]string{}
	stepNames(st, catalog, names)
	want := map[string]string{
		"item:family/graceful_hood": "Graceful hood",
		"item:any(1355x2|1357x1)":   "Mithril axe / Adamant axe",
		"item:1355":                 "Mithril axe",
		"item:1357":                 "Adamant axe",
	}
	for k, v := range want {
		if names[k] != v {
			t.Errorf("names[%s] = %q, want %q", k, names[k], v)
		}
	}
}
