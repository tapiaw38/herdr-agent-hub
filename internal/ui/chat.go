package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/theme"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui/components"
)

// Live chat view pairs agent navigation with terminal output.
func (m Model) liveChats() []agents.Agent {
	var out []agents.Agent
	for _, a := range m.agents {
		if a.PaneID != "" && a.Status != "OFFLINE" {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) readAgent(a agents.Agent) tea.Cmd {
	reader, ok := m.client.(herdr.Reader)
	if !ok || a.PaneID == "" || a.Status == "OFFLINE" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		value, err := reader.Read(ctx, a)
		return terminalRead{paneID: a.PaneID, text: value, err: err}
	}
}

func (m Model) readSelected() tea.Cmd { return m.readAgent(m.selected) }

func (m *Model) openChat(a agents.Agent, origin string) tea.Cmd {
	m.selected, m.chatOrigin, m.view = a, origin, "detail"
	m.terminalOutput = ""
	m.terminalLines = nil
	m.terminalTop = 0
	m.outputFocus = false
	m.err = ""
	return m.startTerminalStream()
}

func (m Model) readPreviews() tea.Cmd {
	reader, ok := m.client.(herdr.Reader)
	if !ok {
		return nil
	}
	list := m.liveChats()
	if len(list) == 0 {
		return nil
	}
	a := list[min(m.chatCursor, len(list)-1)]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		value, err := reader.Read(ctx, a)
		return previewRead{paneID: a.PaneID, text: value, err: err}
	}
}

func (m *Model) chatKey(key tea.KeyPressMsg) tea.Cmd {
	list := m.liveChats()
	switch strings.ToLower(key.String()) {
	case "esc", "b":
		m.view = "dashboard"
		return m.stopTerminalStream()
	case "q", "ctrl+c":
		return m.quitWithTerminalCleanup()
	case "left", "up", "h", "k":
		if m.chatCursor > 0 {
			m.chatCursor--
			return m.startTerminalStream()
		}
		return nil
	case "right", "down", "l", "j":
		if m.chatCursor < len(list)-1 {
			m.chatCursor++
			return m.startTerminalStream()
		}
		return nil
	case "r":
		return tea.Batch(m.refresh(), m.startTerminalStream())
	case "d":
		if len(list) > 0 {
			m.beginDelete(list[min(m.chatCursor, len(list)-1)])
		}
	case "enter":
		if len(list) > 0 {
			return m.openChat(list[min(m.chatCursor, len(list)-1)], "chats")
		}
	}
	return nil
}

func terminalKey(key tea.KeyPressMsg) (string, bool) {
	name := strings.ToLower(key.String())
	switch name {
	case "enter", "esc", "tab", "up", "down", "left", "right":
		return name, true
	}
	if len(key.Text) == 1 && key.Text[0] >= 0x20 && key.Text[0] <= 0x7e {
		return key.Text, true
	}
	if len(name) == 1 && name[0] >= 0x20 && name[0] <= 0x7e {
		return name, true
	}
	return "", false
}

func terminalInput(key tea.KeyPressMsg) ([]byte, bool) {
	if key.Text != "" {
		return []byte(key.Text), true
	}
	name := strings.ToLower(key.String())
	switch name {
	case "enter":
		return []byte("\r"), true
	case "esc":
		return []byte{0x1b}, true
	case "tab":
		return []byte("\t"), true
	case "backspace":
		return []byte{0x7f}, true
	case "delete":
		return []byte("\x1b[3~"), true
	case "up":
		return []byte("\x1b[A"), true
	case "down":
		return []byte("\x1b[B"), true
	case "right":
		return []byte("\x1b[C"), true
	case "left":
		return []byte("\x1b[D"), true
	case "home":
		return []byte("\x1b[H"), true
	case "end":
		return []byte("\x1b[F"), true
	case "pgup", "pageup":
		return []byte("\x1b[5~"), true
	case "pgdown", "pagedown":
		return []byte("\x1b[6~"), true
	}
	if strings.HasPrefix(name, "ctrl+") && len(name) == len("ctrl+")+1 {
		ch := name[len("ctrl+")]
		if ch >= 'a' && ch <= 'z' {
			return []byte{ch - 'a' + 1}, true
		}
		if ch == '[' {
			return []byte{0x1b}, true
		}
	}
	if key.Code >= 0x20 && key.Code <= 0x7e {
		return []byte{byte(key.Code)}, true
	}
	return nil, false
}

