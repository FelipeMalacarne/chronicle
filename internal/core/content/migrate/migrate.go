// Package migrate converts the normalizer output (data/guide/normalized/guide.json,
// schema v0.1 produced by tools/normalize_guide.py) into the canonical content
// model (content/schema/guide.schema.json). The field-by-field mapping is
// documented in docs/05-conteudo-e-normalizacao.md, section
// "Mapeamento para o schema canônico (Go)".
package migrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content/derive"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities/enrich"
)

// Normalized is the root of the v0.1 normalizer output.
type Normalized struct {
	SchemaVersion string     `json:"schema_version"`
	Guide         NGuide     `json:"guide"`
	Sections      []NSection `json:"sections"`
}

// NGuide is the v0.1 guide header.
type NGuide struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Source NSource `json:"source"`
}

// NSource is the v0.1 provenance block.
type NSource struct {
	URL         string `json:"url"`
	Author      string `json:"author"`
	Maintainer  string `json:"maintainer"`
	ExtractedAt string `json:"extracted_at"`
	LicenseNote string `json:"license_note"`
}

// NSection is a v0.1 section.
type NSection struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	SEOTitle    string            `json:"seo_title"`
	Track       bool              `json:"track"`
	Related     []content.Related `json:"related"`
	Steps       []NStep           `json:"steps"`
}

// NStep is a v0.1 step.
type NStep struct {
	ID            string   `json:"id"`
	Text          string   `json:"text"`
	Location      *string  `json:"location"`
	Note          *string  `json:"note"`
	UpdateNote    *string  `json:"update_note"`
	Items         []string `json:"items"`
	Links         []NLink  `json:"links"`
	Layer         string   `json:"layer"`
	Modes         []string `json:"modes"`
	Tags          []string `json:"tags"`
	Phase         *string  `json:"phase"`
	Quest         *NQuest  `json:"quest"`
	CompleteWhen  []NRule  `json:"complete_when"`
	Prerequisites []NRule  `json:"prerequisites"`
	Requires      []string `json:"requires"`
	CompletesIn   *string  `json:"completes_in"`
	TrackVia      string   `json:"track_via"`
}

// NLink is a v0.1 link.
type NLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Text string `json:"text"`
}

// NQuest is the v0.1 structured quest reference of a step.
type NQuest struct {
	Name   string  `json:"name"`
	Status *string `json:"status"`
	Wiki   string  `json:"wiki"`
}

// NRule is a v0.1 rule (flat superset of all rule types).
type NRule struct {
	Type       string `json:"type"`
	Skill      string `json:"skill"`
	Level      int    `json:"level"`
	Quest      string `json:"quest"`
	State      string `json:"state"`
	OrFinished bool   `json:"or_finished"`
	Area       string `json:"area"`
	Tier       string `json:"tier"`
	Boss       string `json:"boss"`
	KC         int    `json:"kc"`
	Source     string `json:"source"`
	Inferred   bool   `json:"inferred"`
	Confidence string `json:"confidence"`
	Match      string `json:"match"`
}

// Options controls the conversion.
type Options struct {
	// GuideID is the canonical guide id (and slug). Default "oziris-main".
	GuideID string
	// ContentVersion is written to guide.version. Default "2026.11.0".
	ContentVersion string
	// BaseURL resolves relative links (e.g. "/articles/x"). Default "https://ironman.guide".
	BaseURL string
}

func (o *Options) defaults() {
	if o.GuideID == "" {
		o.GuideID = "oziris-main"
	}
	if o.ContentVersion == "" {
		o.ContentVersion = "2026.11.0"
	}
	if o.BaseURL == "" {
		o.BaseURL = "https://ironman.guide"
	}
}

// Supported normalizer schema version.
const SupportedSchemaVersion = "0.1"

// ReadNormalized parses a v0.1 normalizer file.
func ReadNormalized(path string) (*Normalized, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var n Normalized
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&n); err != nil {
		return nil, fmt.Errorf("migrate: parse %s: %w", path, err)
	}
	if n.SchemaVersion != SupportedSchemaVersion {
		return nil, fmt.Errorf("migrate: %s has schema_version %q, want %q", path, n.SchemaVersion, SupportedSchemaVersion)
	}
	return &n, nil
}

