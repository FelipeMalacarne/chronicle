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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	"github.com/FelipeMalacarne/chronicle/internal/sources/hiscores"
	storeactivity "github.com/FelipeMalacarne/chronicle/internal/store/activity"
	storeidentity "github.com/FelipeMalacarne/chronicle/internal/store/identity"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeplans "github.com/FelipeMalacarne/chronicle/internal/store/plans"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
	storerhythm "github.com/FelipeMalacarne/chronicle/internal/store/rhythm"
	"github.com/FelipeMalacarne/chronicle/internal/store/syncrun"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
	"github.com/FelipeMalacarne/chronicle/internal/workerctl"
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
	tsFrom                   string   // the module of tsImports, relative to contractDir; "" is ../../progress/types
	tsImports                []string // names imported from tsFrom
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
	workers, control := contractWorkers(ctx, t, pool, ingest)
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	link := &agent.Link{Log: quiet}
	srv := httptest.NewServer(New(Options{
		Version: "test", Content: cnt, DefaultGuideID: "oziris-main", Ingest: ingest, Holdings: ingest,
		Progress: progress, Sync: syncs, Accounts: progress, Catalog: catalog, DevMode: true, Logger: quiet,
		Identity: storeidentity.New(pool), Rhythm: storerhythm.New(pool, catalog), Levels: storeactivity.New(pool),
		Microbot: &MicrobotOptions{Link: link, Audit: &memAudit{}, Sessions: workers, Control: control, WorkerImage: contractWorkerImage},
		Plans:    storeplans.New(pool, cnt, catalog),
	}))
	defer srv.Close()
	// The running worker's agent and the one attached on zaros, for /api/workers.
	startAgent(t, srv.URL, "", agent.Hello{Host: "chronicle-worker-acc-worker-a-7d9f-x2k4q", Account: "acc_worker_a", Kind: agent.KindManaged, Worker: "chronicle-worker-acc-worker-a-7d9f-x2k4q", Bridge: "1.11.0"}, "W")
	startAgent(t, srv.URL, "", agent.Hello{Host: "zaros", Account: "acc_contract"}, "Z")
	waitLinked(t, link, 2)

	// A plugin push with skills, quests and a bank, as the plugin sends it.
	token, err := ingest.CreateToken(ctx, "acc_contract", "contract", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, push := range []coreingest.Push{contractPush(manifest.ManifestVersion), contractSessionPush(manifest.ManifestVersion)} {
		raw, _ := json.Marshal(push)
		if resp, err := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); err != nil || !resp.OK {
			t.Fatalf("push %d: %+v, %v", push.ClientSeq, resp, err)
		}
	}
	contractLevels(ctx, t, pool, ingest)

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
	// Another account of the dev user pursues a quest, for the Mundo da frota.
	call("POST", "/api/goals?account=acc_worker_a", `{"objective_key":"quest:dragon_slayer_i@finished"}`)
	if err := json.Unmarshal(call("GET", "/api/guides/oziris-main/board"+acc, ""), &board); err != nil {
		t.Fatal(err)
	}
	next := board.Cursor.StepID
	// A custom plan the account follows (docs/10 section 9): the feathers
	// it carries and the axe in its bank meet the first two tasks, so the
	// next one is the walk to Draynor.
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(call("POST", "/api/plans"+acc, `{"kind":"custom","title":"Logística","items":[
		{"kind":"task","task":{"type":"bank","items":[{"name":"Feather","item_id":314,"qty":10}]}},
		{"kind":"task","task":{"type":"buy","shop":"shop:bobs_brilliant_axes","item":"item:1351","qty":1}},
		{"kind":"task","task":{"type":"travel","location":"location:draynor_village"}},
		{"kind":"step","guide":"oziris-main","step":"`+next+`"}]}`), &created); err != nil {
		t.Fatal(err)
	}
	call("PUT", "/api/plans/active"+acc, `{"plan":"`+created.ID+`"}`)

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
		{name: "sync", method: "POST", path: "/api/sync" + acc, tsImports: []string{"SyncRun"}, tsType: "SyncRun"},
		{name: "sync-runs", method: "GET", path: "/api/sync/runs" + acc + "&limit=5", tsImports: []string{"SyncRunsResponse"}, tsType: "SyncRunsResponse"},
		{name: "account-levels", method: "GET", path: "/api/account/levels?account=acc_levels", tsImports: []string{"AccountLevels"}, tsType: "AccountLevels"},
		{name: "fleet-levels", method: "GET", path: "/api/levels", tsImports: []string{"FleetLevels"}, tsType: "FleetLevels"},
		{name: "workers", method: "GET", path: "/api/workers", tsFrom: "../microbot", tsImports: []string{"WorkersView"}, tsType: "WorkersView"},
		{name: "account-rhythm", method: "GET", path: "/api/account/rhythm" + acc, tsImports: []string{"AccountRhythm"}, tsType: "AccountRhythm"},
		{name: "fleet-rhythm", method: "GET", path: "/api/rhythm", tsImports: []string{"FleetRhythm"}, tsType: "FleetRhythm"},
		{name: "fleet-goals", method: "GET", path: "/api/fleet/goals?keys=quest:cooks_assistant@finished", tsImports: []string{"FleetGoalsResponse"}, tsType: "FleetGoalsResponse"},
		{name: "plan-next", method: "GET", path: "/api/plan/next" + acc, tsImports: []string{"PlanNext"}, tsType: "PlanNext"},
		{name: "plans", method: "GET", path: "/api/plans" + acc, tsImports: []string{"PlansResponse"}, tsType: "PlansResponse"},
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

