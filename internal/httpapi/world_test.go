package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

func TestGoalsAndWorld(t *testing.T) {
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	cnt := &content.Content{Guides: []*content.Guide{crossGuide("oziris-main", "1.1.40", "", 3), crossGuide("bruhsailer-main", "bruh-1.1-x", "CH1.6", 2)}}
	fake := &fakeProgress{}
	server := httptest.NewServer(New(Options{Version: "test", Content: cnt, Catalog: catalog, Progress: fake, DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
	t.Cleanup(server.Close)
	base := server.URL + "/api"

	status, body := do(t, "GET", base+"/goals?account=acc-1", "")
	goals, _ := body["goals"].([]any)
	if status != http.StatusOK || len(goals) != 1 {
		t.Fatalf("goals = %d %v", status, body)
	}
	g := goals[0].(map[string]any)
	if g["name"] != "Monk's Friend" || g["label"] != "Monk's Friend" || len(g["routes"].([]any)) != 2 {
		t.Fatalf("goal = %v", g)
	}
	if status, body = do(t, "POST", base+"/goals?account=acc-1", `{"objective_key":"skill:agility>=70"}`); status != http.StatusCreated || body["name"] != "Agility" {
		t.Fatalf("add goal = %d %v", status, body)
	}
	if status, _ = do(t, "POST", base+"/goals?account=acc-1", `{"objective_key":"skill:agility:70"}`); status != http.StatusBadRequest {
		t.Fatalf("invalid goal = %d", status)
	}
	if status, _ = do(t, "DELETE", base+"/goals/goal_1?account=acc-1", ""); status != http.StatusNoContent {
		t.Fatalf("remove goal = %d", status)
	}
	if status, _ = do(t, "DELETE", base+"/goals/goal_9?account=acc-1", ""); status != http.StatusNotFound {
		t.Fatalf("remove missing goal = %d", status)
	}

	status, body = do(t, "GET", base+"/world?account=acc-1", "")
	list, _ := body["entities"].([]any)
	if status != http.StatusOK || len(list) == 0 {
		t.Fatalf("world = %d %v", status, body)
	}
	var monk map[string]any
	for _, e := range list {
		if m := e.(map[string]any); m["ref"] == "quest:monks_friend" {
			monk = m
		}
	}
	if monk == nil || monk["name"] != "Monk's Friend" || monk["kind"] != "quest" || monk["wiki"] == "" ||
		len(monk["routes"].([]any)) != 2 || monk["objectives"].([]any)[0].(map[string]any)["verdict"] != "met" {
		t.Fatalf("monk's friend = %v", monk)
	}
}