// Convert maps a normalized guide onto the canonical model. The result is
// indexed but not yet schema-validated; use Encode + content.ValidateJSON.
func Convert(n *Normalized, opts Options) (*content.Guide, error) {
	g, _, err := ConvertWithReport(n, opts)
	return g, err
}

// ConvertWithReport is Convert plus the entity enrichment report (the
// ambiguous and unresolved references left out of the guide).
func ConvertWithReport(n *Normalized, opts Options) (*content.Guide, *enrich.Report, error) {
	opts.defaults()
	imported := n.Guide.Source.ExtractedAt
	g := &content.Guide{
		SchemaVersion: 1,
		ID:            opts.GuideID,
		Slug:          opts.GuideID,
		Title:         n.Guide.Title,
		Description: "Guide text by " + n.Guide.Source.Author + ", republished, maintained and extended by " +
			n.Guide.Source.Maintainer + ". Migrated from data/guide/normalized/guide.json (normalizer schema " +
			n.SchemaVersion + ") by `ironman content migrate`.",
		Version:       opts.ContentVersion,
		DefaultModes:  []string{"ironman", "hcim", "uim", "gim"},
		DefaultLayers: []string{"classic", "enhanced"},
		Source: &content.Source{
			Origin:     "ironman.guide",
			OriginalID: n.Guide.ID,
			URL:        n.Guide.Source.URL,
			Author:     n.Guide.Source.Author,
			License:    n.Guide.Source.LicenseNote,
			ImportedAt: imported,
		},
	}
	for _, ns := range n.Sections {
		sec := &content.Section{
			ID:          ns.ID,
			Slug:        ns.Slug,
			Title:       ns.Title,
			Description: ns.Description,
			Kind:        content.KindMain,
			Related:     ns.Related,
		}
		if ns.Track {
			sec.Kind = content.KindTrack
		}
		for _, nst := range ns.Steps {
			st, err := convertStep(nst, imported, opts)
			if err != nil {
				return nil, nil, fmt.Errorf("migrate: section %s step %s: %w", ns.ID, nst.ID, err)
			}
			sec.Steps = append(sec.Steps, st)
		}
		g.Sections = append(g.Sections, sec)
	}
	derive.EnrichGuide(g)
	reg, err := entities.Default()
	if err != nil {
		return nil, nil, err
	}
	return g, enrich.Guide(g, reg), nil
}

func convertStep(n NStep, imported string, opts Options) (*content.Step, error) {
	st := &content.Step{
		ID:       n.ID,
		Text:     n.Text,
		Location: deref(n.Location),
		Phase:    deref(n.Phase),
		Modes:    append([]string(nil), n.Modes...),
		Source:   &content.Source{Origin: "ironman.guide", OriginalID: n.ID, ImportedAt: imported},
	}
	tags := convertTags(n.Tags)
	hcim := contains(n.Tags, "hcim")

	if note := strings.TrimSpace(deref(n.Note)); note != "" {
		kind := content.NoteTip
		if hcim {
			kind = content.NoteHCIM
		}
		st.Notes = append(st.Notes, content.Note{Kind: kind, Text: note})
	}
	if upd := strings.TrimSpace(deref(n.UpdateNote)); upd != "" {
		st.Notes = append(st.Notes, content.Note{Kind: content.NoteUpdate, Text: upd})
	}
	for _, it := range n.Items {
		if it = strings.TrimSpace(it); it != "" {
			st.Items = append(st.Items, content.Item{Name: it})
		}
	}
	for _, l := range n.Links {
		st.Links = append(st.Links, content.Link{Type: l.Type, URL: absolute(l.URL, opts.BaseURL), Text: l.Text})
	}
	if n.Quest != nil && n.Quest.Wiki != "" {
		st.Links = append(st.Links, content.Link{Type: "wiki", URL: n.Quest.Wiki, Text: n.Quest.Name})
	}
	for _, r := range n.CompleteWhen {
		if r.Type == content.TypeManual {
			// Manual steps are expressed by an absent complete_when.
			continue
		}
		rule, err := convertRule(r)
		if err != nil {
			return nil, err
		}
		st.CompleteWhen = append(st.CompleteWhen, rule)
	}
	for _, r := range n.Prerequisites {
		rule, err := convertRule(r)
		if err != nil {
			return nil, fmt.Errorf("prerequisites: %w", err)
		}
		st.Prerequisites = append(st.Prerequisites, rule)
	}
	for _, id := range n.Requires {
		st.Requires = appendUnique(st.Requires, "step:"+id)
	}
	switch n.Layer {
	case "enhanced":
		st.Layers = []string{"enhanced"}
	case "classic", "":
		st.Layers = []string{"classic", "enhanced"}
	default:
		return nil, fmt.Errorf("unknown layer %q", n.Layer)
	}
	st.Tags = tags
	return st, nil
}

