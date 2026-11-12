package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	"github.com/FelipeMalacarne/chronicle/internal/sources/hiscores"
	storeprogress "github.com/FelipeMalacarne/chronicle/internal/store/progress"
)

type fakeAccounts struct{}

func (fakeAccounts) FindAccount(_ context.Context, _, rsn string) (storeprogress.AccountView, bool, error) {
	if storeprogress.NormalizeRSN(rsn) == "iron_man" {
		return storeprogress.AccountView{ID: "acc_iron_man", RSN: "Iron Man", Mode: "ironman"}, true, nil
	}
	return storeprogress.AccountView{}, false, nil
}

func (fakeAccounts) CreateAccount(_ context.Context, _ string, in storeprogress.AccountInput) (storeprogress.AccountView, bool, error) {
	if in.Mode == "main" {
		return storeprogress.AccountView{}, false, storeprogress.ErrInvalidAccount
	}
	if storeprogress.NormalizeRSN(in.RSN) == "iron_man" {
		return storeprogress.AccountView{ID: "acc_iron_man"}, false, nil
	}
	return storeprogress.AccountView{ID: "acc_" + storeprogress.NormalizeRSN(in.RSN), RSN: in.RSN, Mode: in.Mode}, true, nil
}

type fakeDetector struct{ err error }

func (f fakeDetector) Detect(_ context.Context, rsn string) (hiscores.Detection, error) {
	if f.err != nil {
		return hiscores.Detection{}, f.err
	}
	if rsn == "Nobody" {
		return hiscores.Detection{}, nil
	}
	return hiscores.Detection{Mode: "hcim", Found: true, Snapshot: hiscores.Snapshot{Skills: []hiscores.Skill{
		{Name: "Overall", Rank: 1, Level: 701}, {Name: "Firemaking", Rank: 9, Level: 50}, {Name: "Fletching", Rank: -1, Level: 1},
	}}}, nil
}

func TestSetupRoutes(t *testing.T) {
	newServer := func(dev bool, det ModeDetector) string {
		srv := httptest.NewServer(New(Options{
			Version: "test", Content: &content.Content{}, Accounts: fakeAccounts{}, Hiscores: det, DevMode: dev,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}))
		t.Cleanup(srv.Close)
		return srv.URL + "/api"
	}
	if status, _ := do(t, "GET", newServer(false, fakeDetector{})+"/setup/lookup?rsn=x", ""); status != http.StatusNotFound {
		t.Fatalf("setup outside dev mode = %d", status)
	}
	base := newServer(true, fakeDetector{})
	status, body := do(t, "GET", base+"/setup/lookup?rsn=Iron%20Man", "")
	if status != http.StatusOK || body["account"].(map[string]any)["id"] != "acc_iron_man" {
		t.Fatalf("existing = %d %v", status, body)
	}
	status, body = do(t, "GET", base+"/setup/lookup?rsn=New%20Guy", "")
	if status != http.StatusOK || body["account"] != nil || body["mode"] != "hcim" || body["total_level"] != float64(701) || len(body["skills"].([]any)) != 1 {
		t.Fatalf("new = %d %v", status, body)
	}
	if status, body = do(t, "GET", base+"/setup/lookup?rsn=Nobody", ""); status != http.StatusOK || body["found"] != false {
		t.Fatalf("unknown name = %d %v", status, body)
	}
	if status, _ = do(t, "GET", base+"/setup/lookup?rsn=", ""); status != http.StatusBadRequest {
		t.Fatalf("empty name = %d", status)
	}
	down := newServer(true, fakeDetector{err: errors.New("503")})
	if status, body = do(t, "GET", down+"/setup/lookup?rsn=New", ""); status != http.StatusOK || body["hiscores_error"] != "503" {
		t.Fatalf("hiscores down = %d %v", status, body)
	}
	if status, body = do(t, "POST", base+"/accounts", `{"rsn":"New Guy","mode":"hcim","guide_id":"oziris-main","layer":"enhanced"}`); status != http.StatusCreated || body["id"] != "acc_new_guy" {
		t.Fatalf("create = %d %v", status, body)
	}
	if status, _ = do(t, "POST", base+"/accounts", `{"rsn":"Iron Man","mode":"ironman","guide_id":"oziris-main","layer":"enhanced"}`); status != http.StatusOK {
		t.Fatalf("existing create = %d", status)
	}
	if status, _ = do(t, "POST", base+"/accounts", `{"rsn":"X","mode":"main","guide_id":"oziris-main","layer":"enhanced"}`); status != http.StatusBadRequest {
		t.Fatalf("invalid create = %d", status)
	}
}
