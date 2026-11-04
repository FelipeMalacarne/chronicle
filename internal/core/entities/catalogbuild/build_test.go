package catalogbuild

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
)

var update = flag.Bool("update", false, "rewrite content/reference generated catalogs")

const repoRoot = "../../../.."

// TestGeneratedCatalogsUpToDate regenerates the derived catalogs from the
// offline inputs and compares them byte for byte with the committed files.
func TestGeneratedCatalogsUpToDate(t *testing.T) {
	files, err := Build(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range files {
		path := filepath.Join(repoRoot, "content", "reference", name)
		if *update {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run go test ./internal/core/entities/catalogbuild -update)", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run go test ./internal/core/entities/catalogbuild -update", path)
		}
	}
}

// TestBuildIsDeterministic guards against map-order leaks into the output.
func TestBuildIsDeterministic(t *testing.T) {
	a, err := Build(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		b, err := Build(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		for name := range a {
			if !bytes.Equal(a[name], b[name]) {
				t.Fatalf("%s differs between runs", name)
			}
		}
	}
}

// curatedCorrections are the curated entries the wiki extract corrects: the
// curated catalog had the raw fish id under the cooked name, and the wiki
// renamed Rune sword (kept as an alias).
var curatedCorrections = map[int]string{371: "Raw swordfish", 1289: "Rune shortsword"}

// TestGeneratedItemsAgreeWithCurated checks the generated item catalog
// against the hand-curated subset it replaced (testdata/items_curated.json):
// every curated id is present with the same name, noted form and family.
func TestGeneratedItemsAgreeWithCurated(t *testing.T) {
	files, err := Build(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var gen, cur entities.ItemCatalog
	if err = json.Unmarshal(files[entities.ItemsFile], &gen); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "items_curated.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cur); err != nil {
		t.Fatal(err)
	}
	byID := map[int]entities.ItemEntry{}
	for _, it := range gen.Items {
		byID[it.ID] = it
	}
	for _, c := range cur.Items {
		g, ok := byID[c.ID]
		if !ok {
			t.Errorf("curated item %d %q missing from the generated catalog", c.ID, c.Name)
			continue
		}
		wantName := c.Name
		if fixed, ok := curatedCorrections[c.ID]; ok {
			wantName = fixed
		}
		if g.Name != wantName {
			t.Errorf("item %d: name %q, curated %q", c.ID, g.Name, wantName)
		}
		if c.NotedID != nil && (g.NotedID == nil || *g.NotedID != *c.NotedID) {
			t.Errorf("item %d: noted id %v, curated %d", c.ID, g.NotedID, *c.NotedID)
		}
		if c.Family != "" && g.Family != c.Family {
			t.Errorf("item %d: family %q, curated %q", c.ID, g.Family, c.Family)
		}
		for _, a := range c.Aliases {
			if !slices.Contains(g.Aliases, a) {
				t.Errorf("item %d: curated alias %q lost", c.ID, a)
			}
		}
	}
	amb := map[string]bool{}
	for _, a := range gen.Ambiguous {
		amb[a.Text] = true
	}
	for _, a := range cur.Ambiguous {
		if !amb[a.Text] {
			t.Errorf("curated ambiguous name %q lost", a.Text)
		}
	}
}
