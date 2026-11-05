package progress

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FelipeMalacarne/ironman-planner/internal/core/content"
	"github.com/FelipeMalacarne/ironman-planner/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/ironman-planner/internal/store/ingest"
)

// Item 7 acceptance (docs/04 section 9): a full push with quests and the
// reference varbits, against the real Oziris content and catalog, takes quest,
// diary and quest point objectives out of unknown with snapshot evidence.
func TestQuestAndDiaryFactsPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, subject, username) VALUES ('user-1', 'dev-quest-test', 'dev-quest-test');
		INSERT INTO accounts (id, user_id, rsn, rsn_normalized, mode, guide_id, layer)
		VALUES ('account-1', 'user-1', 'Example', 'example', 'ironman', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	cnt, err := content.Load("../../../content")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := entities.Default()
	if err != nil {
		t.Fatal(err)
	}
	repo := New(pool, cnt, catalog)
	if _, err = repo.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	def := storeingest.BuildDefinition(cnt, catalog)
	ingest := storeingest.New(pool, def).WithReevaluator(repo)
	token, err := ingest.CreateToken(ctx, "account-1", "desktop", nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Quests || !slices.Contains(manifest.Varbits, 4458) || !slices.Contains(manifest.Varps, 101) || manifest.Items.OnlyItemIDs != nil {
		t.Fatalf("manifest = %+v", manifest)
	}

	const (
		xMarks      = "quest:x_marks_the_spot@finished"
		runeStarted = "quest:rune_mysteries@started"
		ardyEasy    = "diary:ardougne:easy@complete"
		ardyMedium  = "diary:ardougne:medium@complete"
		varrockEasy = "diary:varrock:easy@complete"
	)

	// A plugin 1.0.0 full: no data.quest and no reference varbits. Nothing
	// about quests or diaries is known.
	push(ctx, t, ingest, token.Token, questPush(1, manifest.ManifestVersion, coreingest.PushFull, nil, map[string]int64{}, map[string]int64{}))
	for _, key := range []string{xMarks, runeStarted, ardyEasy, varrockEasy} {
		if v := verdictOf(ctx, t, pool, key); v != "" {
			t.Fatalf("%s = %q before any quest or diary fact, want no row (unknown)", key, v)
		}
	}

	// Plugin 1.1.0 full: quests by RuneLite name, diary varbits, quest points.
	quests := coreingest.QuestMap{
		"X Marks the Spot":                     coreingest.QuestFinished,
		"Rune Mysteries":                       coreingest.QuestNotStarted,
		"Recipe for Disaster - Mountain Dwarf": coreingest.QuestInProgress,
		"Not A Real Quest":                     coreingest.QuestFinished,
	}
	push(ctx, t, ingest, token.Token, questPush(2, manifest.ManifestVersion, coreingest.PushFull, &quests,
		map[string]int64{"4458": 1, "4459": 0}, map[string]int64{"101": 3}))
	snapshot := latestSnapshot(ctx, t, pool)

	met := objectiveRow(ctx, t, pool, xMarks)
	if met.verdict != "met" || met.snapshot != snapshot || !strings.Contains(met.evidence, `"facet": "quest"`) || !strings.Contains(met.evidence, `"key": "X Marks the Spot"`) {
		t.Fatalf("%s = %+v", xMarks, met)
	}
	if v := verdictOf(ctx, t, pool, runeStarted); v != "unmet" {
		t.Fatalf("%s = %q, want unmet (NOT_STARTED)", runeStarted, v)
	}
	diary := objectiveRow(ctx, t, pool, ardyEasy)
	if diary.verdict != "met" || diary.snapshot != snapshot || !strings.Contains(diary.evidence, `"facet": "diary"`) {
		t.Fatalf("%s = %+v", ardyEasy, diary)
	}
	if v := verdictOf(ctx, t, pool, ardyMedium); v != "unmet" {
		t.Fatalf("%s = %q, want unmet", ardyMedium, v)
	}
	if v := verdictOf(ctx, t, pool, varrockEasy); v != "" {
		t.Fatalf("%s = %q, want no row: its varbit was not pushed", varrockEasy, v)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM objective_progress WHERE objective_key LIKE 'qp>=%' AND verdict = 'unmet'`); n == 0 {
		t.Fatal("no quest point objective evaluated from varp 101")
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM player_state WHERE facet = 'quest'`); n != 3 {
		t.Fatalf("quest facts = %d, want 3 (the unknown name is dropped)", n)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM player_state WHERE facet = 'quest' AND key = 'Recipe for Disaster/Freeing the Mountain Dwarf' AND value->>'state' = 'IN_PROGRESS'`); n != 1 {
		t.Fatal("a RuneLite alias was not stored under the canonical name")
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM snapshot_facts WHERE snapshot_id = '`+snapshot+`' AND facet IN ('quest', 'diary', 'varbit', 'varp')`); n != 3+2+2+1 {
		t.Fatalf("snapshot facts = %d, want 8", n)
	}

	// The account screen reads the same facts, with the catalog id of each quest.
	st, err := repo.AccountState(ctx, "account-1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Account.RSN != "Example" || len(st.Quests) != 3 || st.QuestPoints == nil || *st.QuestPoints != 3 || len(st.Diaries) != 2 {
		t.Fatalf("account state = %+v", st)
	}
	for _, q := range st.Quests {
		if q.ID == "" || q.Source != "plugin" || q.ObservedAt.IsZero() {
			t.Fatalf("quest view = %+v", q)
		}
	}
	if d := st.Diaries[0]; d.Area != "ardougne" || d.Tier != "easy" || !d.Complete {
		t.Fatalf("diaries = %+v", st.Diaries)
	}
	if _, err = repo.AccountState(ctx, "nobody"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("unknown account = %v", err)
	}

	// A delta with one changed quest.
	changed := coreingest.QuestMap{"Rune Mysteries": coreingest.QuestInProgress}
	push(ctx, t, ingest, token.Token, questPush(3, manifest.ManifestVersion, coreingest.PushDelta, &changed, nil, nil))
	if v := verdictOf(ctx, t, pool, runeStarted); v != "met" {
		t.Fatalf("%s after the delta = %q", runeStarted, v)
	}

	// A plugin 1.0.0 full again keeps the quests it does not report, and the
	// met diary stays latched with its evidence.
	push(ctx, t, ingest, token.Token, questPush(4, manifest.ManifestVersion, coreingest.PushFull, nil, map[string]int64{}, map[string]int64{}))
	if n := count(ctx, t, pool, `SELECT count(*) FROM player_state WHERE facet = 'quest'`); n != 3 {
		t.Fatalf("quest facts after a full without data.quest = %d", n)
	}
	if n := count(ctx, t, pool, `SELECT count(*) FROM player_state WHERE facet = 'diary'`); n != 0 {
		t.Fatalf("diary facts after a full without varbits = %d", n)
	}
	if again := objectiveRow(ctx, t, pool, ardyEasy); again.verdict != "met" || again.snapshot != snapshot {
		t.Fatalf("latched diary = %+v", again)
	}
}

type objectiveRowView struct{ verdict, snapshot, evidence string }

func objectiveRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string) objectiveRowView {
	t.Helper()
	var row objectiveRowView
	var snapshot *string
	var evidence []byte
	if err := pool.QueryRow(ctx, `SELECT verdict, evidence_snapshot_id, evidence FROM objective_progress WHERE account_id = 'account-1' AND objective_key = $1`, key).
		Scan(&row.verdict, &snapshot, &evidence); err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	if snapshot != nil {
		row.snapshot = *snapshot
	}
	var pretty any
	if json.Unmarshal(evidence, &pretty) == nil {
		indented, _ := json.MarshalIndent(pretty, "", " ")
		row.evidence = strings.ReplaceAll(string(indented), "\n", "")
	}
	return row
}

func verdictOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, key string) string {
	t.Helper()
	var v string
	_ = pool.QueryRow(ctx, `SELECT verdict FROM objective_progress WHERE account_id = 'account-1' AND objective_key = $1`, key).Scan(&v)
	return v
}

func questPush(seq, manifestVersion int64, kind coreingest.PushKind, quests *coreingest.QuestMap, varbits, varps map[string]int64) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	p := coreingest.Push{
		ContractVersion: coreingest.ContractVersion, Username: "Example", Profile: "STANDARD",
		ManifestVersion: manifestVersion, ClientSeq: seq, Kind: kind, SentAt: sentAt,
		Data: coreingest.PushData{Quests: quests},
	}
	if kind == coreingest.PushFull {
		empty := func() *coreingest.IntMap { m := coreingest.IntMap{}; return &m }
		vb, vp := coreingest.IntMap(varbits), coreingest.IntMap(varps)
		unavailable := func() *coreingest.ItemObservation {
			return &coreingest.ItemObservation{State: coreingest.ItemUnavailable, ObservedAt: sentAt.Add(-time.Second)}
		}
		p.Data.Varbits, p.Data.Varps = &vb, &vp
		p.Data.Levels, p.Data.XP, p.Data.Boosted = empty(), empty(), empty()
		p.Data.Items = &coreingest.ItemContainers{Bank: unavailable(), Inventory: unavailable(), Equipment: unavailable()}
	}
	return p
}