func terminalExcerpt(s string, width, rows int) []string {
	var lines []string
	for _, raw := range strings.Split(sanitizeTerminal(s), "\n") {
		line := strings.TrimSpace(raw)
		if line != "" {
			lines = append(lines, truncate(line, width))
		}
	}
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	return lines
}

func (m Model) chatsView() string {
	width := max(24, m.width-4)
	list := m.liveChats()
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.White).Render("◈ AGENT HUB / CHATS"),
		lipgloss.NewStyle().Foreground(theme.Muted).Render("Select an agent; Enter opens its live terminal"), "",
	}
	if m.confirm {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("Delete local profile ‘"+truncate(displayText(m.selected.Name), max(8, width-36))+"’? Y confirm · Esc cancel"),
			"Herdr pane stays open.", "")
	}
	if m.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, width)), "")
	}
	if len(list) == 0 {
		lines = append(lines, "No live agents. Start one with A on dashboard.")
	} else {
		selected := min(m.chatCursor, len(list)-1)
		if width >= 76 {
			bodyHeight := max(8, m.height-11)
			listWidth := min(36, max(24, width/4))
			conversationWidth := width - listWidth - 3
			capacity := max(1, (bodyHeight-2)/4)
			start := max(0, min(selected-capacity+1, len(list)-capacity))
			end := min(len(list), start+capacity)
			items := make([]string, 0, end-start)
			for i := start; i < end; i++ {
				a := list[i]
				avatar := components.Avatar(a.Provider, a.Color, a.Icon)
				state := lipgloss.NewStyle().Foreground(statusColor(a.Status)).Render(a.Status)
				name := truncate(displayText(a.Name), listWidth-12)
				provider := truncate(displayText(a.Provider), listWidth-8)
				item := []string{
					fmt.Sprintf("%s %s", avatar[0], name),
					fmt.Sprintf("%s %s", avatar[1], state),
					fmt.Sprintf("%s %s", avatar[2], provider),
					avatar[3],
				}
				style := lipgloss.NewStyle().Width(listWidth-2).Padding(0, 1)
				if i == selected {
					style = style.Foreground(theme.White).Background(theme.Panel).BorderLeft(true).BorderForeground(theme.Green)
				}
				items = append(items, style.Render(strings.Join(item, "\n")))
			}
			left := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(theme.Muted).Render("AGENTS\n\n" + strings.Join(items, "\n\n"))
			a := list[selected]
			avatar := components.Avatar(a.Provider, a.Color, a.Icon)
			header := fmt.Sprintf("%s  %s\n%s · %s · %s", displayText(a.Name), a.Status,
				displayText(a.Provider), displayText(firstNonempty(a.WorkspaceName, a.WorkspaceID)), displayText(a.PaneID))
			content := []string{header, avatar[0], avatar[1], avatar[2], avatar[3], "", "LIVE HERDR TERMINAL", ""}
			terminal := m.terminalScreenFor(a.PaneID)
			if terminal != "" {
				content = append(content, strings.Split(terminal, "\n")...)
			} else if m.terminalStreamErr != "" && m.terminalTarget == a.PaneID {
				content = append(content, lipgloss.NewStyle().Foreground(theme.Yellow).Render("Live terminal unavailable: "+truncate(m.terminalStreamErr, conversationWidth-10)))
				content = append(content, terminalExcerpt(m.chatPreviews[a.PaneID], conversationWidth-8, max(3, bodyHeight-14))...)
			} else {
				content = append(content, "Connecting to Herdr terminal…")
			}
			content = append(content, "", "Wheel scroll output · Enter open · ↑/↓ switch")
			right := lipgloss.NewStyle().Width(conversationWidth).Height(bodyHeight).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(theme.Muted).Render(strings.Join(content, "\n"))
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
			footer := fmt.Sprintf("%d/%d · ↑/↓ agent · Enter open · R refresh · D delete · B/Esc dashboard", selected+1, len(list))
			lines = append(lines, footer)
		} else {
			a := list[selected]
			preview := m.chatPreviews[a.PaneID]
			lines = append(lines, fmt.Sprintf("%s · %s · %s", displayText(a.Name), a.Status, displayText(a.Provider)))
			lines = append(lines, terminalExcerpt(preview, width, max(3, m.height-10))...)
			lines = append(lines, "↑/↓ switch agent · Enter open terminal · B/Esc dashboard")
		}
	}
	return lipgloss.NewStyle().Padding(1, 2).Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}
