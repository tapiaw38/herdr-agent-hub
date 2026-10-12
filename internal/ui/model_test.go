package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/vt"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui/components"
)

func TestSanitizeTerminalOutput(t *testing.T) {
	got := sanitizeTerminal("ok\x1b[31mred\x1b[0m\x07\nnext")
	if got != "ok[31mred[0m\nnext" {
		t.Fatalf("sanitized output=%q", got)
	}
}

func TestListViewportFollowsSelectionAndBackKey(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 80, 20
	for i := 0; i < 6; i++ {
		m.agents = append(m.agents, agents.Agent{ID: fmt.Sprintf("pane-%d", i), Name: fmt.Sprintf("Agent-%d", i), Provider: "codex"})
	}
	for i := 0; i < 5; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(Model)
	}
	if m.cursor != 5 || m.listOffset == 0 {
		t.Fatalf("cursor=%d offset=%d; expected scrolling selection", m.cursor, m.listOffset)
	}
	view := m.render()
	if !strings.Contains(view, "Agent-5") || strings.Contains(view, "Agent-0") {
		t.Fatalf("visible rows did not follow selection: %s", view)
	}
	m.view = "detail"
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.view != "dashboard" {
		t.Fatalf("B returned to view %q", m.view)
	}
}

func TestFavoritePersistsForDetectedAgent(t *testing.T) {
	dir := t.TempDir()
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, dir)
	m.setLiveAgents([]agents.Agent{{ID: "w1:p1", PaneID: "w1:p1", Name: "Codex", Provider: "codex"}})
	m.toggleFavorite()
	loaded, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Favorites) != 1 || loaded.Favorites[0] != "local|w1:p1" {
		t.Fatalf("persisted favorites=%v", loaded.Favorites)
	}
	if len(m.agents) != 1 || !m.agents[0].Favorite {
		t.Fatalf("live agent was not marked favorite: %+v", m.agents)
	}
}

func TestFavoritesAreScopedToHerdrServer(t *testing.T) {
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion, Favorites: []string{"server-a|w1:p1"}}, t.TempDir(), InvocationContext{ServerScope: "server-b"})
	m.setLiveAgents([]agents.Agent{{ID: "w1:p1", Name: "Agent", Provider: "codex"}})
	if m.agents[0].Favorite {
		t.Fatal("favorite from another Herdr server leaked into this view")
	}
}

func TestProfileEditorDoesNotReuseStaleInvocationWorkspace(t *testing.T) {
	ctx := InvocationContext{WorkspaceID: "w4", WorkspaceCWD: "/repo", FocusedPaneID: "w4:p2", FocusedPaneCWD: "/repo/src", FocusedPaneAgent: "claude"}
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir(), ctx)
	m.startEditor(false)
	if got := m.edit[6].Value(); got != "" {
		t.Fatalf("working directory=%q", got)
	}
	if got := m.edit[4].Value(); got != "claude" {
		t.Fatalf("provider=%q", got)
	}
	if got := m.edit[8].Value(); got != "" {
		t.Fatalf("workspace=%q", got)
	}
	if got := m.edit[9].Value(); got != "" {
		t.Fatalf("pane=%q", got)
	}
}

func TestNewAgentDefaultsToWorkspaceThatOpenedHub(t *testing.T) {
	stateDir := t.TempDir()
	if err := storage.SaveLaunchWorkspace(stateDir, "socket-a", storage.LaunchWorkspace{WorkspaceID: "w3", CWD: "/project"}); err != nil {
		t.Fatal(err)
	}
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir(), InvocationContext{WorkspaceID: "w-old", WorkspaceCWD: "/plugin", StateDir: stateDir, SocketPath: "socket-a"})
	m.startEditor(false)
	if m.edit[8].Value() != "" || m.edit[6].Value() != "" {
		t.Fatalf("new template unexpectedly tied to workspace/cwd: workspace=%q cwd=%q", m.edit[8].Value(), m.edit[6].Value())
	}
	if err := storage.SaveLaunchWorkspace(stateDir, "socket-a", storage.LaunchWorkspace{WorkspaceID: "w4", CWD: "/other"}); err != nil {
		t.Fatal(err)
	}
	m.startAgent()
	if m.startWorkspaceID != "w4" || m.startWorkdir != "/other" {
		t.Fatalf("new launch workspace=%q cwd=%q", m.startWorkspaceID, m.startWorkdir)
	}
}

