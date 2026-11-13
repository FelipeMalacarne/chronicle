package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/client"
)

// fakeProfiles is the profile routes of the API, in memory, for one account.
type fakeProfiles struct {
	mu       sync.Mutex
	profiles map[string]client.ProfileDef
	account  client.AccountClient
}

func (f *fakeProfiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimPrefix(r.URL.Path, "/api/client-profiles/")
	switch {
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/client-profiles/"):
		var def client.ProfileDef
		_ = json.NewDecoder(r.Body).Decode(&def)
		f.profiles[name] = def
		_ = json.NewEncoder(w).Encode(client.Profile{Name: name, ProfileDef: def})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/client-profiles/"):
		def, ok := f.profiles[name]
		if !ok {
			http.Error(w, `{"error":"workers: client profile not found"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(client.Profile{Name: name, ProfileDef: def})
	case r.URL.Path == "/api/accounts/client" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(f.account)
	case r.URL.Path == "/api/accounts/client" && r.Method == http.MethodPut:
		var body struct {
			Profile  string                 `json:"profile"`
			Settings []agent.ProfileSetting `json:"settings"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.account = client.AccountClient{AccountID: r.URL.Query().Get("account"), Profile: body.Profile, Settings: body.Settings}
		_ = json.NewEncoder(w).Encode(f.account)
	default:
		http.NotFound(w, r)
	}
}

func TestProfilesCLI(t *testing.T) {
	f := &fakeProfiles{profiles: map[string]client.ProfileDef{}, account: client.AccountClient{
		AccountID: "acc_farm", Settings: []agent.ProfileSetting{{Group: "blackjacker", Key: "room", Value: "THUG_TENT"}},
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "farm.json")
	def := `{"parent":"farm-base","plugins":[{"source":"core","name":"a.EntityHiderPlugin","start":true}],
		"settings":[{"group":"entityhider","key":"hidePlayers","value":"true"}]}`
	if werr := os.WriteFile(file, []byte(def), 0o600); werr != nil {
		t.Fatal(werr)
	}

	out, err := runCLI(t, "profiles", "put", "farm", "-f", file, "--url", srv.URL)
	if err != nil || !strings.Contains(out, "Perfil farm gravado: 1 plugins, 1 configurações") {
		t.Fatalf("put = %q, %v", out, err)
	}
	// show prints what put reads.
	out, err = runCLI(t, "profiles", "show", "farm", "--url", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var shown client.ProfileDef
	if jerr := json.Unmarshal([]byte(out), &shown); jerr != nil || !reflect.DeepEqual(shown, f.profiles["farm"]) {
		t.Fatalf("show = %q (%v)", out, jerr)
	}
	if werr := os.WriteFile(file, []byte(out), 0o600); werr != nil {
		t.Fatal(werr)
	}
	if _, err = runCLI(t, "profiles", "put", "farm-copy", "-f", file, "--url", srv.URL); err != nil || !reflect.DeepEqual(f.profiles["farm-copy"], shown) {
		t.Fatalf("round trip = %+v, %v", f.profiles["farm-copy"], err)
	}
	if werr := os.WriteFile(file, []byte(`{"plugin":[]}`), 0o600); werr != nil {
		t.Fatal(werr)
	}
	if _, err = runCLI(t, "profiles", "put", "x", "-f", file, "--url", srv.URL); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field = %v", err)
	}

	// use keeps the account's own settings; account -f replaces them and keeps the profile.
	if _, err = runCLI(t, "profiles", "use", "--url", srv.URL, "--account", "acc_farm"); err == nil {
		t.Fatal("use without a profile or --none")
	}
	out, err = runCLI(t, "profiles", "use", "farm", "--url", srv.URL, "--account", "acc_farm")
	if err != nil || !strings.Contains(out, "acc_farm: perfil farm") || f.account.Profile != "farm" || len(f.account.Settings) != 1 {
		t.Fatalf("use = %q, %v, %+v", out, err, f.account)
	}
	settings := filepath.Join(dir, "settings.json")
	if werr := os.WriteFile(settings, []byte(`[{"group":"entityhider","key":"hidePlayers","value":"false"}]`), 0o600); werr != nil {
		t.Fatal(werr)
	}
	out, err = runCLI(t, "profiles", "account", "-f", settings, "--url", srv.URL, "--account", "acc_farm")
	want := []agent.ProfileSetting{{Group: "entityhider", Key: "hidePlayers", Value: "false"}}
	if err != nil || f.account.Profile != "farm" || !reflect.DeepEqual(f.account.Settings, want) || !strings.Contains(out, "perfil farm · 1 configurações próprias") {
		t.Fatalf("account -f = %q, %v, %+v", out, err, f.account)
	}
	if _, err = runCLI(t, "profiles", "use", "--none", "--url", srv.URL, "--account", "acc_farm"); err != nil || f.account.Profile != "" || len(f.account.Settings) != 1 {
		t.Fatalf("use --none = %v, %+v", err, f.account)
	}
}
