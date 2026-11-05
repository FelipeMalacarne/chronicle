package hiscores

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "data", "samples", "hiscores", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name      string
		file      string
		skills    int
		missing   []string
		check     map[string]map[string]int64
		activity  string
		minActive int
	}{
		{name: "maxed iron", file: "iron_ohzone.json", skills: 24,
			check:    map[string]map[string]int64{"skill/attack": {"level": 99, "xp": 122462831}, "skill/sailing": {"level": 99}},
			activity: "Barrows Chests", minActive: 50},
		{name: "unranked skills are unknown, not level 1", file: "ironman260.json", missing: []string{"fletching", "herblore", "agility"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse(sample(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			facts := Normalize(s)
			byKey := map[string]Fact{}
			skills, activities := 0, 0
			for _, f := range facts {
				byKey[f.Facet+"/"+f.Key] = f
				if f.Facet == "skill" {
					skills++
				} else {
					activities++
				}
			}
			if _, ok := byKey["skill/overall"]; ok {
				t.Fatal("Overall is not a skill")
			}
			if tc.skills > 0 && skills != tc.skills {
				t.Fatalf("skills = %d, want %d", skills, tc.skills)
			}
			for _, m := range tc.missing {
				if _, ok := byKey["skill/"+m]; ok {
					t.Fatalf("unranked %s was recorded", m)
				}
			}
			for k, want := range tc.check {
				for field, v := range want {
					if byKey[k].Value[field] != v {
						t.Fatalf("%s.%s = %d, want %d", k, field, byKey[k].Value[field], v)
					}
				}
			}
			if tc.activity != "" {
				if _, ok := byKey["activity/"+tc.activity]; !ok || activities < tc.minActive {
					t.Fatalf("activities = %d, %s missing", activities, tc.activity)
				}
			}
		})
	}
	if _, err := Parse(sample(t, "nonexistent.json")); err == nil {
		t.Fatal("the 404 HTML page parsed as hiscores")
	}
}

func TestFetch(t *testing.T) {
	body := sample(t, "iron_ohzone.json")
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case strings.Contains(r.URL.Path, "limited"):
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Query().Get("player") == "Gone Player":
			http.NotFound(w, r)
		case strings.Contains(r.URL.Path, "hardcore"):
			http.NotFound(w, r)
		default:
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	res, err := c.Fetch(ctx, "Iron Ohzone", "ironman")
	if err != nil || res.Table != "hiscore_oldschool_ironman" || len(res.Raw) == 0 {
		t.Fatalf("ironman = %+v, %v", res.Table, err)
	}
	if paths[0] != "/m=hiscore_oldschool_ironman/index_lite.json?player=Iron+Ohzone" {
		t.Fatalf("path = %s", paths[0])
	}
	if res, err = c.Fetch(ctx, "Dead Hc", "hcim"); err != nil || res.Table != "hiscore_oldschool" {
		t.Fatalf("hcim fallback = %+v, %v", res.Table, err)
	}
	if _, err = c.Fetch(ctx, "Gone Player", "ironman"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusNotFound {
		t.Fatalf("status error = %v", err)
	}
	endpoints["limited"] = "hiscore_limited"
	t.Cleanup(func() { delete(endpoints, "limited") })
	if _, err = c.Fetch(ctx, "x", "limited"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429 = %v", err)
	}
}

func TestDetect(t *testing.T) {
	body := sample(t, "iron_ohzone.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		player := r.URL.Query().Get("player")
		switch {
		case player == "Uim Guy" && strings.Contains(r.URL.Path, "ultimate"),
			player == "Iron Guy" && r.URL.Path == "/m=hiscore_oldschool_ironman/index_lite.json",
			player == "Main Guy" && r.URL.Path == "/m=hiscore_oldschool/index_lite.json":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	for rsn, want := range map[string]Detection{
		"Uim Guy":  {Mode: "uim", Found: true},
		"Iron Guy": {Mode: "ironman", Found: true},
		"Main Guy": {Mode: "", Found: true},
		"Nobody":   {Found: false},
	} {
		got, err := c.Detect(context.Background(), rsn)
		if err != nil || got.Mode != want.Mode || got.Found != want.Found {
			t.Fatalf("%s = %+v, %v", rsn, got.Mode, err)
		}
		if got.Found && len(got.Snapshot.Skills) == 0 {
			t.Fatalf("%s: no skills", rsn)
		}
	}
}