// contractWorkerImage is the server's WORKER_IMAGE in the contract test.
const contractWorkerImage = "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-def5678"

// contractWorkers makes the workers the Workers tab shows, on the real
// store: one running (session with a login, pod, agent), one stopped by an
// error on an older image (its pod still terminating) and one whose session
// was deleted.
func contractWorkers(ctx context.Context, t *testing.T, pool *pgxpool.Pool, tokens storeworkers.TokenMinter) (*storeworkers.Repository, *memControl) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer) VALUES
		('acc_worker_a', 'dev', 'Worker A', 'worker_a', 'ironman', 'oziris-main', 'enhanced'),
		('acc_worker_b', 'dev', 'Worker B', 'worker_b', 'ironman', 'oziris-main', 'enhanced'),
		('acc_worker_c', 'dev', 'Worker C', 'worker_c', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	repo := storeworkers.New(pool, testVaultKey(t), tokens)
	older := "ghcr.io/felipemalacarne/chronicle-worker:2.6.52-abc1234-0000000"
	for _, step := range []error{
		repo.PutSession(ctx, "acc_worker_a", "dev", agent.GameSession{Character: "Worker A", SessionID: "sid-a", CharacterID: "1"}),
		repo.SetDesired(ctx, "acc_worker_a", storeworkers.StateRunning, 330, contractWorkerImage),
		repo.RecordLogin(ctx, "acc_worker_a", "ok"),
		repo.PutSession(ctx, "acc_worker_b", "dev", agent.GameSession{Character: "Worker B", SessionID: "sid-b", CharacterID: "2"}),
		repo.RecordLogin(ctx, "acc_worker_b", "Invalid credentials"), // login_failed
		repo.SetDesired(ctx, "acc_worker_b", storeworkers.StateRunning, 0, older),
		repo.SetDesired(ctx, "acc_worker_b", storeworkers.StateStopped, 0, ""),
		repo.SetError(ctx, "acc_worker_b", "the session was refused"),
		repo.PutSession(ctx, "acc_worker_c", "dev", agent.GameSession{Character: "Worker C", SessionID: "sid-c", CharacterID: "3"}),
		repo.SetDesired(ctx, "acc_worker_c", storeworkers.StateStopped, 0, ""),
		repo.DeleteSession(ctx, "acc_worker_c"),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	// The saradomin quota of 2026-11-13 (2200m, 3328Mi, 4 Deployments) with
	// the app and one worker in it.
	cpuHard, cpuFree, memHard, memFree := 2.2, 1.48, int64(3328<<20), int64(2240<<20)
	control := &memControl{
		pods: map[string]workerctl.PodInfo{
			"acc_worker_a": {Deployment: "chronicle-worker-acc-worker-a", Pod: "chronicle-worker-acc-worker-a-7d9f-x2k4q", State: "running", Image: contractWorkerImage, Version: "v1.2.3", Restarts: 1, Seen: now},
			"acc_worker_b": {Deployment: "chronicle-worker-acc-worker-b", State: "terminating", Reason: "stopped", Image: older, Seen: now},
		},
		usage: map[string]workerctl.Usage{
			"acc_worker_a": {CPUCores: 0.404, MemoryBytes: 877350912, MeasuredAt: now.Add(-15 * time.Second)},
		},
		capacity: &workerctl.Capacity{
			Fits: 2, LimitedBy: "requests.cpu", CPUHardCores: &cpuHard, CPUFreeCores: &cpuFree,
			MemoryHardBytes: &memHard, MemoryFreeBytes: &memFree, WorkerCPUCores: 0.7, WorkerMemoryBytes: 1 << 30, Seen: now,
		},
	}
	return repo, control
}

// contractLevels makes the level-ups the Histórico and the Atividade show,
// on an account of its own: a full plugin push 30 hours ago (the baseline)
// and, an hour ago, a delta with Fishing 75 -> 76 and Farming 37 -> 39.
func contractLevels(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ingest *storeingest.Repository) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('acc_levels', 'dev', 'Felipe Pesca', 'felipe_pesca', 'ironman', 'oziris-main', 'enhanced')`); err != nil {
		t.Fatal(err)
	}
	token, err := ingest.CreateToken(ctx, "acc_levels", "contract", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	xp := entities.XPForLevel
	for i, p := range []struct {
		ago time.Duration
		xp  coreingest.IntMap
	}{
		{30 * time.Hour, coreingest.IntMap{"fishing": int64(xp(75) + 1200), "farming": int64(xp(37) + 300)}},
		{time.Hour, coreingest.IntMap{"fishing": int64(xp(76) + 40), "farming": int64(xp(39) + 10)}},
	} {
		levels := coreingest.IntMap{}
		for skill, v := range p.xp {
			levels[skill] = int64(entities.LevelForXP(int(v)))
		}
		sentAt := now.Add(-p.ago)
		push := coreingest.Push{
			ContractVersion: coreingest.ContractVersion, Username: "Felipe Pesca", Profile: "STANDARD",
			ManifestVersion: manifest.ManifestVersion, ClientSeq: int64(i + 1), Kind: coreingest.PushDelta, SentAt: sentAt,
			Data: coreingest.PushData{Levels: &levels, XP: &p.xp},
		}
		if i == 0 {
			rev, empty := int64(1), coreingest.IntMap{}
			live := func() *coreingest.ItemObservation {
				q := coreingest.QuantityMap{}
				return &coreingest.ItemObservation{State: coreingest.ItemLive, ObservedAt: sentAt, Revision: &rev, LiveEpoch: "login-1", Quantities: &q}
			}
			push.Kind = coreingest.PushFull
			push.Data.Varbits, push.Data.Varps, push.Data.Boosted = &empty, &empty, &empty
			push.Data.Items = &coreingest.ItemContainers{Bank: live(), Inventory: live(), Equipment: live()}
		}
		raw, _ := json.Marshal(push)
		if resp, err := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); err != nil || !resp.OK {
			t.Fatalf("levels push %d: %+v, %v", i, resp, err)
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
	sentAt := time.Now().Add(-15 * time.Minute).UTC().Truncate(time.Second)
	revision := int64(1)
	levels := coreingest.IntMap{"attack": 20, "strength": 20, "defence": 10, "magic": 25, "firemaking": 30, "woodcutting": 30}
	xp := coreingest.IntMap{"attack": 4470, "strength": 4470, "defence": 1154, "magic": 8740, "firemaking": 13363, "woodcutting": 13363}
	empty := coreingest.IntMap{}
	quests := coreingest.QuestMap{"Cook's Assistant": coreingest.QuestFinished, "X Marks the Spot": coreingest.QuestInProgress}
	bank := coreingest.QuantityMap{"952": 1, "590": 1, "1351": 1, "995": 25000}
	inv := coreingest.QuantityMap{"590": 1, "314": 100}
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

// contractSessionPush is a later push of the same session: 5 logs came and
// 40 of the feathers went, for the session's items in the rhythm.
func contractSessionPush(manifestVersion int64) coreingest.Push {
	sentAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	revision := int64(2)
	inv := coreingest.QuantityMap{"314": 60, "1511": 5}
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion, Username: "ironman260", Profile: "STANDARD",
		ManifestVersion: manifestVersion, ClientSeq: 2, Kind: coreingest.PushDelta, SentAt: sentAt,
		Data: coreingest.PushData{Items: &coreingest.ItemContainers{Inventory: &coreingest.ItemObservation{
			State: coreingest.ItemLive, ObservedAt: sentAt, Revision: &revision, LiveEpoch: "login-1", Quantities: &inv,
		}}},
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
	from := c.tsFrom
	if from == "" {
		from = "../../progress/types"
	}
	fmt.Fprintf(&b, "import type { %s } from '%s';\n\n", strings.Join(c.tsImports, ", "), from)
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
