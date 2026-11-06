package httpapi

import (
	"net/http"
	"testing"
)

func TestWorldGraphRoute(t *testing.T) {
	fake := &fakeProgress{}
	server := newProgressServer(t, true, fake)
	base := server.URL + "/api/world/graph?account="

	status, body := do(t, "GET", base+"acc-1&focus=quest:shilo_village@finished", "")
	if status != http.StatusOK || body["focus"] != "quest:shilo_village@finished" || body["depth"] != float64(3) || body["account"] != "acc-1" || fake.call != "graph" {
		t.Fatalf("graph = %d %v", status, body)
	}
	if status, body := do(t, "GET", base+"acc-1&focus=skill:agility%3E%3D50&depth=6", ""); status != http.StatusOK || body["depth"] != float64(6) || fake.step != "skill:agility>=50" {
		t.Fatalf("graph depth 6 = %d %v (focus %q)", status, body, fake.step)
	}
	for _, q := range []string{
		"acc-1",
		"acc-1&focus=%20",
		"acc-1&focus=quest:x@finished&depth=0",
		"acc-1&focus=quest:x@finished&depth=7",
		"acc-1&focus=quest:x@finished&depth=two",
		"acc-1&focus=nonsense",
	} {
		if status, _ := do(t, "GET", base+q, ""); status != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", q, status)
		}
	}
	if status, _ := do(t, "GET", base+"nobody&focus=quest:x@finished", ""); status != http.StatusNotFound {
		t.Fatalf("unknown account = %d", status)
	}
	off := newProgressServer(t, false, fake)
	if status, _ := do(t, "GET", off.URL+"/api/world/graph?account=acc-1&focus=quest:x@finished", ""); status != http.StatusNotFound {
		t.Fatalf("graph outside dev mode without oidc = %d", status)
	}
}