func TestTerminalViewReturnsWithCtrlG(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.view = "detail"
	m.chatOrigin = "dashboard"
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.view != "dashboard" {
		t.Fatalf("Ctrl+G should return to dashboard: view=%s", m.view)
	}
}

func TestStartAgentRequiresConfirmation(t *testing.T) {
	client := &herdr.Fake{}
	m := New(client, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startAgent()
	m.startPanes = []herdr.Pane{{ID: "w1:p3", ShellAvailable: true, ProcessKnown: true, ProcessName: "bash"}}
	m.startInputs[0].SetValue("reviewer")
	m.startInputs[1].SetValue("codex")
	m.startInputs[2].SetValue("w1:p3")
	m.startInputs[3].SetValue(`["--model","gpt test"]`)
	m.startInputs[4].SetValue("You are reviewer. Wait for tasks.")
	m.startField = 4
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.startConfirm {
		t.Fatal("final form field should show confirmation")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if cmd == nil || !m.startConfirm {
		t.Fatal("confirmation should dispatch asynchronous Herdr start")
	}
	result, ok := cmd().(agentStarted)
	if !ok || result.err != nil || result.promptErr != nil {
		t.Fatal("start command did not return result")
	}
	if client.StartedName != "reviewer" || client.StartedPane != "w1:p3" || len(client.StartedArgs) != 2 || client.StartedArgs[1] != "gpt test" {
		t.Fatalf("unexpected start request: %+v", client)
	}
	if client.PromptedPane != "w1:p3" || !strings.Contains(client.PromptedText, "You are reviewer. Wait for tasks.") {
		t.Fatalf("identity prompt not sent after start: %+v", client)
	}
}

func TestStartAgentConfirmationAcceptsEnter(t *testing.T) {
	client := &herdr.Fake{}
	m := New(client, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startAgent()
	m.startPanes = []herdr.Pane{{ID: "w1:p3", ShellAvailable: true, ProcessKnown: true, ProcessName: "bash"}}
	m.startInputs[0].SetValue("reviewer")
	m.startInputs[1].SetValue("codex")
	m.startInputs[2].SetValue("w1:p3")
	m.startInputs[3].SetValue("[]")
	m.startConfirm = true
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("Enter did not confirm start")
	}
	cmd()
	if client.StartedName != "reviewer" || client.StartedPane != "w1:p3" {
		t.Fatalf("start request name=%q pane=%q", client.StartedName, client.StartedPane)
	}
}

func TestStartAgentProfileSelectionPrefillsConfiguredValues(t *testing.T) {
	profile := storage.Profile{ID: "growth", Name: "Growth Marketer", Provider: "codex", PaneID: "w1:p4", Args: []string{"--model", "gpt-test"}, Instructions: "You are a growth marketer. Wait for user approval before publishing."}
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{profile}}, t.TempDir())
	m.agents = []agents.Agent{{ID: "live", Name: "growth-marketer", Status: "DONE"}}
	m.startAgent()
	if !m.startSelecting || !strings.Contains(m.starterView(), "Growth Marketer") {
		t.Fatal("start wizard should offer saved profiles first")
	}
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.startSelecting {
		t.Fatal("profile selection should advance to launch form")
	}
	if got := m.startInputs[0].Value(); got != "growth-marketer-2" {
		t.Fatalf("agent name=%q", got)
	}
	if got := m.startInputs[1].Value(); got != "codex" {
		t.Fatalf("provider=%q", got)
	}
	if got := m.startInputs[2].Value(); got != "" {
		t.Fatalf("pane=%q", got)
	}
	if got := m.startInputs[3].Value(); got != `["--model","gpt-test"]` {
		t.Fatalf("argv=%q", got)
	}
	if got := m.startInputs[4].Value(); got != profile.Instructions {
		t.Fatalf("identity instructions=%q", got)
	}
}

