package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/vt"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
)

type chatClient struct {
	herdr.Fake
	reads map[string]string
}

func (c *chatClient) Read(_ context.Context, a agents.Agent) (string, error) {
	return c.reads[a.PaneID], nil
}

func TestChatsShowOnlyLiveAgentsAndRealOutput(t *testing.T) {
	c := &chatClient{reads: map[string]string{"w1:p1": "actual terminal response"}}
	m := New(c, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 100, 30
	m.agents = []agents.Agent{
		{ID: "w1:p1", PaneID: "w1:p1", Name: "Developer", Provider: "claude", Status: "IDLE"},
		{ID: "w1:p2", PaneID: "w1:p2", Name: "Researcher", Provider: "codex", Status: "WORKING"},
		{ID: "old", Name: "Old profile", Status: "OFFLINE"},
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = updated.(Model)
	if m.view != "chats" || cmd == nil || len(m.liveChats()) != 2 {
		t.Fatal("live chat board did not open")
	}
	updated, _ = m.Update(previewRead{paneID: "w1:p1", text: c.reads["w1:p1"]})
	m = updated.(Model)
	view := m.render()
	for _, want := range []string{"AGENTS", "Developer", "Researcher", "actual terminal response", "LIVE HERDR TERMINAL"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q from chat board", want)
		}
	}
	if strings.Contains(view, "Old profile") {
		t.Fatal("offline profile displayed as live chat")
	}
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.view != "detail" || m.chatOrigin != "chats" || cmd == nil {
		t.Fatal("Enter did not open conversation")
	}
	updated, _ = m.Update(terminalRead{paneID: "w1:p1", text: c.reads["w1:p1"]})
	m = updated.(Model)
	if !strings.Contains(m.render(), "actual terminal response") {
		t.Fatal("conversation missing terminal output")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.view != "chats" {
		t.Fatal("Esc did not return to chat board")
	}
}

func TestChatInboxLoadsSelectedAgentOutput(t *testing.T) {
	c := &chatClient{reads: map[string]string{"w1:p1": "first conversation", "w1:p2": "second conversation"}}
	m := New(c, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 120, 32
	m.agents = []agents.Agent{
		{ID: "w1:p1", PaneID: "w1:p1", Name: "First", Provider: "claude", Status: "IDLE"},
		{ID: "w1:p2", PaneID: "w1:p2", Name: "Second", Provider: "codex", Status: "WORKING"},
	}
	m.view = "chats"
	cmd := m.readPreviews()
	if cmd == nil {
		t.Fatal("selected agent output was not requested")
	}
	msg, ok := cmd().(previewRead)
	if !ok || msg.paneID != "w1:p1" || msg.text != "first conversation" {
		t.Fatalf("initial inbox read=%+v", msg)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	cmd = m.readPreviews()
	msg, ok = cmd().(previewRead)
	if !ok || msg.paneID != "w1:p2" || msg.text != "second conversation" {
		t.Fatalf("selected inbox read=%+v", msg)
	}
}

func TestConversationSendsKeysDirectly(t *testing.T) {
	c := &herdr.Fake{}
	m := New(c, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.selected = agents.Agent{ID: "w1:p1", PaneID: "w1:p1", Name: "Waiting", Provider: "claude", Status: "WAITING"}
	m.view, m.chatOrigin = "detail", "chats"
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("direct terminal input command missing")
	}
	result, ok := cmd().(terminalKeySent)
	if !ok || result.err != nil || c.SentPane != "w1:p1" || c.SentKey != "y" {
		t.Fatalf("direct send=%+v pane=%q key=%q", result, c.SentPane, c.SentKey)
	}
	updated, _ = m.Update(result)
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("Enter was not sent to terminal")
	}
	result, ok = cmd().(terminalKeySent)
	if !ok || result.err != nil || c.SentKey != "enter" {
		t.Fatalf("terminal Enter=%+v key=%q", result, c.SentKey)
	}
	updated, _ = m.Update(result)
	m = updated.(Model)
	updated, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("Esc was not sent to agent")
	}
	result, ok = cmd().(terminalKeySent)
	if !ok || result.err != nil || c.SentKey != "esc" {
		t.Fatalf("terminal Esc=%+v key=%q", result, c.SentKey)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.view != "chats" {
		t.Fatal("Ctrl+G did not return to agent list")
	}
}

func TestStaleTerminalReadIgnored(t *testing.T) {
	m := New(&chatClient{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.view = "detail"
	m.selected = agents.Agent{PaneID: "w1:p2"}
	updated, _ := m.Update(terminalRead{paneID: "w1:p1", text: "wrong agent"})
	m = updated.(Model)
	if m.terminalOutput != "" {
		t.Fatal("stale terminal output replaced active conversation")
	}
}

func TestNarrowChatBoardFollowsSelection(t *testing.T) {
	m := New(&chatClient{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 50, 25
	m.agents = []agents.Agent{
		{ID: "w1:p1", PaneID: "w1:p1", Name: "First", Provider: "claude", Status: "IDLE"},
		{ID: "w1:p2", PaneID: "w1:p2", Name: "Second", Provider: "codex", Status: "WORKING"},
	}
	m.view, m.chatCursor = "chats", 1
	view := m.render()
	if !strings.Contains(view, "Second") || strings.Contains(view, "First") {
		t.Fatalf("narrow chat card did not follow selected agent: %s", view)
	}
}

func TestEmbeddedTerminalScrollbackMouseAndRender(t *testing.T) {
	m := New(&herdr.Fake{}, storage.Config{Version: storage.SchemaVersion}, t.TempDir())
	m.width, m.height = 80, 24
	m.view = "detail"
	m.terminalTarget = "w1:p1"
	m.terminalScreen = vt.NewSafeEmulator(24, 4)
	m.terminalScreen.SetScrollbackSize(100)
	_, _ = m.terminalScreen.Write([]byte("first line\r\nsecond line\r\nthird line\r\nfourth line\r\nfifth line"))
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("interactive terminal view did not enable mouse wheel input")
	}
	if m.terminalScreen.ScrollbackLen() == 0 {
		t.Fatal("test terminal did not produce scrollback")
	}

	updated, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = updated.(Model)
	if m.terminalScrollOffset == 0 {
		t.Fatal("mouse wheel did not scroll terminal history")
	}
	output := m.terminalScreenFor("w1:p1")
	if !strings.Contains(output, "first line") {
		t.Fatalf("scrolled terminal omitted older output: %q", output)
	}

	m.terminalStream = &herdr.TerminalStream{}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = updated.(Model)
	if m.terminalScrollOffset >= 3 {
		t.Fatalf("PageDown did not return terminal toward live output: offset=%d", m.terminalScrollOffset)
	}
}
