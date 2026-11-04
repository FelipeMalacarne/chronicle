package bruhsailer_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/importer/bruhsailer"
)

func TestGeneratePinnedSourceCountsAndDeterminism(t *testing.T) {
	sourceDir, guidePath := repositoryPaths(t)
	first, result, err := bruhsailer.Generate(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	second, secondResult, err := bruhsailer.Generate(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two imports of the pinned sources produced different bytes")
	}
	if result.SourceFiles != 3 || result.Sections != 9 || result.Steps != 226 {
		t.Fatalf("counts = sources %d, sections %d, steps %d; want 3, 9, 226", result.SourceFiles, result.Sections, result.Steps)
	}
	if result.Variants != 1 || result.Segments != 102 || result.BankLegs != 99 || result.Partial != 84 || result.Unknown != 18 {
		t.Fatalf("derived counts = variants %d, segments %d, bank legs %d, partial %d, unknown %d; want 1, 102, 99, 84, 18",
			result.Variants, result.Segments, result.BankLegs, result.Partial, result.Unknown)
	}
	wantChapterSteps := map[int]int{1: 98, 2: 99, 3: 29}
	for chapter, want := range wantChapterSteps {
		if got := result.ChapterSteps[chapter]; got != want {
			t.Errorf("chapter %d steps = %d, want %d", chapter, got, want)
		}
	}
	if secondResult.Steps != result.Steps {
		t.Fatalf("second result steps = %d, want %d", secondResult.Steps, result.Steps)
	}
	wantMetadata := bruhsailer.MetadataCounts{GP: 226, Items: 225, Skills: 225, Time: 226, Links: 158}
	if result.Metadata != wantMetadata {
		t.Fatalf("metadata counts = %+v, want %+v", result.Metadata, wantMetadata)
	}

	checkedIn, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, checkedIn) {
		t.Fatal("checked-in guide differs from deterministic importer output; run `go run ./cmd/ironman content import-bruhsailer`")
	}
}

func TestGeneratedGuideSatisfiesCanonicalSchema(t *testing.T) {
	sourceDir, _ := repositoryPaths(t)
	outputPath := filepath.Join(t.TempDir(), "guide.json")
	result, err := bruhsailer.Run(sourceDir, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.GuideFile != outputPath {
		t.Fatalf("guide file = %q, want %q", result.GuideFile, outputPath)
	}
	guide, err := content.LoadGuideFile(outputPath)
	if err != nil {
		t.Fatalf("canonical content validation failed: %v", err)
	}
	if guide.ID != bruhsailer.GuideID || guide.StepCount() != 226 {
		t.Fatalf("loaded guide = %q with %d steps, want %q with 226", guide.ID, guide.StepCount(), bruhsailer.GuideID)
	}
	if guide.DefaultVariant != "main" || len(guide.Variants) != 1 {
		t.Fatalf("variants = default %q, count %d; want main, 1", guide.DefaultVariant, len(guide.Variants))
	}
	step, ok := guide.Step("bruh-1.1-f7e685efb210")
	if !ok || step.Source == nil || step.Source.OriginalID != "CH1.4" {
		t.Fatalf("representative CH1.4 step = %#v, %v", step, ok)
	}
	segment, ok := guide.Segment(step.SegmentID)
	if !ok || segment.Kind != content.SegmentBankLeg || len(segment.BankActions) != 2 {
		t.Fatalf("CH1.4 bank leg = %#v, %v", segment, ok)
	}
	if segment.BankActions[0].Type != content.BankDepositAll || segment.BankActions[0].Source.StepIDs[0] != step.ID {
		t.Errorf("CH1.4 deposit provenance = %#v", segment.BankActions[0])
	}
	if segment.Loadout == nil || segment.Loadout.Status != content.LoadoutPartial || len(segment.Loadout.Inventory) != 7 {
		t.Fatalf("CH1.4 loadout = %#v", segment.Loadout)
	}
	if source := segment.Loadout.Source; source == nil || source.Method != content.ProvenanceExplicit || len(source.StepIDs) != 1 || source.StepIDs[0] != step.ID {
		t.Errorf("CH1.4 loadout provenance = %#v", source)
	}
}

func TestPinnedSourceChecksumIsEnforced(t *testing.T) {
	sourceDir, _ := repositoryPaths(t)
	copyDir := t.TempDir()
	for _, spec := range bruhsailer.PinnedSources {
		data, err := os.ReadFile(filepath.Join(sourceDir, spec.Filename))
		if err != nil {
			t.Fatal(err)
		}
		if spec.Chapter == 2 {
			data = append([]byte(nil), data...)
			data[len(data)/2] ^= 0xff
		}
		if err := os.WriteFile(filepath.Join(copyDir, spec.Filename), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := bruhsailer.Generate(copyDir); err == nil {
		t.Fatal("Generate accepted a source snapshot whose checksum changed")
	}
}

func repositoryPaths(t *testing.T) (string, string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(root, "data", "sources", "bruhsailer"), filepath.Join(root, "content", "guides", bruhsailer.GuideID, "guide.json")
}
