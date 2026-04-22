package config

import (
	"reflect"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := &AppState{
		Name:         "app1",
		Repo:         "iluxa/app1",
		RepoURL:      "git@github.com:iluxa/app1.git",
		Ref:          "main",
		Commit:       "abc1234",
		Service:      "web",
		Port:         3000,
		Scale:        2,
		URL:          "https://app1.example.com",
		EnvFile:      "/var/lib/tcd/apps/app1/.env",
		ComposeFile:  "/var/lib/tcd/apps/app1/repo/compose.yml",
		OverrideFile: "/var/lib/tcd/apps/app1/override.yml",
	}
	if err := SaveState(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}
