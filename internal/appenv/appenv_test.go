package appenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetFallsBackToTheOldName(t *testing.T) {
	t.Setenv("CHRONICLE_URL", "")
	os.Unsetenv("CHRONICLE_URL")
	t.Setenv("IRONMAN_URL", "http://old")
	if got := Get("URL"); got != "http://old" {
		t.Fatalf("legacy only = %q", got)
	}
	t.Setenv("CHRONICLE_URL", "http://new")
	if got := Get("URL"); got != "http://new" {
		t.Fatalf("both = %q", got)
	}
	// set but empty still wins: it is how a user clears the old one
	t.Setenv("CHRONICLE_URL", "")
	if got := Get("URL"); got != "" {
		t.Fatalf("empty new = %q", got)
	}
}

func TestConfigDirMovesTheOldOne(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	for _, k := range []string{"CHRONICLE_CONFIG_DIR", "IRONMAN_CONFIG_DIR"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	old := filepath.Join(base, "ironman")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "credentials.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(base, "chronicle") {
		t.Fatalf("dir = %s", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); err != nil {
		t.Fatalf("login not moved: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old dir still there: %v", err)
	}
	t.Setenv("IRONMAN_CONFIG_DIR", "/elsewhere")
	if dir, _ = ConfigDir(); dir != "/elsewhere" {
		t.Fatalf("legacy override = %s", dir)
	}
}
