// Package bruhsailer deterministically imports the pinned BRUHsailer DOCX
// snapshots into the planner's canonical guide JSON shape.
package bruhsailer

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	corecontent "github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/content/derive"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities/enrich"
)

const (
	GuideID       = "bruhsailer-main"
	GuideRevision = "b697c0a63f120a9e35233503f13c1aeaf224bb2d"
	GuideVersion  = "2026.11.4"
	ImportedAt    = "2026-11-04"
	SourceURL     = "https://github.com/Osrsper/BRUHsailer/tree/" + GuideRevision
	SourceAuthor  = "So Iron BRUH and ParasailerOSRS"
	LicenseNote   = "No explicit licence declared upstream; personal, self-hosted use only. Keep attribution; do not redistribute publicly or use commercially without the authors' permission." //nolint:misspell // upstream wording, reproduced in generated content
)

var PinnedSources = []SourceSpec{
	{Chapter: 1, Filename: "Chapter1.docx", SHA256: "9ae5795939a4d19fb97014e9b74f6b0adb536eb12ae8010511497b54c8e0e659"},
	{Chapter: 2, Filename: "Chapter2.docx", SHA256: "35c373ba47f9d11cf0d29a8e87e26a1b17050eae96161d98a7757eb37ff112b4"},
	{Chapter: 3, Filename: "Chapter3.docx", SHA256: "644e6160e9aa3c039a5795b5b381aa218e0e6351d063189ad1f718a2da023852"},
}

type SourceSpec struct {
	Chapter  int
	Filename string
	SHA256   string
}

type Result struct {
	GuideFile    string
	SourceFiles  int
	Sections     int
	Steps        int
	ChapterSteps map[int]int
	Metadata     MetadataCounts
	Variants     int
	Segments     int
	BankLegs     int
	Partial      int
	Unknown      int
	// Entities counts entity references; EntityReport is the
	// entities.report.json listing the ambiguous and unresolved ones.
	Entities     enrich.Tally
	EntityReport []byte
	ReportFile   string
}

type MetadataCounts struct {
	GP     int
	Items  int
	Skills int
	Time   int
	Links  int
}

type Guide struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Version       string    `json:"version"`
	DefaultModes  []string  `json:"default_modes"`
	DefaultLayers []string  `json:"default_layers"`
	Source        Source    `json:"source"`
	Sections      []Section `json:"sections"`
}

