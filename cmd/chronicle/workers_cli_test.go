package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLauncherAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	// The launcher keeps a character without a display name as null (shown "Not set").
	if err := os.WriteFile(path, []byte(`[
		{"displayName":"xCloudd","sessionId":"s1","accountId":"1"},
		{"displayName":"Sr cobalt","sessionId":"s2","accountId":"2"},
		{"displayName":null,"sessionId":"s3","accountId":"3"},
		{"displayName":"NoSession","sessionId":"","accountId":"4"}
	]`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		pick    launcherPick
		want    string // character; "" expects an error containing wantErr
		wantID  string
		wantErr string
	}{
		{name: "by name, any case", pick: launcherPick{Character: "sr COBALT"}, want: "Sr cobalt", wantID: "2"},
		{name: "by index", pick: launcherPick{Index: 1}, want: "xCloudd", wantID: "1"},
		{name: "not set by index with as", pick: launcherPick{Index: 3, As: "EventSourced"}, want: "EventSourced", wantID: "3"},
		{name: "not set by name with as", pick: launcherPick{Character: "Not set", As: "EventSourced"}, want: "EventSourced", wantID: "3"},
		{name: "not set without as", pick: launcherPick{Index: 3}, wantErr: "--as"},
		{name: "unknown name hints at --as", pick: launcherPick{Character: "eventsourced"}, wantErr: "--index <número> --as"},
		{name: "index out of range", pick: launcherPick{Index: 9}, wantErr: "4 personagens"},
		{name: "no session", pick: launcherPick{Character: "NoSession"}, wantErr: "não tem sessão"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readLauncherAccount(path, tt.pick)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Character != tt.want || got.CharacterID != tt.wantID || got.SessionID == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestReadLauncherAccountAmbiguousNotSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := os.WriteFile(path, []byte(`[{"displayName":null,"sessionId":"a","accountId":"1"},{"sessionId":"b","accountId":"2"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLauncherAccount(path, launcherPick{Character: "Not set", As: "X"}); err == nil || !strings.Contains(err.Error(), "--index") {
		t.Fatalf("two Not set characters by name: err = %v", err)
	}
	got, err := readLauncherAccount(path, launcherPick{Index: 2, As: "X"})
	if err != nil || got.CharacterID != "2" {
		t.Fatalf("by index: %+v %v", got, err)
	}
}
