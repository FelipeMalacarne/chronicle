package workers

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
)

func TestClientProfilesPostgres(t *testing.T) {
	ctx := context.Background()
	_, repo := setup(ctx, t)

	model := agent.ClientProfile{
		Plugins: []agent.ProfilePlugin{
			{Source: agent.PluginCore, Name: "net.runelite.client.plugins.entityhider.EntityHiderPlugin", Start: true},
			{Source: agent.PluginHub, Name: "GotrPlugin", Version: "1.0.0"},
		},
		Settings: []agent.ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "true"},
			{Group: "gotr", Key: "mode", Value: "fast"},
		},
	}
	gotr := agent.ClientProfile{
		Plugins: []agent.ProfilePlugin{
			{Source: agent.PluginHub, Name: "GotrPlugin", ClassName: "a.GotrPlugin", Start: true},
			{Source: agent.PluginOwn, Name: "a.CatSwapPlugin"},
		},
		Settings: []agent.ProfileSetting{{Group: "gotr", Key: "mode", Value: "safe"}},
	}

	// Refusals before anything is stored.
	for name, err := range map[string]error{
		"bad name":  func() error { _, e := repo.PutProfile(ctx, "u1", "Gotr Base", "", model); return e }(),
		"own model": func() error { _, e := repo.PutProfile(ctx, "u1", "gotr", "gotr", gotr); return e }(),
		"bad layer": func() error {
			_, e := repo.PutProfile(ctx, "u1", "x", "", agent.ClientProfile{Settings: []agent.ProfileSetting{{Group: "runelite", Key: "k", Value: "v"}}})
			return e
		}(),
		"unknown user": func() error { _, e := repo.PutProfile(ctx, "u9", "x", "", model); return e }(),
	} {
		want := ErrInvalid
		if name == "unknown user" {
			want = ErrNotFound
		}
		if !errors.Is(err, want) {
			t.Errorf("%s = %v, want %v", name, err, want)
		}
	}
	if _, err := repo.PutProfile(ctx, "u1", "gotr", "missing", gotr); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("unknown model = %v", err)
	}

	if _, err := repo.PutProfile(ctx, "u1", "gotr-base", "", model); err != nil {
		t.Fatal(err)
	}
	p, err := repo.PutProfile(ctx, "u1", "gotr", "gotr-base", gotr)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "gotr" || p.Parent != "gotr-base" || !reflect.DeepEqual(p.Plugins, gotr.Plugins) || !reflect.DeepEqual(p.Settings, gotr.Settings) || len(p.Accounts) != 0 {
		t.Fatalf("put = %+v", p)
	}
	// One level: a profile with a model is not a model, and a model gets no model.
	if _, err = repo.PutProfile(ctx, "u1", "gotr-alt", "gotr", gotr); !errors.Is(err, ErrInvalid) {
		t.Fatalf("model with a model = %v", err)
	}
	if _, err = repo.PutProfile(ctx, "u1", "other", "", model); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.PutProfile(ctx, "u1", "gotr-base", "other", model); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a model given a model = %v", err)
	}
	// Profiles are per user.
	if list, lerr := repo.ListProfiles(ctx, "u2"); lerr != nil || len(list) != 0 {
		t.Fatalf("u2 profiles = %+v, %v", list, lerr)
	}
	if _, err = repo.GetProfile(ctx, "u2", "gotr"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("another user's profile = %v", err)
	}
	if _, err = repo.SetAccountClient(ctx, "acc_c", "gotr", nil); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("u2's account with u1's profile = %v", err)
	}

	// The account: profile, its own settings, the effective merge.
	if _, err = repo.SetAccountClient(ctx, "acc_a", "gotr", []agent.ProfileSetting{{Group: "bank", Key: "pin", Value: "1"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("secret-looking override = %v", err)
	}
	if _, err = repo.SetAccountClient(ctx, "acc_missing", "gotr", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account = %v", err)
	}
	ac, err := repo.SetAccountClient(ctx, "acc_a", "gotr", []agent.ProfileSetting{{Group: "entityhider", Key: "hidePlayers", Value: "false"}})
	if err != nil {
		t.Fatal(err)
	}
	want := agent.ClientProfile{
		Plugins: []agent.ProfilePlugin{
			{Source: agent.PluginCore, Name: "net.runelite.client.plugins.entityhider.EntityHiderPlugin", Start: true},
			{Source: agent.PluginHub, Name: "GotrPlugin", ClassName: "a.GotrPlugin", Start: true},
			{Source: agent.PluginOwn, Name: "a.CatSwapPlugin"},
		},
		Settings: []agent.ProfileSetting{
			{Group: "entityhider", Key: "hidePlayers", Value: "false"},
			{Group: "gotr", Key: "mode", Value: "safe"},
		},
	}
	if ac.Profile != "gotr" || !reflect.DeepEqual(ac.Effective, want) {
		t.Fatalf("account client = %+v", ac)
	}
	if eff, eerr := repo.EffectiveProfile(ctx, "acc_a"); eerr != nil || !reflect.DeepEqual(eff, want) {
		t.Fatalf("effective = %+v, %v", eff, eerr)
	}
	if got, gerr := repo.AccountClient(ctx, "acc_a"); gerr != nil || !reflect.DeepEqual(got, ac) {
		t.Fatalf("account client read = %+v, %v", got, gerr)
	}
	if _, err = repo.AccountClient(ctx, "acc_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account read = %v", err)
	}
	// An account without a row has an empty profile.
	if got, gerr := repo.AccountClient(ctx, "acc_b"); gerr != nil || got.Profile != "" || len(got.Effective.Plugins)+len(got.Effective.Settings) != 0 {
		t.Fatalf("account without profile = %+v, %v", got, gerr)
	}
	list, err := repo.ListProfiles(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range list {
		names = append(names, p.Name)
		if p.Name == "gotr" && (p.Parent != "gotr-base" || !reflect.DeepEqual(p.Accounts, []string{"acc_a"})) {
			t.Fatalf("listed gotr = %+v", p)
		}
	}
	if !reflect.DeepEqual(names, []string{"gotr", "gotr-base", "other"}) {
		t.Fatalf("listed = %v", names)
	}

	// Replacing a profile replaces its plugins and settings.
	if _, err = repo.PutProfile(ctx, "u1", "gotr", "gotr-base", agent.ClientProfile{}); err != nil {
		t.Fatal(err)
	}
	if eff, _ := repo.EffectiveProfile(ctx, "acc_a"); len(eff.Plugins) != 2 || eff.Settings[1].Value != "fast" {
		t.Fatalf("effective after replace = %+v", eff)
	}

	// Deleting: never a model in use nor an account's profile.
	if err = repo.DeleteProfile(ctx, "u1", "gotr-base"); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("delete a model = %v", err)
	}
	if err = repo.DeleteProfile(ctx, "u1", "gotr"); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("delete an account's profile = %v", err)
	}
	if err = repo.DeleteProfile(ctx, "u2", "other"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("delete another user's profile = %v", err)
	}
	if ac, err = repo.SetAccountClient(ctx, "acc_a", "", ac.Settings); err != nil || ac.Profile != "" ||
		!reflect.DeepEqual(ac.Effective, agent.ClientProfile{Plugins: []agent.ProfilePlugin{}, Settings: ac.Settings}) {
		t.Fatalf("account without profile = %+v, %v", ac, err)
	}
	for _, name := range []string{"gotr", "gotr-base", "other"} {
		if err = repo.DeleteProfile(ctx, "u1", name); err != nil {
			t.Fatalf("delete %s: %v", name, err)
		}
	}
	if list, _ = repo.ListProfiles(ctx, "u1"); len(list) != 0 {
		t.Fatalf("after deletes = %+v", list)
	}
}