type Source struct {
	Origin     string `json:"origin,omitempty"`
	OriginalID string `json:"original_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Author     string `json:"author,omitempty"`
	License    string `json:"license,omitempty"`
	ImportedAt string `json:"imported_at,omitempty"`
}

type Section struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Steps       []Step `json:"steps"`
}

type Step struct {
	ID     string   `json:"id"`
	Text   string   `json:"text"`
	Notes  []Note   `json:"notes,omitempty"`
	Items  []string `json:"items,omitempty"`
	Links  []Link   `json:"links,omitempty"`
	Source Source   `json:"source"`
}

type Note struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type Link struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

// Run imports sourceDir and atomically writes canonical JSON to outputPath.
func Run(sourceDir, outputPath string) (Result, error) {
	data, result, err := Generate(sourceDir)
	if err != nil {
		return Result{}, err
	}
	if err = os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".bruhsailer-*.json")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary guide: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return Result{}, fmt.Errorf("write temporary guide: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Result{}, fmt.Errorf("close temporary guide: %w", err)
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return Result{}, fmt.Errorf("replace guide: %w", err)
	}
	result.GuideFile = outputPath
	result.ReportFile = filepath.Join(filepath.Dir(outputPath), enrich.ReportFileName)
	if err := os.WriteFile(result.ReportFile, result.EntityReport, 0o644); err != nil {
		return Result{}, fmt.Errorf("write entity report: %w", err)
	}
	return result, nil
}

// Generate returns the canonical JSON bytes without mutating the filesystem.
func Generate(sourceDir string) ([]byte, Result, error) {
	chapters := make([]rawChapter, 0, len(PinnedSources))
	for _, spec := range PinnedSources {
		path := filepath.Join(sourceDir, spec.Filename)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, Result{}, fmt.Errorf("read pinned source %s: %w", spec.Filename, err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != spec.SHA256 {
			return nil, Result{}, fmt.Errorf("pinned source %s has SHA-256 %s, want %s", spec.Filename, got, spec.SHA256)
		}
		chapter, err := parseChapter(data, spec.Chapter)
		if err != nil {
			return nil, Result{}, fmt.Errorf("parse %s: %w", spec.Filename, err)
		}
		chapters = append(chapters, chapter)
	}

	guide, result, err := buildGuide(chapters)
	if err != nil {
		return nil, Result{}, err
	}
	legacy, err := json.Marshal(guide)
	if err != nil {
		return nil, Result{}, fmt.Errorf("encode legacy guide: %w", err)
	}
	var canonical corecontent.Guide
	if err = json.Unmarshal(legacy, &canonical); err != nil {
		return nil, Result{}, fmt.Errorf("promote guide hierarchy: %w", err)
	}
	stats := derive.EnrichGuide(&canonical)
	result.Variants = stats.Variants
	result.Segments = stats.Segments
	result.BankLegs = stats.BankLegs
	result.Partial = stats.PartialLoadouts
	result.Unknown = stats.UnknownLoadouts
	reg, err := entities.Default()
	if err != nil {
		return nil, Result{}, err
	}
	report := enrich.Guide(&canonical, reg)
	result.Entities = report.Counts.Tally
	if result.EntityReport, err = report.Encode(); err != nil {
		return nil, Result{}, fmt.Errorf("encode entity report: %w", err)
	}
	buf, err := json.MarshalIndent(&canonical, "", "  ")
	if err != nil {
		return nil, Result{}, fmt.Errorf("encode guide: %w", err)
	}
	buf = append(buf, '\n')
	result.SourceFiles = len(PinnedSources)
	return buf, result, nil
}

type rawChapter struct {
	Number   int
	Title    string
	Sections []rawSection
}

type rawSection struct {
	Ref   string
	Title string
	Steps []rawStep
}

type rawStep struct {
	Bullets  []rawBullet
	Metadata map[string]string
}

type rawBullet struct {
	Level int
	Text  string
	Links []rawLink
}

type rawLink struct {
	URL  string
	Text string
}

type paragraph struct {
	Text        string
	BulletLevel *int
	Links       []rawLink
}

var (
	sectionPattern = regexp.MustCompile(`^\s*(\d+\.\d+)\s*:\s*(\S.*)$`)
	eocPattern     = regexp.MustCompile(`(?i)^=*\s*end of chapter`)
	sectionReject  = regexp.MustCompile(`(?i)(start of section|for section|of section)`)
	sentenceEnd    = regexp.MustCompile(`^\.\s+[A-Z0-9]`)
	metaPatterns   = []struct {
		key string
		re  *regexp.Regexp
	}{
		{"gp", regexp.MustCompile(`(?i)^\s*GP\s*(?:stack|after)[^:]*:\s*(.*)$`)},
		{"items", regexp.MustCompile(`(?i)^\s*Items\s*needed[^:]*:\s*(.*)$`)},
		{"skills", regexp.MustCompile(`(?i)^\s*Skills\s*/\s*quests[^:]*:\s*(.*)$`)},
		{"time", regexp.MustCompile(`(?i)^\s*Total\s+time[^:]*:\s*(.*)$`)},
	}
)

func parseChapter(data []byte, chapterNumber int) (rawChapter, error) {
	documentXML, err := zipEntry(data, "word/document.xml")
	if err != nil {
		return rawChapter{}, err
	}
	relsXML, err := zipEntry(data, "word/_rels/document.xml.rels")
	if err != nil {
		return rawChapter{}, err
	}
	rels, err := parseRelationships(relsXML)
	if err != nil {
		return rawChapter{}, err
	}
	paragraphs, err := parseParagraphs(documentXML, rels)
	if err != nil {
		return rawChapter{}, err
	}

	chapter := rawChapter{Number: chapterNumber}
	var current *rawSection
	var bullets []rawBullet
	metadata := map[string]string{}
	inEOC := false
	closeStep := func() {
		if current != nil && len(bullets) > 0 && len(metadata) > 0 {
			current.Steps = append(current.Steps, rawStep{Bullets: bullets, Metadata: metadata})
		}
		bullets = nil
		metadata = map[string]string{}
	}
	closeSection := func() {
		if current != nil {
			chapter.Sections = append(chapter.Sections, *current)
			current = nil
		}
	}

	for _, p := range paragraphs {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		if chapter.Title == "" {
			chapter.Title = text
			continue
		}
		if !inEOC && eocPattern.MatchString(text) {
			closeStep()
			closeSection()
			inEOC = true
			continue
		}
		if inEOC {
			continue
		}
		if match := sectionPattern.FindStringSubmatch(text); match != nil {
			closeStep()
			closeSection()
			current = &rawSection{Ref: match[1], Title: strings.TrimSpace(match[2])}
			continue
		}
		if found := classifyMetadata(text); len(found) > 0 {
			for key, value := range found {
				metadata[key] = value
			}
			continue
		}
		if p.BulletLevel != nil {
			if len(metadata) > 0 && len(bullets) > 0 {
				closeStep()
			}
			bullets = append(bullets, rawBullet{Level: *p.BulletLevel, Text: text, Links: p.Links})
			continue
		}
		if len(bullets) > 0 && len(metadata) == 0 {
			bullets = nil
		}
	}
	closeStep()
	closeSection()
	if chapter.Title == "" {
		return rawChapter{}, errors.New("chapter title not found")
	}
	if len(chapter.Sections) == 0 {
		return rawChapter{}, errors.New("no sections found")
	}
	return chapter, nil
}

func classifyMetadata(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if colon := strings.IndexByte(line, ':'); colon > 0 && sectionReject.MatchString(line[:colon]) {
			continue
		}
		for _, pattern := range metaPatterns {
			if match := pattern.re.FindStringSubmatch(line); match != nil {
				out[pattern.key] = strings.TrimSpace(match[1])
				break
			}
		}
	}
	return out
}

func buildGuide(chapters []rawChapter) (Guide, Result, error) {
	guide := Guide{
		SchemaVersion: 1,
		ID:            GuideID,
		Slug:          GuideID,
		Title:         "BRUHsailer Ironman Guide",
		Description:   "Independent import of the BRUHsailer route by So Iron BRUH and ParasailerOSRS from pinned public Google Docs exports.",
		Version:       GuideVersion,
		DefaultModes:  []string{"ironman"},
		DefaultLayers: []string{"classic"},
		Source: Source{
			Origin:     "other",
			OriginalID: GuideRevision,
			URL:        SourceURL,
			Author:     SourceAuthor,
			License:    LicenseNote,
			ImportedAt: ImportedAt,
		},
	}
	result := Result{ChapterSteps: map[int]int{}}
	seenIDs := map[string]int{}

	for _, chapter := range chapters {
		chapterStep := 0
		for _, sourceSection := range chapter.Sections {
			sectionID := "bruh-" + sourceSection.Ref
			section := Section{
				ID:          sectionID,
				Slug:        slugify(sourceSection.Title),
				Title:       sourceSection.Title,
				Description: chapter.Title,
				Kind:        "main",
			}
			for _, sourceStep := range sourceSection.Steps {
				chapterStep++
				text := formatBullets(sourceStep.Bullets)
				if text == "" {
					return Guide{}, Result{}, fmt.Errorf("chapter %d section %s contains an empty step", chapter.Number, sourceSection.Ref)
				}
				stepID := stableStepID(sectionID, sourceStep)
				seenIDs[stepID]++
				if seenIDs[stepID] > 1 {
					stepID = fmt.Sprintf("%s-%d", stepID, seenIDs[stepID])
				}
				step := Step{
					ID:   stepID,
					Text: text,
					Source: Source{
						Origin:     "other",
						OriginalID: fmt.Sprintf("CH%d.%d", chapter.Number, chapterStep),
					},
				}
				step.Links = collectLinks(sourceStep.Bullets)
				result.Metadata.Links += len(step.Links)
				appendMetadata(&step, sourceStep.Metadata, &result.Metadata)
				section.Steps = append(section.Steps, step)
			}
			guide.Sections = append(guide.Sections, section)
			result.Sections++
		}
		result.ChapterSteps[chapter.Number] = chapterStep
		result.Steps += chapterStep
	}
	return guide, result, nil
}

func formatBullets(bullets []rawBullet) string {
	if len(bullets) == 0 {
		return ""
	}
	minLevel := bullets[0].Level
	for _, bullet := range bullets[1:] {
		if bullet.Level < minLevel {
			minLevel = bullet.Level
		}
	}
	lines := make([]string, 0, len(bullets))
	for _, bullet := range bullets {
		indent := strings.Repeat("  ", bullet.Level-minLevel)
		text := strings.ReplaceAll(strings.TrimSpace(bullet.Text), "\n", " ")
		lines = append(lines, indent+"- "+text)
	}
	return strings.Join(lines, "\n")
}

func stableStepID(sectionID string, step rawStep) string {
	var material strings.Builder
	material.WriteString(sectionID)
	material.WriteByte('\n')
	for _, bullet := range step.Bullets {
		fmt.Fprintf(&material, "%d:", bullet.Level)
		material.WriteString(normalizeFingerprintText(bullet.Text))
		material.WriteByte('\n')
	}
	keys := make([]string, 0, len(step.Metadata))
	for key := range step.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		material.WriteString(key)
		material.WriteByte(':')
		material.WriteString(normalizeFingerprintText(step.Metadata[key]))
		material.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(material.String()))
	return sectionID + "-" + hex.EncodeToString(sum[:6])
}

func normalizeFingerprintText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func appendMetadata(step *Step, metadata map[string]string, counts *MetadataCounts) {
	if value := metadata["gp"]; value != "" {
		step.Notes = append(step.Notes, Note{Kind: "tip", Text: "GP after step: " + value})
		counts.GP++
	}
	if value := metadata["items"]; value != "" {
		step.Notes = append(step.Notes, Note{Kind: "tip", Text: "Items needed: " + value})
		counts.Items++
		if !isEmptyItemValue(value) {
			step.Items = append(step.Items, splitItems(value)...)
		}
	}
	if value := metadata["skills"]; value != "" {
		step.Notes = append(step.Notes, Note{Kind: "tip", Text: "Skills / quests: " + value})
		counts.Skills++
	}
	if value := metadata["time"]; value != "" {
		step.Notes = append(step.Notes, Note{Kind: "tip", Text: "Total time: " + value})
		counts.Time++
	}
}

// splitItems splits an "Items needed" value into items. Commas and
// semicolons separate items only outside parentheses, so a parenthetical
// remark stays with its item; a sentence end (". " before a capital or a
// digit) separates too, and a trailing period is dropped.
func splitItems(value string) []string {
	var (
		items []string
		start int
		depth int
	)
	add := func(item string) {
		item = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(item), "."))
		if item != "" {
			items = append(items, item)
		}
	}
	for i, r := range value {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth = max(depth-1, 0)
		case depth > 0:
		case r == ',' || r == ';' || r == '.' && sentenceEnd.MatchString(value[i:]):
			add(value[start:i])
			start = i + 1
		}
	}
	add(value[start:])
	return items
}

func isEmptyItemValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "n/a", "na", "tbd", "-":
		return true
	default:
		return false
	}
}

func collectLinks(bullets []rawBullet) []Link {
	var links []Link
	seen := map[string]bool{}
	for _, bullet := range bullets {
		for _, sourceLink := range bullet.Links {
			if sourceLink.URL == "" {
				continue
			}
			key := sourceLink.URL + "\x00" + sourceLink.Text
			if seen[key] {
				continue
			}
			seen[key] = true
			links = append(links, Link{Type: linkType(sourceLink.URL), URL: sourceLink.URL, Text: strings.TrimSpace(sourceLink.Text)})
		}
	}
	return links
}

func linkType(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		host := strings.ToLower(parsed.Hostname())
		path := strings.ToLower(parsed.Path)
		if strings.Contains(host, "oldschool.runescape.wiki") {
			return "wiki"
		}
		if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".gif") || strings.HasSuffix(path, ".webp") {
			return "image"
		}
		if strings.Contains(host, "docs.google.com") || strings.Contains(host, "drive.google.com") {
			return "guide"
		}
		if strings.Contains(strings.ToLower(rawURL), "map") {
			return "map"
		}
	}
	return "other"
}

func slugify(value string) string {
	var out strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == ':' || r == ',' || r == '/' || r == '&':
			if out.Len() > 0 && !lastHyphen {
				out.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

func zipEntry(data []byte, name string) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open DOCX archive: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		value, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		return value, nil
	}
	return nil, fmt.Errorf("DOCX entry %s not found", name)
}

type relationshipsXML struct {
	Relationships []relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

func parseRelationships(data []byte) (map[string]string, error) {
	var document relationshipsXML
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode document relationships: %w", err)
	}
	result := map[string]string{}
	for _, relationship := range document.Relationships {
		if strings.Contains(relationship.Type, "hyperlink") {
			result[relationship.ID] = relationship.Target
		}
	}
	return result, nil
}

func parseParagraphs(data []byte, relationships map[string]string) ([]paragraph, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var paragraphs []paragraph
	var current *paragraph
	var text strings.Builder
	var currentLinkID string
	var linkText strings.Builder
	inText := false
	flushLink := func() {
		if current == nil || currentLinkID == "" {
			return
		}
		if target := relationships[currentLinkID]; target != "" {
			current.Links = append(current.Links, rawLink{URL: target, Text: linkText.String()})
		}
		currentLinkID = ""
		linkText.Reset()
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode word/document.xml: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				current = &paragraph{}
				text.Reset()
			case "numPr":
				if current != nil && current.BulletLevel == nil {
					level := 0
					current.BulletLevel = &level
				}
			case "ilvl":
				if current != nil {
					for _, attr := range value.Attr {
						if attr.Name.Local == "val" {
							var level int
							if _, err := fmt.Sscanf(attr.Value, "%d", &level); err == nil {
								current.BulletLevel = &level
							}
						}
					}
				}
			case "hyperlink":
				flushLink()
				for _, attr := range value.Attr {
					if attr.Name.Local == "id" {
						currentLinkID = attr.Value
					}
				}
			case "t":
				inText = true
			case "tab":
				if current != nil {
					text.WriteByte('\t')
					if currentLinkID != "" {
						linkText.WriteByte('\t')
					}
				}
			case "br":
				if current != nil {
					text.WriteByte('\n')
					if currentLinkID != "" {
						linkText.WriteByte('\n')
					}
				}
			}
		case xml.CharData:
			if current != nil && inText {
				text.Write([]byte(value))
				if currentLinkID != "" {
					linkText.Write([]byte(value))
				}
			}
		case xml.EndElement:
			switch value.Name.Local {
			case "t":
				inText = false
			case "hyperlink":
				flushLink()
			case "p":
				flushLink()
				if current != nil {
					current.Text = text.String()
					paragraphs = append(paragraphs, *current)
				}
				current = nil
				text.Reset()
			}
		}
	}
	return paragraphs, nil
}
