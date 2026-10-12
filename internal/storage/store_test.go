package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadAtomicConfig(t *testing.T) {
	d := t.TempDir()
	c := Config{Version: 1, Profiles: []Profile{{ID: "x", Name: "Agent"}}}
	if e := Save(d, c); e != nil {
		t.Fatal(e)
	}
	got, e := Load(d)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].Name != "Agent" {
		t.Fatalf("got %+v", got)
	}
	fi, e := os.Stat(filepath.Join(d, "agents.json"))
	if e != nil {
		t.Fatal(e)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}
func TestCorruptConfigErrors(t *testing.T) {
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, "agents.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(d); e == nil {
		t.Fatal("expected decode error")
	}
}

func TestLoadMigratesLegacyFavorites(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":1,"profiles":[{"id":"sales","name":"Sales","pane_id":"w1:p2","favorite":true}]}`
	if err := os.WriteFile(filepath.Join(dir, "agents.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != SchemaVersion || len(got.Favorites) != 1 || got.Favorites[0] != "w1:p2" {
		t.Fatalf("migration result=%+v", got)
	}
}

func TestLoadMigratesSchemaTwoProfilesWithIdentityInstructionsField(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"version":2,"profiles":[{"id":"writer","name":"Writer","description":"Short summary"}]}`
	if err := os.WriteFile(filepath.Join(dir, "agents.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != SchemaVersion || len(got.Profiles) != 1 || got.Profiles[0].Instructions != "" {
		t.Fatalf("schema migration result=%+v", got)
	}
}

func TestValidateRejectsUnsafeAndDuplicateAssociations(t *testing.T) {
	c := Config{Version: 1, Profiles: []Profile{{ID: "a", Name: "A", PaneID: "w1:p1", Icon: "\x1b"}, {ID: "b", Name: "B", PaneID: "w1:p1"}}}
	if err := Validate(c); err == nil {
		t.Fatal("expected invalid profile config to be rejected")
	}
}
