package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FelipeMalacarne/chronicle/internal/client"
	"github.com/FelipeMalacarne/chronicle/internal/core/mta"
	"github.com/FelipeMalacarne/chronicle/internal/httpapi"
	"github.com/FelipeMalacarne/chronicle/internal/mcp"
)

func TestMTACLI(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()

	out, err := runCLI(t, "mta", "--url", srv.URL, "--want", "master_wand", "--own", "beginner_wand",
		"--points", "100,0,500,0", "--magic", "70")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"apprentice_wand, teacher_wand, master_wand", "law 613, nature 1860, cosmic 728", "Lvl-5 Enchant"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}

	out, err = runCLI(t, "mta", "rewards", "--url", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bones_to_peaches") || !strings.Contains(out, "upgrade de teacher_wand") {
		t.Errorf("rewards:\n%s", out)
	}

	if _, err := runCLI(t, "mta", "--url", srv.URL); err == nil {
		t.Error("no --want should be refused")
	}
	if _, err := runCLI(t, "mta", "--url", srv.URL, "--want", "master_wand", "--points", "1,2"); err == nil {
		t.Error("--points with two numbers should be refused")
	}
	if _, err := runCLI(t, "mta", "--url", srv.URL, "--want", "nope", "--magic", "99"); err == nil {
		t.Error("an unknown reward should be refused")
	}
}

func TestMTAMCPTools(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(httpapi.Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	defer srv.Close()
	c, err := client.New(context.Background(), client.Options{URL: srv.URL + "/", Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]mcp.Tool{}
	for _, tool := range newAccountMCPServer(c).Tools {
		tools[tool.Name] = tool
	}
	calc, ok := tools["mta_calculate"]
	if !ok || tools["mta_rewards"].Handler == nil {
		t.Fatal("mta_calculate and mta_rewards must be registered")
	}
	got, err := calc.Handler(context.Background(), json.RawMessage(`{"rewards":["bones_to_peaches"],"magic":99}`))
	if err != nil {
		t.Fatal(err)
	}
	res, ok := got.(mta.Result)
	if !ok || res.Runes != (mta.Runes{Law: 350, Nature: 1060, Cosmic: 304}) {
		t.Errorf("got %#v", got)
	}
	if _, err := calc.Handler(context.Background(), json.RawMessage(`{"rewards":["nope"]}`)); err == nil {
		t.Error("an unknown reward should be an error")
	}
}
