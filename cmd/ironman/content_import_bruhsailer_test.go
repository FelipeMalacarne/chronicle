package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/importer/bruhsailer"
)

func TestContentImportBruhsailerCommand(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	sourceDir := filepath.Join(root, "data", "sources", "bruhsailer")
	outputPath := filepath.Join(t.TempDir(), "guide.json")

	cmd := newContentImportBruhsailerCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--source", sourceDir, "--out", outputPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "3 pinned sources, 9 sections, 102 segments, 99 bank legs, 226 steps") {
		t.Fatalf("unexpected command output: %s", stdout.String())
	}
	guide, err := content.LoadGuideFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if guide.ID != bruhsailer.GuideID {
		t.Fatalf("guide id = %q, want %q", guide.ID, bruhsailer.GuideID)
	}
}