func convertRule(r NRule) (content.Rule, error) {
	h := content.RuleHeader{Type: r.Type}
	if r.Source != "" || r.Inferred || r.Confidence != "" || r.Match != "" {
		h.Meta = &content.RuleMeta{Origin: r.Source, Inferred: r.Inferred, Confidence: r.Confidence, Match: r.Match}
	}
	switch r.Type {
	case content.TypeSkillLevel:
		return content.SkillLevelRule{RuleHeader: h, Skill: r.Skill, Level: r.Level}, nil
	case content.TypeQuestState:
		q := content.QuestStateRule{RuleHeader: h, Quest: r.Quest, State: r.State}
		if r.OrFinished && r.State == content.QuestInProgress {
			q.Op = "gte"
		}
		return q, nil
	case content.TypeDiaryTier:
		return content.DiaryTierRule{RuleHeader: h, Area: r.Area, Tier: r.Tier}, nil
	case content.TypeBossKC:
		return content.BossKCRule{RuleHeader: h, Boss: r.Boss, KC: r.KC}, nil
	case content.TypeManual:
		return content.ManualRule{RuleHeader: h}, nil
	}
	return nil, fmt.Errorf("unsupported rule type %q", r.Type)
}

var nonTag = regexp.MustCompile(`[^a-z0-9:_-]+`)

// convertTags maps normalizer tags onto the schema's tag pattern
// (^[a-z0-9][a-z0-9:_-]*$): "boss:The Corrupted Gauntlet" → "boss:the-corrupted-gauntlet",
// "new-quest" → "new-content" (vocabulary of docs/04).
func convertTags(in []string) []string {
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "new-quest" {
			t = "new-content"
		}
		t = nonTag.ReplaceAllString(strings.ReplaceAll(t, " ", "-"), "")
		t = strings.Trim(t, "-:")
		if t == "" {
			continue
		}
		out = appendUnique(out, t)
	}
	return out
}