func TestStartAgentWithBusyProfileRequiresNewPane(t *testing.T) {
	profile := storage.Profile{ID: "assistant", Name: "Assistant", Provider: "claude", PaneID: "w1:p4"}
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{profile}}, t.TempDir())
	m.agents = []agents.Agent{{ID: "w1:p4", PaneID: "w1:p4", Name: "assistant", Status: "IDLE"}}
	m.startAgent()
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.startSelecting || m.startInputs[2].Value() != "" {
		t.Fatal("template should not remain bound to its saved pane")
	}
}

func TestStartSelectorShowsProfilesBeforeManualAndFollowsCursor(t *testing.T) {
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{
		{ID: "one", Name: "Chief of Staff", Provider: "claude", Icon: "pixel1", Color: "#B34CFF"},
		{ID: "two", Name: "Talent Scout", Provider: "codex", Icon: "pixel2", Color: "#2596FF"},
	}}
	m := New(&herdr.Fake{}, cfg, t.TempDir())
	m.width, m.height = 70, 30
	m.startAgent()
	view := m.starterView()
	if !strings.Contains(view, "Chief of Staff") || !strings.Contains(view, "Talent Scout") || !strings.Contains(view, "AGENT TEMPLATES") {
		t.Fatalf("profile selector did not show names and group: %s", view)
	}
	for row, pixels := range components.Avatar("claude", "#B34CFF", "pixel1") {
		if !strings.Contains(view, pixels) {
			t.Fatalf("profile selector cropped avatar row %d: %s", row, view)
		}
	}
	if strings.Index(view, "Start without template") < strings.Index(view, "Chief of Staff") {
		t.Fatal("manual setup appeared before templates")
	}
	m.height = 18
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyDown})
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.startProfileCursor != 2 || !strings.Contains(m.starterView(), "Start without template") {
		t.Fatal("manual setup not reachable after templates")
	}
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.startSelecting || m.startProfileName != "Manual setup" {
		t.Fatal("manual setup did not open")
	}
}

func TestStartAgentSelectsPaneByReadableContext(t *testing.T) {
	client := &herdr.Fake{PaneList: []herdr.Pane{
		{ID: "w3:p2", Workspace: "practiq", Tab: "main", CWD: "/home/user/project", Title: "Terminal", Location: "left", Number: 1, Focused: true, ProcessKnown: true, ProcessName: "zsh", ShellAvailable: true},
		{ID: "w3:p3", Workspace: "practiq", Tab: "main", CWD: "/home/user/project", Title: "Terminal", Location: "right", Number: 2, ProcessKnown: true, ProcessName: "claude"},
	}}
	m := New(client, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startAgent()
	updated, _ := m.Update(panesLoaded{panes: client.PaneList})
	m = updated.(Model)
	m.startChoosingPane = true
	view := m.starterView()
	for _, want := range []string{"practiq", "project", "focused", "Pane 1 · left", "Pane 2 · right", "[READY]", "Foreground: claude", "Create a fresh shell tab"} {
		if !strings.Contains(view, want) {
			t.Fatalf("pane picker missing %q: %s", want, view)
		}
	}
	if strings.Contains(view, "w3:p2") {
		t.Fatalf("pane picker exposed opaque ID: %s", view)
	}
	m.starterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.startInputs[2].Value(); got != "w3:p2" {
		t.Fatalf("selected pane=%q", got)
	}
}

func TestStartAgentCanCreateFreshShellTab(t *testing.T) {
	client := &herdr.Fake{CreatedPane: herdr.Pane{ID: "w3:p9", WorkspaceID: "w3", Workspace: "project", Tab: "Agent", ProcessName: "zsh", ProcessKnown: true, ShellAvailable: true}, WorkspaceList: []herdr.Workspace{{ID: "w3", Label: "project", CWD: "/repo"}}}
	m := NewWithContext(client, storage.Config{Version: storage.SchemaVersion}, t.TempDir(), InvocationContext{WorkspaceID: "w-old", WorkspaceCWD: "/home/user/Workspace/herdr-agent-hub"})
	m.startAgent()
	m.startInputs[0].SetValue("reviewer")
	m.startChoosingPane = true
	cmd := m.starterKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd != nil || !m.choosingWorkspace {
		t.Fatal("N should open workspace picker")
	}
	m.workspaces = client.WorkspaceList
	cmd = m.starterKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.startPaneCreating {
		t.Fatal("choosing workspace should create tab asynchronously")
	}
	message := cmd()
	updated, _ := m.Update(message)
	m = updated.(Model)
	if m.startChoosingPane || m.startInputs[2].Value() != "w3:p9" || !m.selectedPaneReady("w3:p9") {
		t.Fatalf("new pane not selected: %+v", m.startPanes)
	}
	if client.CreatedWorkspace != "w3" || client.CreatedCWD != "/repo" || client.CreatedLabel != "Agent · reviewer" {
		t.Fatalf("create args: %+v", client)
	}
}

func TestPanePickerFitsNarrowTerminal(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 60, 18
	m.startAgent()
	m.startChoosingPane = true
	m.startPanes = []herdr.Pane{{ID: "w1:p2", Number: 1, Location: "right", Title: "a very long terminal title for an agent pane", Workspace: "important-workspace", Tab: "engineering", CWD: "/home/user/a/very/long/path/to/repository", ShellAvailable: true}}
	for _, line := range strings.Split(m.starterView(), "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("pane picker exceeds terminal width: %q", line)
		}
	}
}

