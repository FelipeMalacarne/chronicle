package catalogbuild

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
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