func absolute(u, base string) string {
	if strings.HasPrefix(u, "/") {
		return strings.TrimRight(base, "/") + u
	}
	return u
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func appendUnique(xs []string, s string) []string {
	if contains(xs, s) {
		return xs
	}
	return append(xs, s)
}

// Encode renders a guide as the canonical, deterministic JSON (2-space indent,
// no HTML escaping, trailing newline).
func Encode(g *content.Guide) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Result summarizes a Run.
type Result struct {
	GuideFile  string
	ReadmeFile string
	ReportFile string
	Entities   enrich.Tally
	Variants   int
	Sections   int
	Segments   int
	BankLegs   int
	Partial    int
	Unknown    int
	Steps      int
	WithRules  int
	Manual     int
}

// Run reads the normalizer file, converts it, validates the output against
// the canonical schema and writes <outDir>/guide.json and <outDir>/README.md.
func Run(inPath, outDir string, opts Options) (*Result, error) {
	opts.defaults()
	n, err := ReadNormalized(inPath)
	if err != nil {
		return nil, err
	}
	g, report, err := ConvertWithReport(n, opts)
	if err != nil {
		return nil, err
	}
	data, err := Encode(g)
	if err != nil {
		return nil, fmt.Errorf("migrate: encode: %w", err)
	}
	outFile := filepath.Join(outDir, content.GuideFileName)
	if _, err := content.ParseGuide(outFile, data); err != nil {
		return nil, fmt.Errorf("migrate: output does not satisfy the canonical schema: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(outFile, data, 0o644); err != nil {
		return nil, err
	}
	reportData, encErr := report.Encode()
	if encErr != nil {
		return nil, fmt.Errorf("migrate: encode entity report: %w", encErr)
	}
	reportFile := filepath.Join(outDir, enrich.ReportFileName)
	if err := os.WriteFile(reportFile, reportData, 0o644); err != nil {
		return nil, err
	}
	readme := filepath.Join(outDir, "README.md")
	if err := os.WriteFile(readme, []byte(readmeFor(n, g, inPath)), 0o644); err != nil {
		return nil, err
	}
	res := &Result{
		GuideFile:  outFile,
		ReadmeFile: readme,
		ReportFile: reportFile,
		Entities:   report.Counts.Tally,
		Variants:   len(g.Variants),
		Sections:   len(g.Sections),
		Steps:      g.StepCount(),
	}
	for _, segment := range g.Segments() {
		res.Segments++
		if segment.Kind == content.SegmentBankLeg {
			res.BankLegs++
		}
		if segment.Loadout != nil {
			switch segment.Loadout.Status {
			case content.LoadoutPartial:
				res.Partial++
			case content.LoadoutUnknown:
				res.Unknown++
			}
		}
	}
	for _, st := range g.Steps() {
		if st.IsManual() {
			res.Manual++
		} else {
			res.WithRules++
		}
	}
	return res, nil
}

func readmeFor(n *Normalized, g *content.Guide, inPath string) string {
	withRules := 0
	for _, st := range g.Steps() {
		if !st.IsManual() {
			withRules++
		}
	}
	segments, bankLegs, partial, unknown := 0, 0, 0, 0
	for _, segment := range g.Segments() {
		segments++
		if segment.Kind == content.SegmentBankLeg {
			bankLegs++
		}
		if segment.Loadout != nil {
			switch segment.Loadout.Status {
			case content.LoadoutPartial:
				partial++
			case content.LoadoutUnknown:
				unknown++
			}
		}
	}
	return fmt.Sprintf(`# %s

Guide id: `+"`%s`"+` · content version `+"`%s`"+` · %d source variant · %d sections · %d segments (%d explicit bank legs) · %d steps (%d with automatic rules, %d manual).

## Attribution

- Author: **%s** (the original "OSRS Ironman Efficiency Guide", first published as a public Google Doc).
- Republished, reorganized and extended ("enhanced" steps, update notes) by **%s** (%s).
- No explicit license is declared by either layer. This copy is for personal, self-hosted use only:
  keep this attribution, show it in the UI, do not redistribute publicly or use commercially.

## Provenance

- Extracted from %s on %s (`+"`tools/extract_page.py`"+`, raw pages under `+"`data/guide/`"+`).
- Normalised by `+"`tools/normalize_guide.py`"+` into `+"`%s`"+` (schema v%s).
- Migrated to the canonical schema (`+"`content/schema/guide.schema.json`"+`) by
  `+"`ironman content migrate`"+` (package `+"`internal/core/content/migrate`"+`).
  Field mapping: docs/05-conteudo-e-normalizacao.md, "Mapeamento para o schema canônico (Go)".
- Explicit bank/deposit/withdraw words deterministically start a bank-leg
  segment. The checked-in result contains %d partial loadouts and %d explicitly
  unknown loadouts; absent requirements are never represented as empty arrays.
- Bank actions and structured loadouts retain source step ids, confidence and
  derivation evidence. Segment ids are fingerprints of the section and first
  stable step id, independent of array positions.

## Editing

`+"`guide.json`"+` is generated. Do not hand-edit it while the normalizer is still the source of truth;
fix the extraction/normalisation and re-run:

    go run ./cmd/ironman content migrate
    go run ./cmd/ironman content validate

Step ids are stable and inherited from ironman.guide; never reuse one.
`, g.Title, g.ID, g.Version, len(g.Variants), len(g.Sections), segments, bankLegs, g.StepCount(), withRules, g.StepCount()-withRules,
		n.Guide.Source.Author, n.Guide.Source.Maintainer, n.Guide.Source.URL,
		n.Guide.Source.URL, n.Guide.Source.ExtractedAt, filepath.ToSlash(inPath), n.SchemaVersion, partial, unknown)
}