func TestStartAgentCannotLaunchTwiceWhileStarting(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startAgent()
	m.startConfirm, m.startLaunching = true, true
	if cmd := m.starterKey(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd != nil {
		t.Fatal("second start command dispatched")
	}
	if !strings.Contains(m.starterView(), "Starting agent in Herdr") {
		t.Fatal("starting state hidden")
	}
}

func TestUncertainAgentStartDoesNotOfferImmediateRetry(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startAgent()
	m.startConfirm, m.startLaunching = true, true
	updated, cmd := m.Update(agentStarted{err: fmt.Errorf("Herdr agent_not_ready: detected blocked prompt")})
	m = updated.(Model)
	if m.view != "dashboard" || m.startLaunching || cmd == nil || !strings.Contains(m.status, "may have started") {
		t.Fatalf("uncertain start result view=%q launching=%v status=%q", m.view, m.startLaunching, m.status)
	}
}

func TestStartAgentDoesNotFallbackToInvocationPaneAndExcludesPluginPane(t *testing.T) {
	m := NewWithContext(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir(), InvocationContext{
		WorkspaceID: "wproject-42", WorkspaceLabel: "project", WorkspaceCWD: "/repo", FocusedPaneID: "wproject-42:p1", FocusedPaneCWD: "/repo", PluginPaneID: "wproject-42:p1",
	})
	m.startAgent()
	updated, _ := m.Update(panesLoaded{panes: []herdr.Pane{{ID: "wproject-42:p1"}, {ID: "wproject-42:p2", ShellAvailable: true}}})
	m = updated.(Model)
	if len(m.startPanes) != 1 || m.startPanes[0].ID != "wproject-42:p2" {
		t.Fatalf("plugin pane was not excluded: %+v", m.startPanes)
	}
	updated, _ = m.Update(panesLoaded{})
	m = updated.(Model)
	if len(m.startPanes) != 0 {
		t.Fatalf("unexpected invocation fallback: %+v", m.startPanes)
	}
	if m.selectedPaneReady("wproject-42:p1") {
		t.Fatal("plugin pane considered launchable")
	}
}

func TestDashboardNavigationAndSearch(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: 1}, t.TempDir())
	m.agents = []agents.Agent{{ID: "one", Name: "Claude"}, {ID: "two", Name: "Codex"}}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor=%d, want 1", m.cursor)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: '/'})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.query != "c" || len(m.filtered()) != 2 {
		t.Fatalf("query=%q filtered=%d", m.query, len(m.filtered()))
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.view != "dashboard" {
		t.Fatalf("search enter view=%q", m.view)
	}
	if m.query != "c" {
		t.Fatalf("search enter lost query: %q", m.query)
	}
}

