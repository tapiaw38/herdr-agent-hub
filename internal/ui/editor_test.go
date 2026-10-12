package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
)

func TestNewProfileCompactFlowAndSave(t *testing.T) {
	dir := t.TempDir()
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, dir)
	m.width, m.height = 70, 20
	m.startEditor(false)
	m.edit[0].SetValue("Research Helper")
	for _, want := range []int{1, 2, 3, 4, 6, 9, 10} {
		m.editorKey(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.editField != want {
			t.Fatalf("Tab reached field %d, want %d", m.editField, want)
		}
	}
	m.edit[10].SetValue("Act as research analyst. Cite sources; wait for tasks.")
	view := m.editorView()
	if !strings.Contains(view, "Ctrl+S save") || !strings.Contains(view, "Choose Herdr workspace when starting") {
		t.Fatalf("new profile instructions missing: %s", view)
	}
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.view != "dashboard" || len(m.config.Profiles) != 1 {
		t.Fatal("Enter on final field did not save profile")
	}
	loaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "Research Helper" || loaded.Profiles[0].Instructions != "Act as research analyst. Cite sources; wait for tasks." {
		t.Fatalf("saved profiles=%+v", loaded.Profiles)
	}
}

func TestProviderPickerAndAdvancedFields(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startEditor(false)
	m.editorKey(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	if !m.editorChoosingProvider || !strings.Contains(m.editorView(), "CHOOSE PROVIDER") {
		t.Fatal("provider picker did not open")
	}
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.edit[4].Value() != "codex" || m.editorChoosingProvider {
		t.Fatalf("provider=%q", m.edit[4].Value())
	}
	m.editorKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if !m.editAdvanced || len(m.editorFields()) != 10 {
		t.Fatal("advanced fields not shown")
	}
}

func TestNewProfileCanLinkActiveAgentPane(t *testing.T) {
	dir := t.TempDir()
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, dir, InvocationContext{WorkspaceID: "w2", WorkspaceCWD: "/other", FocusedPaneAgent: "codex", FocusedPaneCWD: "/other"})
	m.liveAgents = []agents.Agent{{ID: "w1:p2", PaneID: "w1:p2", Name: "golang-developer", Provider: "claude", Status: "IDLE", WorkspaceID: "w1", CWD: "/repo"}}
	m.startEditor(false)
	m.startPanes = []herdr.Pane{{ID: "w1:p2", Agent: "claude", Workspace: "project", Title: "Claude", Number: 2}}
	m.editorChoosingPane = true
	if !strings.Contains(m.editorView(), "golang-developer") {
		t.Fatal("pane picker omitted live agent name")
	}
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editorChoosingPane || m.edit[9].Value() != "w1:p2" || m.edit[0].Value() != "golang-developer" || m.edit[4].Value() != "claude" || m.edit[8].Value() != "" || m.edit[6].Value() != "/repo" {
		t.Fatalf("live pane did not prefill agent fields: name=%q provider=%q pane=%q", m.edit[0].Value(), m.edit[4].Value(), m.edit[9].Value())
	}
	m.editorKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.view != "dashboard" || len(m.agents) != 1 || !m.agents[0].Custom {
		t.Fatal("linked profile not saved over live agent")
	}
}

func TestShellPaneSelectionUsesItsWorkspace(t *testing.T) {
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir(), InvocationContext{WorkspaceID: "w-old", WorkspaceCWD: "/plugin"})
	m.startEditor(false)
	m.startPanes = []herdr.Pane{{ID: "w3:p2", WorkspaceID: "w3", Workspace: "project", CWD: "/project", ShellAvailable: true}}
	m.editorChoosingPane = true
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.edit[8].Value() != "" || m.edit[6].Value() != "/project" {
		t.Fatalf("workspace=%q cwd=%q", m.edit[8].Value(), m.edit[6].Value())
	}
}

func TestProfileSaveFailureKeepsEditorAndConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(dir, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, dir)
	m.startEditor(false)
	m.edit[0].SetValue("Keep input")
	m.editorKey(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.view != "editor" || len(m.config.Profiles) != 0 || !strings.Contains(m.err, "Could not save") || m.edit[0].Value() != "Keep input" {
		t.Fatalf("save failure lost draft or config: view=%q err=%q", m.view, m.err)
	}
}

func TestEditorRejectsPaneLinkedToAnotherProfile(t *testing.T) {
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{{ID: "taken", Name: "Taken", PaneID: "w1:p2"}}}
	m := New(&herdr.Fake{}, cfg, t.TempDir())
	m.startEditor(false)
	m.startPanes = []herdr.Pane{{ID: "w1:p2", Agent: "claude"}}
	m.editorChoosingPane = true
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.editorChoosingPane || m.edit[9].Value() != "" || !strings.Contains(m.err, "already used") {
		t.Fatal("duplicate pane association was allowed")
	}
}

func TestEditorFitsNarrowTerminalAtAvatarSelection(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 60, 20
	m.startEditor(false)
	m.editField = 2
	view := m.editorView()
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("editor uses %d rows in %d-row terminal", len(lines), m.height)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("editor line exceeds terminal width: %q", line)
		}
	}
	if !strings.Contains(view, "Ctrl+S save") {
		t.Fatal("save shortcut hidden")
	}
}

func TestAgentPickerShowsLiveNamesBeforeShells(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 70, 24
	m.setLiveAgents([]agents.Agent{{ID: "w1:p2", PaneID: "w1:p2", Name: "golang-developer", Provider: "claude", Status: "IDLE"}})
	m.startEditor(false)
	m.editorChoosingPane = true
	panes := []herdr.Pane{
		{ID: "w1:p1", Title: "Shell", Workspace: "project", ShellAvailable: true},
		{ID: "w1:p2", Title: "Claude", Agent: "claude", Workspace: "project"},
	}
	updated, _ := m.Update(panesLoaded{panes: panes})
	m = updated.(Model)
	if m.startPanes[0].ID != "w1:p2" {
		t.Fatal("live agent not first in selector")
	}
	view := m.editorView()
	if !strings.Contains(view, "RUNNING AGENTS") || !strings.Contains(view, "golang-developer") || !strings.Contains(view, "EMPTY SHELLS") {
		t.Fatalf("agent picker hid names or shell group: %s", view)
	}
	if strings.Contains(view, "w1:p2") {
		t.Fatal("opaque pane ID leaked into selector")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("selector line exceeds terminal width: %q", line)
		}
	}
}
