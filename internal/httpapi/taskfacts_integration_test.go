package httpapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

// TestTaskFactsPostgres: what the panel's assisted mode evaluates tasks on
// comes from the account (its mode), the merged facts and the last position
// the plugin sent (docs/10 phase 2).
func TestTaskFactsPostgres(t *testing.T) {
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
		VALUES ('acc_facts', 'dev', 'ironman260', 'ironman260', 'hcim', 'oziris-main', 'enhanced');
	`); err != nil {
		t.Fatal(err)
	}
	progress := storeprogress.New(pool, cnt, catalog)
	if _, err = progress.IndexContent(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	ingest := storeingest.New(pool, storeingest.BuildDefinition(cnt, catalog)).WithReevaluator(progress)
	token, err := ingest.CreateToken(ctx, "acc_facts", "facts", []coreingest.TokenScope{coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ingest.Manifest(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}

	before, err := progress.TaskFacts(ctx, "acc_facts")
	if err != nil || before.Mode != "hcim" || before.Position != nil || before.State == nil {
		t.Fatalf("before any push = %+v, %v", before, err)
	}
	push := contractPush(manifest.ManifestVersion)
	push.Data.Position = &coreingest.Position{World: 330, X: 3222, Y: 3218}
	raw, _ := json.Marshal(push)
	if resp, aerr := ingest.Accept(ctx, token.Token, "127.0.0.1", raw, push); aerr != nil || !resp.OK {
		t.Fatalf("push = %+v, %v", resp, aerr)
	}
	f, err := progress.TaskFacts(ctx, "acc_facts")
	if err != nil {
		t.Fatal(err)
	}
	if f.Position == nil || f.Position.X != 3222 || f.Position.Y != 3218 || f.Position.ObservedAt.IsZero() {
		t.Fatalf("position = %+v", f.Position)
	}
	if f.State.Skills["woodcutting"].Level != 30 {
		t.Fatalf("facts = %+v", f.State.Skills)
	}
	if _, err = progress.TaskFacts(ctx, "acc_nobody"); err != storeprogress.ErrAccountNotFound {
		t.Fatalf("unknown account = %v", err)
	}
}