func TestInteractiveChatShowsEmbeddedTerminalCursor(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 80, 24
	m.view = "detail"
	m.selected = agents.Agent{ID: "agent", PaneID: "w1:p1"}
	m.terminalTarget = "w1:p1"
	m.terminalControllable = true
	m.terminalScreen = vt.NewSafeEmulator(72, 14)
	_, _ = m.terminalScreen.Write([]byte("\x1b[3;5H"))

	view := m.View()
	if view.Cursor == nil {
		t.Fatal("interactive chat did not expose terminal cursor")
	}
	if view.Cursor.X != 4 || view.Cursor.Y != 6 {
		t.Fatalf("cursor position=(%d,%d), want (4,6)", view.Cursor.X, view.Cursor.Y)
	}

	m.view = "chats"
	if m.View().Cursor != nil {
		t.Fatal("read-only chat list should not show interactive terminal cursor")
	}
}

func TestSearchMatchesWorkspaceActivityAndDescription(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.agents = []agents.Agent{
		{ID: "one", Name: "Go dev", Provider: "claude", WorkspaceName: "practiq", Activity: "Landing page design", Description: "Frontend specialist"},
		{ID: "two", Name: "Vue dev", Provider: "codex", WorkspaceName: "marketing", Activity: "Review campaign"},
	}
	for _, query := range []string{"practiq", "Landing", "specialist", "claude"} {
		m.query = query
		if got := m.filtered(); len(got) != 1 || got[0].ID != "one" {
			t.Fatalf("query %q matched %+v", query, got)
		}
	}
}

func TestSearchHandlesUnicodeBackspaceAndPaste(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.view = "search"
	m.query = "café"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(Model)
	if m.query != "caf" {
		t.Fatalf("unicode backspace left query %q", m.query)
	}
	updated, _ = m.Update(tea.PasteMsg{Content: "é"})
	m = updated.(Model)
	if m.query != "café" {
		t.Fatalf("search paste produced %q", m.query)
	}
}

func TestDashboardShowsFavoriteCardsAndCompactFallback(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 100, 40
	m.agents = []agents.Agent{
		{ID: "one", Name: "Chief of Staff", Provider: "claude", Status: "IDLE", Favorite: true},
		{ID: "two", Name: "Talent Scout", Provider: "codex", Status: "WORKING", Activity: "Recruiting workspace", Favorite: true},
	}
	wide := m.render()
	for _, want := range []string{"FEATURED AGENTS", "Chief of Staff", "Talent Scout", "AGENT SESSIONS", "Recruiting workspace"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide dashboard missing %q: %s", want, wide)
		}
	}
	m.width, m.height = 48, 18
	narrow := m.render()
	if strings.Contains(narrow, "FEATURED AGENTS") || !strings.Contains(narrow, "Chief of Staff") {
		t.Fatalf("narrow dashboard should keep agent list and collapse cards: %s", narrow)
	}
}

func TestDashboardSeparatesLiveSessionsFromReusableTemplates(t *testing.T) {
	cfg := storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{
		{ID: "vue-template", Name: "Vue dev", Description: "Frontend with Vue", Provider: "claude", Icon: "pixel2"},
	}}
	m := New(&herdr.Fake{}, cfg, t.TempDir())
	m.width, m.height = 100, 30
	m.setLiveAgents([]agents.Agent{{ID: "w2:p1", PaneID: "w2:p1", Name: "vue-dev", Provider: "claude", Status: "IDLE"}})
	view := m.render()
	for _, want := range []string{"AGENT SESSIONS", "vue-dev", "AGENT TEMPLATES", "Vue dev", "TEMPLATE", "Frontend with Vue"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard missing %q: %s", want, view)
		}
	}
	if strings.Index(view, "AGENT SESSIONS") > strings.Index(view, "AGENT TEMPLATES") {
		t.Fatal("template section appeared before live sessions")
	}
}

