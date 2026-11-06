package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	"github.com/FelipeMalacarne/ironman-planner/internal/sources/hiscores"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/ironman-planner/internal/store/progress"
	"github.com/FelipeMalacarne/ironman-planner/internal/store/syncrun"
)

// updateContract rewrites the web's contract fixtures:
//
//	DATABASE_URL=... go test ./internal/httpapi -run TestContract -update
var updateContract = flag.Bool("update", false, "rewrite web/src/lib/api/contract from the live API")

// contractDir holds one TypeScript module per API answer the web reads. Each
// is a real answer of this test's server, written as
// `export const fixture = {...} satisfies <web type>`, so svelte-check fails
// when the web's hand-written types and the Go answers drift apart: a missing
// field, a null where the type has none, an enum value outside the union or a
// field the type does not know (docs/10 section 12, decision E4).
const contractDir = "../../web/src/lib/api/contract"

// contractCase is one answer: the request, and the web type it must satisfy.
type contractCase struct {
	name, method, path, body string
	tsImports                []string // names imported from web/src/lib/progress/types
	tsType                   string
}

// TestContractPostgres runs the real stores on real content against
// Postgres, makes the state the web shows (a plugin push, marks, a goal, a
// sync) and checks every answer against its fixture: the fixture's JSON shape
// must equal the live one (-update rewrites them).
func TestContractPostgres(t *testing.T) {
	ctx := context.Background()
	pool := authTestPool(ctx, t)
	cnt, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('dev', 'dev', 'dev') ON CONFLICT DO NOTHING;
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_contract', 'dev', 'ironman260', 'ironman260', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, cnt, catalog)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = progress.ReevaluateAll(ctx); err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress)
	syncs := syncrun.New(pool, contractFetcher{}, progress)
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, DefaultGuideID: "oziris-main", Ingest: ingest, Holdings: ingest,
		Progress: progress, Sync: syncs, Accounts: progress, Catalog: catalog, DevMode: true,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()

	// A plugin push with skills, quests and a bank, as the plugin sends it.
	token, err := ingest.CreateToken(ctx, "acc_contract", "contract", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	push := contractPush(manifest.ManifestVersion)
	raw, _ := json.Marshal(push)
	if resp, err := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); err != nil || !resp.OK {
		t.Fatalf("push: %+v, %v", resp, err)
	}

	call := func(method, path, body string) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, srv.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		out, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			t.Fatalf("%s %s: HTTP %d %s", method, path, resp.StatusCode, out)
		}
		return out
	}
	const acc = "?account=acc_contract"
	var board struct {
		Cursor struct {
			StepID string `json:"step_id"`
		} `json:"cursor"`
		Steps []struct {
			ID        string `json:"id"`
			SectionID string `json:"section_id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(call("GET", "/api/guides/oziris-main/board"+acc, ""), &board); err != nil || board.Cursor.StepID == "" {
		t.Fatalf("board cursor: %v", err)
	}
	cursor, section := board.Cursor.StepID, ""
	for _, s := range board.Steps {
		if s.ID == cursor {
			section = s.SectionID
		}
	}
	// The owner marks the step before the next one, so the board has an owner
	// mark, an event and a command.
	call("PATCH", "/api/guides/oziris-main/steps/"+cursor+"/mark"+acc, `{"kind":"manual_done"}`)
	var goal struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(call("POST", "/api/goals"+acc, `{"objective_key":"skill:magic>=43"}`), &goal); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call("GET", "/api/guides/oziris-main/board"+acc, ""), &board); err != nil {
		t.Fatal(err)
	}
	next := board.Cursor.StepID

	cases := []contractCase{
		{name: "guides", method: "GET", path: "/api/guides", tsImports: []string{"GuideInfo"}, tsType: "{ guides: GuideInfo[] }"},
		{name: "section", method: "GET", path: "/api/guides/oziris-main/sections/" + section, tsImports: []string{"ContentSection"}, tsType: "ContentSection"},
		{name: "progress", method: "GET", path: "/api/progress" + acc, tsImports: []string{"ProgressSummary"}, tsType: "ProgressSummary"},
		{name: "board", method: "GET", path: "/api/guides/oziris-main/board" + acc, tsImports: []string{"Board"}, tsType: "Board"},
		{name: "mark", method: "PATCH", path: "/api/guides/oziris-main/steps/" + next + "/mark" + acc, body: `{"kind":"skipped"}`, tsImports: []string{"Board"}, tsType: "Board"},
		{name: "mark-before", method: "POST", path: "/api/guides/oziris-main/marks" + acc, body: fmt.Sprintf(`{"before_step_id":%q}`, board.Steps[len(board.Steps)/8].ID), tsImports: []string{"Board"}, tsType: "Board"},
		{name: "events", method: "GET", path: "/api/guides/oziris-main/events" + acc + "&limit=20", tsImports: []string{"EventsView"}, tsType: "EventsView"},
		{name: "readiness", method: "GET", path: "/api/guides/oziris-main/readiness" + acc + "&step=" + next, tsImports: []string{"Readiness"}, tsType: "Readiness"},
		{name: "goals", method: "GET", path: "/api/goals" + acc, tsImports: []string{"GoalsResponse"}, tsType: "GoalsResponse"},
		{name: "goal-plan", method: "GET", path: "/api/goals/" + goal.ID + "/plan" + acc, tsImports: []string{"GoalPlanResponse"}, tsType: "GoalPlanResponse"},
		{name: "world", method: "GET", path: "/api/world" + acc, tsImports: []string{"WorldResponse"}, tsType: "WorldResponse"},
		{name: "graph", method: "GET", path: "/api/world/graph" + acc + "&focus=quest:recipe_for_disaster@finished&depth=3", tsImports: []string{"GraphResponse"}, tsType: "GraphResponse"},
		{name: "sync", method: "POST", path: "/api/sync" + acc, tsImports: []string{"SyncRun"}, tsType: "SyncRun"},
		{name: "sync-runs", method: "GET", path: "/api/sync/runs" + acc + "&limit=5", tsImports: []string{"SyncRunsResponse"}, tsType: "SyncRunsResponse"},
	}
	for _, c := range cases {
		var live any
		if err := json.Unmarshal(call(c.method, c.path, c.body), &live); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		live = compact(live)
		file := filepath.Join(contractDir, c.name+".ts")
		if *updateContract {
			if err := os.MkdirAll(contractDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, contractModule(c, live), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		stored, err := readContract(file)
		if err != nil {
			t.Fatalf("%s: %v (run with -update)", c.name, err)
		}
		if got, want := shape(live), shape(stored); got != want {
			t.Errorf("%s: the API answer changed shape; run `go test ./internal/httpapi -run TestContract -update`, then `pnpm check`:\nlive:    %s\nfixture: %s", c.name, got, want)
		}
	}
}

// contractFetcher answers a hiscores sample.
type contractFetcher struct{}

func (contractFetcher) Fetch(context.Context, string, string) (hiscores.Result, error) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "data", "samples", "hiscores", "ironman260.json"))
	return hiscores.Result{Raw: raw, Table: "hiscore_oldschool_ironman"}, err
}

func contractPush(manifestVersion int64) coreingest.Push {
	sentAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	revision := int64(1)
	levels := coreingest.IntMap{"attack": 20, "strength": 20, "defence": 10, "magic": 25, "firemaking": 30, "woodcutting": 30}
	xp := coreingest.IntMap{"attack": 4470, "strength": 4470, "defence": 1154, "magic": 8740, "firemaking": 13363, "woodcutting": 13363}
	empty := coreingest.IntMap{}
	quests := coreingest.QuestMap{"Cook's Assistant": coreingest.QuestFinished, "X Marks the Spot": coreingest.QuestInProgress}
	bank := coreingest.QuantityMap{"952": 1, "590": 1, "1351": 1, "995": 25000}
	inv := coreingest.QuantityMap{"590": 1}
	equip := coreingest.QuantityMap{}
	obs := func(q *coreingest.QuantityMap) *coreingest.ItemObservation {
		return &coreingest.ItemObservation{State: coreingest.ItemLive, ObservedAt: sentAt, Revision: &revision, LiveEpoch: "login-1", Quantities: q}
	}
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion, Username: "ironman260", Profile: "STANDARD",
		ManifestVersion: manifestVersion, ClientSeq: 1, Kind: coreingest.PushFull, SentAt: sentAt,
		Data: coreingest.PushData{
			Varbits: &empty, Varps: &empty, Levels: &levels, XP: &xp, Boosted: &empty, Quests: &quests,
			Items: &coreingest.ItemContainers{Bank: obs(&bank), Inventory: obs(&inv), Equipment: obs(&equip)},
		},
	}
}

// compact keeps, in every array, only the elements that add a shape the ones
// before did not have (at most 6), so a fixture stays small but still covers
// every variant of its elements (a step with and without objectives, ...).
func compact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = compact(e)
		}
		return out
	case []any:
		seen := map[string]bool{}
		out := []any{}
		for _, e := range x {
			e = compact(e)
			s := shape(e)
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, e)
			if len(out) == 6 {
				break
			}
		}
		return out
	default:
		return v
	}
}

// shape is the JSON type skeleton of a value: object keys with their shapes,
// the set of element shapes of an array, and the type of a scalar.
func shape(v any) string {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ":" + shape(x[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		set := map[string]bool{}
		for _, e := range x {
			set[shape(e)] = true
		}
		elems := make([]string, 0, len(set))
		for s := range set {
			elems = append(elems, s)
		}
		sort.Strings(elems)
		return "[" + strings.Join(elems, "|") + "]"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

const contractPrefix = "export const fixture = "

func contractModule(c contractCase, v any) []byte {
	body, _ := json.MarshalIndent(v, "", "\t")
	var b bytes.Buffer
	fmt.Fprintf(&b, "// A real answer of %s %s, written by\n", c.method, strings.SplitN(c.path, "?", 2)[0])
	b.WriteString("// `DATABASE_URL=... go test ./internal/httpapi -run TestContract -update`. Do not edit:\n")
	b.WriteString("// svelte-check verifies it against the web's type (docs/10 section 12).\n")
	fmt.Fprintf(&b, "import type { %s } from '../../progress/types';\n\n", strings.Join(c.tsImports, ", "))
	b.WriteString(contractPrefix)
	b.Write(body)
	fmt.Fprintf(&b, " satisfies %s;\n", c.tsType)
	return b.Bytes()
}

func readContract(file string) (any, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	s := string(raw)
	i := strings.Index(s, contractPrefix)
	j := strings.LastIndex(s, " satisfies ")
	if i < 0 || j < i {
		return nil, fmt.Errorf("%s is not a contract module", file)
	}
	var v any
	if err := json.Unmarshal([]byte(s[i+len(contractPrefix):j]), &v); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return v, nil
}
