package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
	"github.com/tapiaw38/herdr-agent-hub/internal/theme"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui/components"
)

func editorProviders() []string {
	priority := []string{"claude", "codex", "grok", "gemini", "pi", "opencode"}
	seen := make(map[string]bool, len(herdr.SupportedKinds))
	var out []string
	for _, name := range append(priority, herdr.SupportedKinds...) {
		if !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	return out
}

func (m Model) editorFields() []int {
	if m.editAdvanced {
		return []int{0, 1, 2, 3, 4, 5, 6, 7, 9, 10}
	}
	return []int{0, 1, 2, 3, 4, 6, 9, 10}
}

func (m *Model) moveEditorField(direction int) tea.Cmd {
	fields := m.editorFields()
	position := 0
	for i, field := range fields {
		if field == m.editField {
			position = i
			break
		}
	}
	next := position + direction
	if next < 0 || next >= len(fields) {
		return nil
	}
	m.edit[m.editField].Blur()
	m.editField = fields[next]
	if m.editField == 9 {
		return nil
	}
	return m.edit[m.editField].Focus()
}

func (m Model) editorPaneAvailable(pane herdr.Pane) bool {
	if pane.ID == "" || pane.ID == m.invocation.PluginPaneID {
		return false
	}
	for _, profile := range m.config.Profiles {
		if profile.PaneID == pane.ID && profile.ID != m.editingID {
			return false
		}
	}
	if m.paneHasLiveAgent(pane) || pane.ShellAvailable {
		return true
	}
	return false
}

func (m Model) paneHasLiveAgent(pane herdr.Pane) bool {
	if pane.Agent != "" {
		return true
	}
	for _, live := range m.liveAgents {
		if live.PaneID == pane.ID {
			return true
		}
	}
	return false
}

func (m Model) editorPaneAgentName(paneID string) string {
	for _, live := range m.liveAgents {
		if live.PaneID == paneID {
			return live.Name
		}
	}
	return ""
}

func (m *Model) saveEditor() tea.Cmd {
	p := storage.Profile{
		ID:           m.editingID,
		Name:         strings.TrimSpace(m.edit[0].Value()),
		Description:  strings.TrimSpace(m.edit[1].Value()),
		Instructions: strings.TrimSpace(m.edit[10].Value()),
		Icon:         m.edit[2].Value(),
		Color:        strings.TrimSpace(m.edit[3].Value()),
		Provider:     strings.TrimSpace(m.edit[4].Value()),
		Executable:   strings.TrimSpace(m.edit[5].Value()),
		Workdir:      strings.TrimSpace(m.edit[6].Value()),
		WorkspaceID:  "",
		PaneID:       m.edit[9].Value(),
	}
	if p.Name == "" {
		m.err = "Agent name is required"
		m.edit[m.editField].Blur()
		m.editField = 0
		m.edit[0].Focus()
		return nil
	}
	if raw := strings.TrimSpace(m.edit[7].Value()); raw != "" {
		if err := json.Unmarshal([]byte(raw), &p.Args); err != nil {
			m.err = "Arguments must be a JSON array of strings"
			m.edit[m.editField].Blur()
			m.editAdvanced, m.editField = true, 7
			m.edit[7].Focus()
			return nil
		}
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	next := m.config
	next.Profiles = append([]storage.Profile(nil), m.config.Profiles...)
	replaced := false
	for i := range next.Profiles {
		if next.Profiles[i].ID == m.editingID && m.editingID != "" {
			p.Favorite = next.Profiles[i].Favorite
			next.Profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		next.Profiles = append(next.Profiles, p)
	}
	if err := storage.Save(m.cfgDir, next); err != nil {
		m.err = "Could not save profile: " + err.Error()
		return nil
	}
	m.config = next
	m.setLiveAgents(m.liveAgents)
	m.err = ""
	m.view = "dashboard"
	m.status = "Agent template saved. Press A to choose workspace and start a Herdr session."
	for _, live := range m.liveAgents {
		if live.PaneID != "" && live.PaneID == p.PaneID {
			m.status = "Profile linked to running Herdr agent"
			break
		}
	}
	id := p.ID
	if p.PaneID != "" {
		id = p.PaneID
	}
	for i, agent := range m.filtered() {
		if agent.ID == id {
			m.cursor = i
			break
		}
	}
	m.keepCursorVisible()
	return nil
}

func (m *Model) rememberStartedInstance(result agentStarted) error {
	var profile *storage.Profile
	for i := range m.config.Profiles {
		if m.config.Profiles[i].ID == result.profileID {
			profile = &m.config.Profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("template %q no longer exists", result.profileID)
	}
	instance := storage.Instance{
		ProfileID:   profile.ID,
		PaneID:      result.pane,
		AgentName:   result.name,
		Name:        profile.Name,
		Description: profile.Description,
		Provider:    result.provider,
		Icon:        profile.Icon,
		Color:       profile.Color,
	}
	next := m.config
	next.Instances = append([]storage.Instance(nil), m.config.Instances...)
	replaced := false
	for i := range next.Instances {
		if next.Instances[i].PaneID == result.pane {
			next.Instances[i] = instance
			replaced = true
			break
		}
	}
	if !replaced {
		next.Instances = append(next.Instances, instance)
	}
	if err := storage.Save(m.cfgDir, next); err != nil {
		return err
	}
	m.config = next
	return nil
}

func (m Model) providerPickerView() string {
	width := max(32, m.width-4)
	choices := editorProviders()
	limit := max(3, min(10, m.height-8))
	start := min(max(0, m.editProviderCursor-limit+1), max(0, len(choices)-limit))
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.Green).Render("CHOOSE PROVIDER"),
		truncate("Herdr supported kinds. You can also type a custom value in form.", width), "",
	}
	for i := start; i < min(start+limit, len(choices)); i++ {
		marker := "  "
		if i == m.editProviderCursor {
			marker = "› "
		}
		lines = append(lines, marker+choices[i])
	}
	lines = append(lines, "", fmt.Sprintf("%d/%d · ↑↓ choose · Enter use · Esc back", m.editProviderCursor+1, len(choices)))
	return lipgloss.NewStyle().Padding(1, 2).Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
}

func (m Model) editorAgentPickerView() string {
	width := max(28, m.width-4)
	if m.width == 0 {
		width = 80
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.Green).Render("◈ AGENT HUB / CHOOSE AGENT"),
		lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("Link a running agent, or reserve an empty shell. Esc keeps profile unlinked.", width)),
		"",
	}
	if m.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, width)), "")
	}
	if m.startPaneLoading {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render("Checking Herdr panes…"))
	}
	if m.startPaneCreating {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render("Creating shell tab…"))
	}
	if len(m.startPanes) == 0 && !m.startPaneLoading {
		lines = append(lines, "No agents or shells found. Press R to refresh.")
	}
	limit := len(m.startPanes)
	if m.height > 0 {
		limit = max(1, (m.height-10)/3)
	}
	offset := min(max(0, m.startPaneCursor-limit+1), max(0, len(m.startPanes)-limit))
	group := ""
	for i := offset; i < min(len(m.startPanes), offset+limit); i++ {
		pane := m.startPanes[i]
		active := m.paneHasLiveAgent(pane)
		label := "EMPTY SHELLS"
		if active {
			label = "RUNNING AGENTS"
		}
		if group != label {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Purple).Bold(true).Render(label))
			group = label
		}
		marker := "  "
		if i == m.startPaneCursor {
			marker = lipgloss.NewStyle().Foreground(theme.Green).Render("› ")
		}
		name, provider, color, icon := "", "", "", ""
		for _, live := range m.liveAgents {
			if live.PaneID == pane.ID {
				name, provider, color, icon = live.Name, live.Provider, live.Color, live.Icon
				break
			}
		}
		state, stateColor := "READY", theme.Green
		if !m.editorPaneAvailable(pane) {
			state, stateColor = "UNAVAILABLE", theme.Red
		}
		if active {
			if name == "" {
				name = firstNonempty(pane.Title, pane.Agent, "Agent")
			}
			if provider == "" {
				provider = firstNonempty(pane.Agent, "agent")
			}
			if state == "READY" {
				state, stateColor = "LINK AGENT", theme.Purple
			}
			avatar := components.Avatar(provider, color, icon)
			lines = append(lines, marker+avatar[1]+"  "+truncate(displayText(name), max(8, width-32))+"  "+lipgloss.NewStyle().Foreground(stateColor).Render("["+state+"]"),
				"  "+avatar[2]+"  "+truncate(displayText(provider+" · "+firstNonempty(pane.Workspace, pane.CWD)), max(8, width-16)))
		} else {
			name = firstNonempty(pane.Title, pane.Tab, "Shell")
			lines = append(lines, marker+truncate(displayText(name), max(8, width-25))+"  "+lipgloss.NewStyle().Foreground(stateColor).Render("["+state+"]"),
				"  "+truncate(displayText(firstNonempty(pane.Workspace, "workspace")+" · "+firstNonempty(pane.CWD, pane.Location)), max(8, width-4)))
		}
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("%d/%d · ↑↓ select · Enter link · N new shell · R refresh · Esc back", min(m.startPaneCursor+1, len(m.startPanes)), len(m.startPanes)))
	return lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(strings.Join(lines, "\n"))
}