func TestStartingTemplateCarriesAvatarIntoLiveSession(t *testing.T) {
	client := &herdr.Fake{}
	dir := t.TempDir()
	profile := storage.Profile{ID: "go-dev", Name: "Go dev", Provider: "claude", Icon: "pixel7", Color: "#2596FF", Description: "Go specialist"}
	m := New(client, storage.Config{Version: storage.SchemaVersion, Profiles: []storage.Profile{profile}}, dir)
	m.startAgent()
	m.startSelecting = false
	m.startPanes = []herdr.Pane{{ID: "w3:p2", ShellAvailable: true, ProcessKnown: true, ProcessName: "bash"}}
	m.prefillStartProfile(profile)
	m.startInputs[0].SetValue("go-dev")
	m.startInputs[1].SetValue("claude")
	m.startInputs[2].SetValue("w3:p2")
	m.startInputs[3].SetValue("[]")
	m.startConfirm = true
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("start command missing")
	}
	started, ok := cmd().(agentStarted)
	if !ok || started.err != nil {
		t.Fatalf("start result=%+v", started)
	}
	updated, _ = m.Update(started)
	m = updated.(Model)
	if len(m.config.Instances) != 1 || m.config.Instances[0].PaneID != "w3:p2" || m.config.Instances[0].AgentName != "go-dev" {
		t.Fatalf("instance binding not saved: %+v", m.config.Instances)
	}
	loaded, err := storage.Load(dir)
	if err != nil || len(loaded.Instances) != 1 || loaded.Instances[0].Icon != "pixel7" {
		t.Fatalf("persisted instance=%+v err=%v", loaded.Instances, err)
	}
	m.setLiveAgents([]agents.Agent{{ID: "w3:p2", PaneID: "w3:p2", Name: "go-dev", Provider: "claude", Status: "IDLE"}})
	var found bool
	for _, agent := range m.agents {
		if agent.PaneID == "w3:p2" {
			found = agent.Custom && agent.Icon == "pixel7" && agent.Color == "#2596FF" && agent.Name == "Go dev"
		}
	}
	if !found {
		t.Fatalf("live session did not inherit template avatar: %+v", m.agents)
	}
	m.setLiveAgents([]agents.Agent{{ID: "w3:p2", PaneID: "w3:p2", Name: "different-agent", Provider: "claude", Status: "IDLE"}})
	for _, agent := range m.agents {
		if agent.PaneID == "w3:p2" && agent.Icon == "pixel7" {
			t.Fatal("template avatar leaked onto a different agent after pane reuse")
		}
	}
}

func TestAgentListRendersFullPixelAvatar(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 90, 30
	m.agents = []agents.Agent{{ID: "local-1", Name: "Pixel Agent", Provider: "codex", Status: "IDLE", Icon: "pixel7", Color: "#F044A1", Custom: true}}
	view := m.render()
	for row, pixels := range components.Avatar("codex", "#F044A1", "pixel7") {
		if !strings.Contains(view, pixels) {
			t.Fatalf("dashboard omitted avatar row %d: %s", row, view)
		}
	}
}

func TestEditorOffersPredefinedAgentIcons(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.startEditor(false)
	if got := m.edit[2].Value(); got != "pixel1" {
		t.Fatalf("default icon=%q, want pixel1", got)
	}
	m.editField = 2
	m.editorKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.edit[2].Value(); got != "pixel2" {
		t.Fatalf("down selected icon=%q, want pixel2", got)
	}
	if got := m.edit[3].Value(); got != "#2596FF" {
		t.Fatalf("avatar selection did not apply its palette color: %q", got)
	}
	m.editorKey(tea.KeyPressMsg{Code: '8', Text: "8"})
	if got := m.edit[2].Value(); got != "pixel8" {
		t.Fatalf("quick select icon=%q, want pixel8", got)
	}
	m.editorKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.edit[2].Value(); got != "pixel8" {
		t.Fatalf("icon field accepted free text: %q", got)
	}
	if view := m.editorView(); !strings.Contains(view, "Pick a pixel avatar") || !strings.Contains(view, "Capsule") {
		t.Fatalf("editor did not show pixel avatar palette: %s", view)
	}
}
