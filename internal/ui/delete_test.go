package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
)

func deleteKey(m Model, text string) Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: rune(text[0]), Text: text})
	return updated.(Model)
}

func TestDeleteLocalProfileConfirmsAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "local-1", Name: "Old agent"}}, Favorites: []string{"local|local-1"}}
	if err := storage.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	m := New(&herdr.Fake{}, cfg, dir)
	m.width, m.height = 80, 25
	m.setLiveAgents(nil)
	m = deleteKey(m, "d")
	if !m.confirm || !strings.Contains(m.render(), "Y confirm") {
		t.Fatal("D did not show confirmation")
	}
	m = deleteKey(m, "y")
	if m.confirm || len(m.agents) != 0 || len(m.config.Profiles) != 0 {
		t.Fatalf("profile remained: %+v", m)
	}
	got, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Profiles) != 0 || len(got.Favorites) != 0 {
		t.Fatalf("saved config still contains deleted profile: %+v", got)
	}
	m.setLiveAgents(nil)
	if len(m.agents) != 0 {
		t.Fatal("deleted profile returned after refresh")
	}
}

func TestDeleteConfirmationCanCancel(t *testing.T) {
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "local-1", Name: "Keep me"}}}
	m := New(&herdr.Fake{}, cfg, t.TempDir())
	m.setLiveAgents(nil)
	m = deleteKey(m, "d")
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.confirm || len(m.config.Profiles) != 1 || len(m.agents) != 1 {
		t.Fatal("Esc did not cancel deletion")
	}
}

func TestDeleteProfileAttachedToLiveAgentKeepsSession(t *testing.T) {
	dir := t.TempDir()
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "local-1", Name: "Custom name", PaneID: "w1:p1"}}}
	if err := storage.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	m := New(&herdr.Fake{}, cfg, dir)
	m.setLiveAgents([]agents.Agent{{ID: "w1:p1", PaneID: "w1:p1", Name: "Native name", Provider: "claude", Status: "IDLE"}})
	m = deleteKey(deleteKey(m, "d"), "y")
	if len(m.agents) != 1 || m.agents[0].Name != "Native name" || m.agents[0].PaneID != "w1:p1" {
		t.Fatalf("live Herdr agent was lost or profile persisted: %+v", m.agents)
	}
	if len(m.config.Profiles) != 0 {
		t.Fatal("profile still configured")
	}
}

func TestDeleteCannotRemoveUnconfiguredLiveAgent(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.setLiveAgents([]agents.Agent{{ID: "w1:p1", PaneID: "w1:p1", Name: "Claude", Status: "IDLE"}})
	m = deleteKey(m, "d")
	if m.confirm || !strings.Contains(m.err, "no local profile") {
		t.Fatalf("live agent deletion was not explained: %q", m.err)
	}
}

func TestDeleteFailureKeepsConfigAndProfile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file-instead-of-directory")
	if err := os.WriteFile(dir, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "local-1", Name: "Keep me"}}}
	m := New(&herdr.Fake{}, cfg, dir)
	m.setLiveAgents(nil)
	m = deleteKey(deleteKey(m, "d"), "y")
	if len(m.config.Profiles) != 1 || len(m.agents) != 1 || !strings.Contains(m.err, "Could not delete") {
		t.Fatalf("failed save lost local data: %+v", m)
	}
}

func TestDeleteFromChatBoard(t *testing.T) {
	dir := t.TempDir()
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "local-1", Name: "Custom", PaneID: "w1:p1"}}}
	if err := storage.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	m := New(&herdr.Fake{}, cfg, dir)
	m.width, m.height = 100, 30
	m.view = "chats"
	m.setLiveAgents([]agents.Agent{{ID: "w1:p1", PaneID: "w1:p1", Name: "Original", Status: "IDLE"}})
	m = deleteKey(m, "d")
	if !m.confirm || !strings.Contains(m.render(), "Y confirm") {
		t.Fatal("chat board D did not ask for confirmation")
	}
	m = deleteKey(m, "y")
	if m.view != "chats" || len(m.config.Profiles) != 0 || len(m.liveChats()) != 1 {
		t.Fatal("chat board deletion removed live agent or failed")
	}
}
