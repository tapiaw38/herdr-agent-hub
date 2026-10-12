package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/vt"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
	"github.com/tapiaw38/herdr-agent-hub/internal/theme"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui/components"
)

type Model struct {
	client                 herdr.Client
	config                 storage.Config
	cfgDir                 string
	agents                 []agents.Agent
	liveAgents             []agents.Agent
	cursor                 int
	listOffset             int
	width, height          int
	loading                bool
	connected              bool
	err, status, query     string
	view                   string
	edit                   []textinput.Model
	terminalOutput         string
	terminalLines          []string
	terminalTop            int
	outputFocus            bool
	invocation             InvocationContext
	startInputs            []textinput.Model
	startField             int
	startSelecting         bool
	startProfileCursor     int
	startProfileName       string
	startProfileID         string
	startInstructions      string
	startChoosingPane      bool
	editorChoosingPane     bool
	editorChoosingProvider bool
	editAdvanced           bool
	editProviderCursor     int
	startPaneCursor        int
	startPanes             []herdr.Pane
	workspaces             []herdr.Workspace
	workspaceCursor        int
	choosingWorkspace      bool
	workspaceCreate        bool
	startPaneLoading       bool
	startPaneCreating      bool
	startConfirm           bool
	startLaunching         bool
	startWorkspaceID       string
	startWorkdir           string
	editField              int
	editingID              string
	selected               agents.Agent
	confirm                bool
	chatOrigin             string
	chatPreviews           map[string]string
	chatCursor             int
	terminalSending        bool
	terminalInputQueue     []pendingTerminalInput
	terminalStream         *herdr.TerminalStream
	terminalControllable   bool
	terminalOpenCancel     context.CancelFunc
	terminalScreen         *vt.SafeEmulator
	terminalScrollOffset   int
	terminalTarget         string
	terminalGeneration     uint64
	terminalStreamCols     int
	terminalStreamRows     int
	terminalStreamErr      string
}

type InvocationContext struct {
	WorkspaceID      string `json:"workspace_id"`
	WorkspaceLabel   string `json:"workspace_label"`
	WorkspaceCWD     string `json:"workspace_cwd"`
	TabID            string `json:"tab_id"`
	TabLabel         string `json:"tab_label"`
	FocusedPaneID    string `json:"focused_pane_id"`
	FocusedPaneCWD   string `json:"focused_pane_cwd"`
	FocusedPaneAgent string `json:"focused_pane_agent"`
	PluginPaneID     string `json:"-"`
	ServerScope      string `json:"-"`
	StateDir         string `json:"-"`
	SocketPath       string `json:"-"`
}
type loaded struct {
	agents []agents.Agent
	err    error
}
type focused struct{ err error }
type terminalRead struct {
	paneID string
	text   string
	err    error
}
type previewRead struct {
	paneID string
	text   string
	err    error
}
type terminalKeySent struct {
	paneID string
	err    error
}
type pendingTerminalInput struct {
	data []byte
	key  string
}
type terminalStreamStarted struct {
	target     string
	generation uint64
	observer   *herdr.TerminalStream
	control    bool
	err        error
}
type terminalFrameReceived struct {
	target     string
	generation uint64
	frame      herdr.TerminalFrame
	err        error
}
type agentStarted struct {
	err, promptErr                  error
	name, pane, provider, profileID string
}
type paneCreated struct {
	pane herdr.Pane
	err  error
}
type panesLoaded struct {
	panes        []herdr.Pane
	workspaces   []herdr.Workspace
	err          error
	workspaceErr error
}

type iconChoice struct {
	id    string
	name  string
	color string
}

var agentIconChoices = []iconChoice{
	{id: "pixel1", name: "Orbit", color: "#B34CFF"},
	{id: "pixel2", name: "Hood", color: "#2596FF"},
	{id: "pixel3", name: "Cloud", color: "#05D68B"},
	{id: "pixel4", name: "Mint", color: "#12D6A0"},
	{id: "pixel5", name: "Amber", color: "#B96830"},
	{id: "pixel6", name: "Capsule", color: "#FF315B"},
	{id: "pixel7", name: "Triangle", color: "#F044A1"},
	{id: "pixel8", name: "Square", color: "#714BFF"},
}

func New(c herdr.Client, cfg storage.Config, dir string) Model {
	return Model{client: c, config: cfg, cfgDir: dir, view: "dashboard", loading: true}
}

func NewWithContext(c herdr.Client, cfg storage.Config, dir string, invocation InvocationContext) Model {
	m := New(c, cfg, dir)
	m.invocation = invocation
	return m
}
func (m Model) Init() tea.Cmd { return m.refresh() }
func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, c := context.WithTimeout(context.Background(), 6*time.Second)
		defer c()
		a, e := m.client.Snapshot(ctx)
		return loaded{a, e}
	}
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.PasteMsg:
		if m.view == "detail" && m.terminalStream != nil && m.terminalControllable {
			return m, m.sendTerminalBytes([]byte(x.Content))
		}
		if m.view == "search" {
			m.query += x.Content
			m.cursor, m.listOffset = 0, 0
			m.keepCursorVisible()
		}
		return m, nil
	case tea.MouseWheelMsg:
		if m.view != "detail" && m.view != "chats" {
			return m, nil
		}
		delta := 0
		switch x.Button {
		case tea.MouseWheelUp:
			delta = 3
		case tea.MouseWheelDown:
			delta = -3
		default:
			return m, nil
		}
		if m.terminalScreen != nil && !m.terminalScreen.IsAltScreen() && m.terminalScreen.ScrollbackLen() > 0 {
			m.scrollTerminalScreen(delta)
			return m, nil
		}
		if m.view == "detail" && m.terminalScreen != nil && m.terminalScreen.IsAltScreen() {
			sequence := []byte("\x1b[5~")
			if delta < 0 {
				sequence = []byte("\x1b[6~")
			}
			return m, m.sendTerminalBytes(sequence)
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
		m.keepCursorVisible()
		if (m.view == "chats" || m.view == "detail") && m.terminalTarget != "" && m.terminalStreamSizeChanged() {
			cols, rows := m.terminalDimensions()
			if m.terminalStream != nil && m.terminalControllable {
				m.terminalStreamCols, m.terminalStreamRows = cols, rows
				if m.terminalScreen != nil {
					m.terminalScreen.Resize(cols, rows)
				}
				controller := m.terminalStream
				return m, func() tea.Msg { return terminalKeySent{err: controller.Resize(cols, rows)} }
			}
			return m, m.startTerminalStream()
		}
	case loaded:
		m.loading = false
		if x.err != nil {
			m.connected = false
			m.err = x.err.Error()
		} else {
			m.connected = true
			m.err = ""
			m.setLiveAgents(x.agents)
			m.keepCursorVisible()
		}
		if m.view == "chats" {
			tick := tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} })
			if m.terminalStream == nil || m.terminalTarget != m.currentTerminalTarget() || m.terminalControllable {
				return m, tea.Batch(tick, m.startTerminalStream())
			}
			return m, tick
		}
		if m.view == "detail" {
			if m.selected.Status == "OFFLINE" && m.terminalStream != nil {
				return m, tea.Batch(tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} }), m.stopTerminalStream())
			}
			if _, ok := m.client.(herdr.TerminalControllerClient); ok && m.terminalStream != nil && m.terminalTarget == m.selected.PaneID && m.terminalControllable {
				return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} })
			}
			if _, ok := m.client.(herdr.TerminalControllerClient); ok {
				return m, tea.Batch(tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} }), m.startTerminalStream())
			}
			return m, tea.Batch(tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} }), m.readSelected())
		}
		return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return refreshTick{} })
	case refreshTick:
		return m, m.refresh()
	case focused:
		if x.err != nil {
			m.err = x.err.Error()
		} else {
			m.status = "Focused Herdr pane"
		}
		return m, m.refresh()
	case terminalRead:
		if x.paneID != m.selected.PaneID || m.view != "detail" {
			return m, nil
		}
		if x.err != nil {
			m.err = x.err.Error()
		} else {
			m.terminalOutput = sanitizeTerminal(x.text)
			m.terminalLines = strings.Split(m.terminalOutput, "\n")
			m.terminalTop = max(0, len(m.terminalLines)-m.terminalViewport())
			m.outputFocus = false
			m.err = ""
		}
		return m, nil
	case previewRead:
		if m.chatPreviews == nil {
			m.chatPreviews = make(map[string]string)
		}
		if x.err != nil {
			m.chatPreviews[x.paneID] = "Read failed: " + x.err.Error()
		} else {
			m.chatPreviews[x.paneID] = sanitizeTerminal(x.text)
		}
		return m, nil
	case terminalStreamStarted:
		if x.generation != m.terminalGeneration || x.target != m.currentTerminalTarget() || (m.view != "chats" && m.view != "detail") {
			if x.observer != nil {
				x.observer.Close()
			}
			return m, nil
		}
		if x.err != nil {
			if m.terminalOpenCancel != nil {
				m.terminalOpenCancel()
				m.terminalOpenCancel = nil
			}
			m.terminalStreamErr = x.err.Error()
			return m, m.readFallbackTerminal()
		}
		m.terminalOpenCancel = nil
		m.terminalStream = x.observer
		m.terminalControllable = x.control
		return m, nextTerminalFrame(x.observer, x.target, x.generation)
	case terminalFrameReceived:
		if x.generation != m.terminalGeneration || x.target != m.terminalTarget {
			return m, nil
		}
		if x.err != nil {
			if x.err.Error() != "EOF" && x.err.Error() != "context canceled" {
				m.terminalStreamErr = x.err.Error()
			}
			m.terminalStream = nil
			m.terminalControllable = false
			return m, m.readFallbackTerminal()
		}
		if x.frame.Err != nil {
			m.terminalStreamErr = x.frame.Err.Error()
			m.terminalStream = nil
			m.terminalControllable = false
			return m, m.readFallbackTerminal()
		}
		if x.frame.Closed {
			m.terminalStream = nil
			m.terminalControllable = false
			m.terminalStreamErr = "Herdr terminal session closed"
			return m, nil
		}
		if m.terminalScreen == nil {
			m.terminalScreen = vt.NewSafeEmulator(m.terminalStreamCols, m.terminalStreamRows)
			m.terminalScreen.SetScrollbackSize(10000)
		}
		_, _ = m.terminalScreen.Write(x.frame.Data)
		return m, nextTerminalFrame(m.terminalStream, x.target, x.generation)
	case terminalKeySent:
		m.terminalSending = false
		if x.err != nil {
			m.err = "Herdr could not send input: " + x.err.Error()
			return m, m.sendNextTerminalInput()
		}
		m.err = ""
		return m, m.sendNextTerminalInput()
	case agentStarted:
		m.startLaunching = false
		if x.err != nil {
			m.startConfirm = false
			if uncertainStart(x.err) {
				m.view = "dashboard"
				m.status = "Herdr may have started the agent. Check the list before retrying."
				return m, m.refresh()
			}
			m.err = "Herdr could not start agent: " + x.err.Error()
		} else {
			m.startConfirm = false
			m.view = "dashboard"
			m.status = "Agent started in Herdr"
			if x.profileID != "" {
				if err := m.rememberStartedInstance(x); err != nil {
					m.status = "Agent started, but template avatar could not be saved: " + err.Error()
				}
			}
			if x.promptErr != nil {
				m.status = "Agent started, but identity instructions were not delivered: " + x.promptErr.Error()
			}
			return m, m.refresh()
		}
		return m, nil
	case paneCreated:
		m.startPaneCreating = false
		if x.err == nil && x.pane.ID == "" {
			x.err = fmt.Errorf("Herdr created a tab without a pane ID")
		}
		if x.pane.ID != "" && x.pane.ID != m.invocation.PluginPaneID {
			m.startPanes = append(m.startPanes, x.pane)
			m.startPaneCursor = len(m.startPanes) - 1
		}
		if x.err != nil {
			m.err = x.err.Error()
			return m, nil
		}
		if m.view == "editor" {
			m.edit[9].SetValue(x.pane.ID)
			m.editorChoosingPane = false
		}
		if m.view == "starter" {
			m.startInputs[2].SetValue(x.pane.ID)
			m.startChoosingPane = false
		}
		m.err = ""
		m.status = "New Herdr shell tab ready"
		return m, nil
	case panesLoaded:
		m.startPaneLoading = false
		if x.workspaceErr == nil {
			m.workspaces = x.workspaces
			m.workspaceCursor = min(m.workspaceCursor, max(0, len(m.workspaces)-1))
		}
		if x.err != nil {
			m.startPanes = nil
			m.startPaneCursor = 0
			m.err = "Could not list Herdr panes: " + x.err.Error()
		} else {
			m.startPanes = m.startPanes[:0]
			for _, pane := range x.panes {
				if pane.ID != "" && pane.ID != m.invocation.PluginPaneID {
					m.startPanes = append(m.startPanes, pane)
				}
			}
			if m.view == "editor" {
				sort.SliceStable(m.startPanes, func(i, j int) bool {
					return m.paneHasLiveAgent(m.startPanes[i]) && !m.paneHasLiveAgent(m.startPanes[j])
				})
			}
			m.err = ""
			if x.workspaceErr != nil && m.choosingWorkspace {
				m.err = "Could not list Herdr workspaces: " + x.workspaceErr.Error()
			}
			if len(m.startPanes) > 0 {
				m.startPaneCursor = 0
				if m.view == "editor" {
					for i, pane := range m.startPanes {
						if m.paneHasLiveAgent(pane) && m.editorPaneAvailable(pane) {
							m.startPaneCursor = i
							break
						}
					}
				} else {
					for i, pane := range m.startPanes {
						if pane.ShellAvailable && !m.profilePaneBusy(pane.ID) {
							m.startPaneCursor = i
							break
						}
					}
				}
			}
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.confirm {
			if strings.ToLower(x.String()) == "y" {
				cmd := m.deleteSelected()
				return m, cmd
			}
			m.confirm = false
			return m, nil
		}
		if m.view == "chats" {
			cmd := m.chatKey(x)
			return m, cmd
		}
		if m.view == "starter" {
			return m, m.starterKey(x)
		}
		if m.view == "editor" {
			return m, m.editorKey(x)
		}
		if m.view == "search" {
			switch strings.ToLower(x.String()) {
			case "esc":
				m.view = "dashboard"
				m.query = ""
				m.cursor, m.listOffset = 0, 0
			case "enter":
				m.view = "dashboard"
			case "backspace":
				query := []rune(m.query)
				if len(query) > 0 {
					m.query = string(query[:len(query)-1])
				}
				m.cursor, m.listOffset = 0, 0
			default:
				if x.Text != "" {
					m.query += x.Text
					m.cursor, m.listOffset = 0, 0
				}
			}
			return m, nil
		}
		if m.view == "detail" {
			if strings.ToLower(x.String()) == "ctrl+g" {
				m.view = firstNonempty(m.chatOrigin, "dashboard")
				if m.view == "chats" {
					return m, m.startTerminalStream()
				}
				return m, m.stopTerminalStream()
			}
			if m.terminalStream != nil {
				if m.terminalScreen != nil && !m.terminalScreen.IsAltScreen() && m.terminalScreen.ScrollbackLen() > 0 {
					switch strings.ToLower(x.String()) {
					case "pgup", "pageup":
						m.scrollTerminalScreen(m.terminalViewport())
						return m, nil
					case "pgdown", "pagedown":
						m.scrollTerminalScreen(-m.terminalViewport())
						return m, nil
					case "home":
						m.scrollTerminalScreen(m.terminalScreen.ScrollbackLen())
						return m, nil
					case "end":
						m.terminalScrollOffset = 0
						return m, nil
					}
				}
				input, ok := terminalInput(x)
				if !ok {
					m.err = "This key cannot be sent to the Herdr terminal"
					return m, nil
				}
				return m, m.queueTerminalInput(pendingTerminalInput{data: input})
			}
			key, ok := terminalKey(x)
			if !ok {
				m.err = "Connecting to Herdr terminal…"
				return m, nil
			}
			_, ok = m.client.(herdr.KeySender)
			if !ok {
				m.err = "Herdr terminal controller is unavailable"
				return m, nil
			}
			return m, m.queueTerminalInput(pendingTerminalInput{key: key})
		}
		switch strings.ToLower(x.String()) {
		case "ctrl+c", "q":
			return m, m.quitWithTerminalCleanup()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.keepCursorVisible()
		case "down", "j":
			if m.cursor < len(m.filtered())-1 {
				m.cursor++
			}
			m.keepCursorVisible()
		case "pgup", "pageup":
			m.cursor = max(0, m.cursor-m.visibleAgents())
			m.keepCursorVisible()
		case "pgdown", "pagedown":
			if count := len(m.filtered()); count > 0 {
				m.cursor = min(count-1, m.cursor+m.visibleAgents())
			}
			m.keepCursorVisible()
		case "home":
			m.cursor = 0
			m.keepCursorVisible()
		case "end":
			m.cursor = max(0, len(m.filtered())-1)
			m.keepCursorVisible()
		case "r":
			return m, m.refresh()
		case "c":
			m.view = "chats"
			m.chatCursor = 0
			return m, m.startTerminalStream()
		case "?":
			m.view = "help"
		case "esc":
			if m.view != "dashboard" {
				m.view = "dashboard"
				m.confirm = false
			} else {
				m.query = ""
			}
		case "/":
			m.view = "search"
		case "n":
			m.startEditor(false)
			return m, m.loadStartPanes()
		case "a":
			m.startAgent()
			return m, m.loadStartPanes()
		case "e":
			if len(m.filtered()) > 0 {
				m.startEditor(true)
				return m, m.loadStartPanes()
			}
		case "d":
			if len(m.filtered()) > 0 {
				m.beginDelete(m.filtered()[m.cursor])
				return m, nil
			}
		case "f":
			m.toggleFavorite()
		case "enter":
			if len(m.filtered()) > 0 {
				cmd := m.openChat(m.filtered()[m.cursor], "dashboard")
				return m, cmd
			}
		}
		return m, nil
	}
	if m.view == "editor" {
		for i := range m.edit {
			var cmd tea.Cmd
			m.edit[i], cmd = m.edit[i].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

type refreshTick struct{}

func (m Model) profileAgents() []agents.Agent {
	out := []agents.Agent{}
	for _, p := range m.config.Profiles {
		id := p.ID
		if p.PaneID != "" {
			id = p.PaneID
		}
		out = append(out, agents.Agent{ID: id, Name: p.Name, Provider: p.Provider, WorkspaceID: p.WorkspaceID, PaneID: p.PaneID, Icon: p.Icon, Color: p.Color, Description: p.Description, Favorite: p.Favorite, Custom: true, Status: "OFFLINE"})
	}
	return out
}

func (m *Model) setLiveAgents(live []agents.Agent) {
	m.liveAgents = append([]agents.Agent(nil), live...)
	for i := range m.liveAgents {
		for _, instance := range m.config.Instances {
			if instance.PaneID != m.liveAgents[i].PaneID || instance.AgentName != m.liveAgents[i].Name {
				continue
			}
			name, icon, color, description, provider := instance.Name, instance.Icon, instance.Color, instance.Description, instance.Provider
			for _, profile := range m.config.Profiles {
				if profile.ID == instance.ProfileID {
					name, icon, color, description = profile.Name, profile.Icon, profile.Color, profile.Description
					if profile.Provider != "" {
						provider = profile.Provider
					}
					break
				}
			}
			m.liveAgents[i].Name = name
			m.liveAgents[i].Icon = icon
			m.liveAgents[i].Color = color
			m.liveAgents[i].Description = description
			m.liveAgents[i].Custom = true
			if m.liveAgents[i].Provider == "" {
				m.liveAgents[i].Provider = provider
			}
			break
		}
	}
	m.agents = agents.Merge(m.liveAgents, m.profileAgents())
	favorites := make(map[string]bool, len(m.config.Favorites))
	for _, id := range m.config.Favorites {
		favorites[id] = true
	}
	for i := range m.agents {
		m.agents[i].Favorite = favorites[m.favoriteRef(m.agents[i].ID)] || favorites[m.agents[i].ID]
	}
	sort.SliceStable(m.agents, func(i, j int) bool {
		iTemplate := isProfileTemplate(m.agents[i])
		jTemplate := isProfileTemplate(m.agents[j])
		if iTemplate != jTemplate {
			return !iTemplate
		}
		return m.agents[i].Favorite && !m.agents[j].Favorite
	})
	if m.view == "detail" {
		for _, a := range m.agents {
			if a.ID == m.selected.ID && a.PaneID == m.selected.PaneID {
				m.selected = a
				break
			}
		}
	}
}

func (m Model) favoriteRef(id string) string {
	scope := m.invocation.ServerScope
	if scope == "" {
		scope = "local"
	}
	return scope + "|" + id
}

func (m Model) favoriteCount() int {
	count := 0
	prefix := m.favoriteRef("")
	for _, id := range m.config.Favorites {
		if strings.HasPrefix(id, prefix) || !strings.Contains(id, "|") {
			count++
		}
	}
	return count
}

func (m *Model) keepCursorVisible() {
	count := len(m.filtered())
	if count == 0 {
		m.cursor, m.listOffset = 0, 0
		return
	}
	if m.cursor >= count {
		m.cursor = count - 1
	}
	visible := m.visibleAgents()
	if m.cursor < m.listOffset {
		m.listOffset = m.cursor
	}
	if m.cursor >= m.listOffset+visible {
		m.listOffset = m.cursor - visible + 1
	}
	maxOffset := max(0, count-visible)
	if m.listOffset > maxOffset {
		m.listOffset = maxOffset
	}
}

func (m Model) visibleAgents() int {
	rowHeight := 5
	reserved := 12
	if m.width >= 68 && m.height >= 26 && m.favoriteCount() > 0 {
		reserved += 9
	}
	return max(1, (m.height-reserved)/rowHeight)
}

func (m Model) filtered() []agents.Agent {
	out := []agents.Agent{}
	q := strings.ToLower(strings.TrimSpace(m.query))
	for _, a := range m.agents {
		searchable := strings.Join([]string{a.Name, a.Provider, a.CWD, a.WorkspaceName, a.WorkspaceID, a.Activity, a.Description, a.ID, a.PaneID}, " ")
		if q == "" || strings.Contains(strings.ToLower(searchable), q) {
			out = append(out, a)
		}
	}
	return out
}

func isProfileTemplate(agent agents.Agent) bool {
	return agent.Custom && agent.Status == "OFFLINE"
}
func (m *Model) toggleFavorite() {
	a := m.filtered()
	if len(a) == 0 {
		return
	}
	target := a[m.cursor]
	favorite := !target.Favorite
	filteredID := target.ID
	ref := m.favoriteRef(filteredID)
	previousFavorites := append([]string(nil), m.config.Favorites...)
	next := m.config.Favorites[:0]
	for _, id := range m.config.Favorites {
		if id != filteredID && id != ref {
			next = append(next, id)
		}
	}
	m.config.Favorites = next
	if favorite {
		m.config.Favorites = append(m.config.Favorites, ref)
	}
	for i := range m.config.Profiles {
		p := &m.config.Profiles[i]
		if p.ID == target.ID || (p.PaneID != "" && p.PaneID == target.PaneID) {
			p.Favorite = favorite
		}
	}
	m.config.Version = storage.SchemaVersion
	if err := storage.Save(m.cfgDir, m.config); err != nil {
		m.config.Favorites = previousFavorites
		for i := range m.config.Profiles {
			p := &m.config.Profiles[i]
			if p.ID == target.ID || (p.PaneID != "" && p.PaneID == target.PaneID) {
				p.Favorite = target.Favorite
			}
		}
		m.setLiveAgents(m.liveAgents)
		m.err = err.Error()
		return
	}
	m.setLiveAgents(m.liveAgents)
	for i, agent := range m.filtered() {
		if agent.ID == filteredID {
			m.cursor = i
			break
		}
	}
	m.keepCursorVisible()
	m.status = "Favorite saved"
}
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	if m.view == "detail" && m.terminalControllable && m.terminalScreen != nil && m.terminalScrollOffset == 0 {
		position := m.terminalScreen.CursorPosition()
		view.Cursor = tea.NewCursor(position.X, position.Y+4)
	}
	if m.view == "detail" || m.view == "chats" {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
}

func (m Model) render() string {
	if m.width == 0 {
		return "Loading Agent Hub…"
	}
	if m.view == "help" {
		return m.help()
	}
	if m.view == "editor" {
		if m.choosingWorkspace {
			return m.workspacePickerView()
		}
		return m.editorView()
	}
	if m.view == "starter" {
		if m.choosingWorkspace {
			return m.workspacePickerView()
		}
		return m.starterView()
	}
	if m.view == "detail" {
		return m.detailView()
	}
	if m.view == "chats" {
		return m.chatsView()
	}
	rows := m.filtered()
	connection := lipgloss.NewStyle().Foreground(theme.Blue).Render("● CONNECTING")
	if !m.loading && (m.connected || m.err == "") {
		connection = lipgloss.NewStyle().Foreground(theme.Green).Render("● HERDR ONLINE")
	}
	if m.err != "" && !m.connected {
		connection = lipgloss.NewStyle().Foreground(theme.Red).Render("● HERDR OFFLINE")
	}
	innerWidth := max(20, m.width-6)
	title := lipgloss.NewStyle().Bold(true).Foreground(theme.White).Render("◈ AGENT HUB") + "   " + connection
	searchNew := lipgloss.NewStyle().Foreground(theme.Muted).Render("/ SEARCH   N NEW")
	if m.view == "search" {
		queryWidth := max(8, innerWidth-lipgloss.Width(title)-18)
		searchNew = lipgloss.NewStyle().Foreground(theme.Blue).Render("/ " + truncate(firstNonempty(m.query, "SEARCH…"), queryWidth) + "  Esc clear")
	}
	if innerWidth >= 52 {
		gap := innerWidth - lipgloss.Width(title) - lipgloss.Width(searchNew) - 2
		if gap >= 1 {
			title += strings.Repeat(" ", gap) + searchNew
		} else if m.view != "search" {
			title += "   / SEARCH   N NEW"
		}
	}
	head := lipgloss.NewStyle().Padding(0, 1).Width(innerWidth).Background(theme.Panel).Render(title)
	favoriteCount := m.favoriteCount()
	body := []string{head}
	if m.confirm {
		body = append(body, "", lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true).Render("Delete local profile ‘"+truncate(displayText(m.selected.Name), max(8, innerWidth-36))+"’? Y confirm · Esc cancel"),
			lipgloss.NewStyle().Foreground(theme.Muted).Render("Existing Herdr session and pane stay open."))
	}
	if m.width >= 68 && m.height >= 26 {
		favorites := make([]agents.Agent, 0, 3)
		for _, a := range m.agents {
			if a.Favorite {
				favorites = append(favorites, a)
			}
			if len(favorites) == 3 {
				break
			}
		}
		if len(favorites) > 0 {
			body = append(body, "", sectionTitle("FEATURED AGENTS", fmt.Sprintf("[%d]", favoriteCount), innerWidth))
			body = append(body, m.favoriteCards(favorites, innerWidth)...)
		}
	} else if favoriteCount > 0 {
		body = append(body, lipgloss.NewStyle().Foreground(theme.Purple).Render(fmt.Sprintf("★ FAVORITES [%d]", favoriteCount)))
	}
	if m.err != "" {
		body = append(body, "", lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, innerWidth)))
	}
	sessionCount, templateCount := 0, 0
	for _, agent := range rows {
		if isProfileTemplate(agent) {
			templateCount++
		} else {
			sessionCount++
		}
	}
	if len(rows) == 0 {
		if m.view == "search" && m.query != "" {
			body = append(body, "", lipgloss.NewStyle().Foreground(theme.Muted).Render("No agents match “"+truncate(m.query, max(8, innerWidth-22))+"”"))
		} else {
			body = append(body, "", sectionTitle("AGENT SESSIONS", "[0]", innerWidth), "", "  No live agents detected. Press A to start a profile.")
		}
	} else {
		limit := m.visibleAgents()
		if limit < 1 {
			limit = 1
		}
		start := min(m.listOffset, max(0, len(rows)-1))
		end := min(len(rows), start+limit)
		if start < sessionCount {
			body = append(body, "", sectionTitle("AGENT SESSIONS", fmt.Sprintf("[%d]", sessionCount), innerWidth), "")
		}
		if start >= sessionCount && templateCount > 0 {
			body = append(body, "", sectionTitle("AGENT TEMPLATES", fmt.Sprintf("[%d]", templateCount), innerWidth), "")
		}
		for i := start; i < end; i++ {
			if i == sessionCount && templateCount > 0 {
				body = append(body, "", sectionTitle("AGENT TEMPLATES", fmt.Sprintf("[%d]", templateCount), innerWidth), "")
			}
			a := rows[i]
			fallback := ""
			if a.Custom {
				fallback = firstNonempty(a.Icon, "◆")
			}
			avatar := components.Avatar(a.Provider, a.Color, fallback)
			name := truncate(displayText(a.Name), 20)
			if a.Favorite {
				name = lipgloss.NewStyle().Foreground(theme.Yellow).Render("★") + " " + name
			}
			prefix := "  "
			if i == m.cursor {
				prefix = "› "
			}
			status := a.Status
			stateColor := statusColor(a.Status)
			if isProfileTemplate(a) {
				status = "TEMPLATE"
				stateColor = theme.Purple
			}
			line := fmt.Sprintf("%s%s  %-20s  %-11s %s", prefix, avatar[0], name, truncate(displayText(a.Provider), 11), lipgloss.NewStyle().Foreground(stateColor).Render(status))
			if i == m.cursor {
				body = append(body, lipgloss.NewStyle().Bold(true).Foreground(theme.White).Background(theme.Panel).Render(line))
			} else {
				body = append(body, line)
			}
			workspace := firstNonempty(a.WorkspaceName, a.WorkspaceID, "workspace pending")
			meta := truncate(displayText(a.Provider+" · "+workspace+" · "+firstNonempty(a.Activity, a.CWD)), max(8, innerWidth-8))
			if isProfileTemplate(a) {
				meta = truncate(displayText(a.Provider+" · saved profile · "+firstNonempty(a.Description, a.CWD)), max(8, innerWidth-8))
			}
			metaLine := "  " + avatar[1] + "  " + lipgloss.NewStyle().Foreground(theme.Muted).Render(meta)
			avatarLine2 := "  " + avatar[2]
			avatarLine3 := "  " + avatar[3]
			if i == m.cursor {
				selectedStyle := lipgloss.NewStyle().Foreground(theme.White).Background(theme.Panel)
				metaLine = selectedStyle.Render(metaLine)
				avatarLine2 = selectedStyle.Render(avatarLine2)
				avatarLine3 = selectedStyle.Render(avatarLine3)
			}
			body = append(body, metaLine, avatarLine2, avatarLine3, "")
		}
	}
	if m.status != "" {
		body = append(body, lipgloss.NewStyle().Foreground(theme.Green).Render(m.status))
	}
	footerText := "↑↓/jk navigate  Enter chat  C chats  A start  N new  E edit  D delete  F favorite  / search  R refresh  ? help  Q quit"
	if m.view == "search" {
		footerText = "Type to search name, provider, workspace, activity  Backspace edit  Enter done  Esc clear"
	}
	footer := lipgloss.NewStyle().Foreground(theme.Muted).Render(footerText)
	body = append(body, footer)
	return lipgloss.NewStyle().Padding(1, 2).Width(m.width).Height(m.height).Render(strings.Join(body, "\n"))
}

func sectionTitle(label, count string, width int) string {
	style := lipgloss.NewStyle().Bold(true).Foreground(theme.Green)
	muted := lipgloss.NewStyle().Foreground(theme.Muted)
	left := style.Render("› " + label)
	right := muted.Render(count)
	fill := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return left + " " + muted.Render(strings.Repeat("─", fill)) + " " + right
}

func (m Model) favoriteCards(favorites []agents.Agent, width int) []string {
	columns := min(3, len(favorites))
	cardWidth := max(18, (width-(columns-1)*2)/columns)
	cards := make([]string, 0, columns)
	for _, a := range favorites[:columns] {
		icon := components.Avatar(a.Provider, a.Color, a.Icon)
		label := lipgloss.NewStyle().Foreground(theme.Purple).Bold(true).Render("★ " + truncate(displayText(a.Name), max(8, cardWidth-5)))
		state := lipgloss.NewStyle().Foreground(statusColor(a.Status)).Render("[ " + a.Status + " ]")
		content := append(append([]string{}, icon...), label, state)
		cards = append(cards, lipgloss.NewStyle().Width(cardWidth-2).Height(6).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(theme.Purple).Align(lipgloss.Center).Render(strings.Join(content, "\n")))
	}
	return []string{lipgloss.JoinHorizontal(lipgloss.Top, cards...)}
}
func (m Model) detailView() string {
	a := m.selected
	maxWidth := max(12, m.width-6)
	lines := []string{
		truncate("◈ AGENT HUB / "+displayText(a.Name), maxWidth),
		lipgloss.NewStyle().Foreground(statusColor(a.Status)).Render(a.Status) + "  " + truncate(displayText(a.Provider+" · "+firstNonempty(a.WorkspaceName, a.WorkspaceID)+" · "+a.PaneID), maxWidth-12),
		"",
		lipgloss.NewStyle().Foreground(theme.Muted).Render("LIVE HERDR TERMINAL · type directly to interact"),
	}
	if terminal := m.terminalScreenFor(a.PaneID); terminal != "" {
		lines = append(lines, strings.Split(terminal, "\n")...)
	} else if m.terminalStreamErr != "" && m.terminalTarget == a.PaneID {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Render("Live terminal unavailable: "+m.terminalStreamErr))
		lines = append(lines, terminalExcerpt(m.terminalOutput, maxWidth, m.terminalViewport())...)
	} else if m.terminalOutput != "" {
		top := min(m.terminalTop, max(0, len(m.terminalLines)-m.terminalViewport()))
		end := min(len(m.terminalLines), top+m.terminalViewport())
		for _, line := range m.terminalLines[top:end] {
			lines = append(lines, truncate(line, maxWidth))
		}
	} else {
		lines = append(lines, "Loading or no terminal output available")
	}
	lines = append(lines, "", "Wheel / PgUp scroll · PgDn / End latest · Type to interact · Ctrl+G back")
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Red).Render(m.err))
	}
	return strings.Join(lines, "\n")
}
func sanitizeTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
}
func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "—"
}
func statusColor(status string) color.Color {
	switch status {
	case "IDLE", "DONE":
		return theme.Green
	case "WORKING":
		return theme.Blue
	case "WAITING":
		return theme.Yellow
	case "OFFLINE":
		return theme.Red
	default:
		return theme.Muted
	}
}
func validColor(s string) bool { return regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(s) }
func (m Model) help() string {
	return "Agent Hub keys\n\n↑/↓ or j/k  navigate agent list\nC            open chat inbox with live Herdr terminal\nEnter        open selected agent terminal\nType         interact directly with selected agent\nCtrl+G       return to chat list or dashboard\nA            start a Herdr-supported agent\nN / E        create / edit local profile\nD            delete profile after confirmation\nF            persist favorite on dashboard\n/            search\nR            refresh from Herdr\nQ            quit"
}
func (m *Model) startEditor(edit bool) {
	m.view = "editor"
	m.err = ""
	m.editAdvanced = false
	m.editorChoosingPane = false
	m.editorChoosingProvider = false
	m.editProviderCursor = 0
	launch := m.launchWorkspace()
	m.startWorkspaceID = launch.WorkspaceID
	m.startWorkdir = launch.CWD
	m.editField = 0
	m.editingID = ""
	m.edit = make([]textinput.Model, 11)
	labels := []string{"Agent name", "Description", "Avatar", "Color (#RRGGBB)", "Provider", "Executable", "Working directory", "Arguments (JSON array)", "Workspace ID", "Pane ID", "Identity instructions"}
	for i, l := range labels {
		t := textinput.New()
		t.Placeholder = l
		t.Prompt = l + ": "
		t.CharLimit = 100
		if i == 10 {
			t.CharLimit = 4000
		}
		if edit {
			a := m.filtered()[m.cursor]
			var profile *storage.Profile
			for _, p := range m.config.Profiles {
				if p.ID == a.ID || (p.PaneID != "" && p.PaneID == a.PaneID) {
					m.editingID = p.ID
					profile = &p
					break
				}
			}
			switch i {
			case 0:
				t.SetValue(a.Name)
			case 1:
				if profile != nil {
					t.SetValue(profile.Description)
				}
			case 2:
				if profile != nil {
					t.SetValue(profile.Icon)
				}
			case 3:
				if profile != nil {
					t.SetValue(profile.Color)
				}
			case 4:
				t.SetValue(a.Provider)
			case 5:
				if profile != nil {
					t.SetValue(profile.Executable)
				}
			case 6:
				if profile != nil {
					t.SetValue(profile.Workdir)
				}
			case 7:
				if profile != nil {
					b, _ := json.Marshal(profile.Args)
					t.SetValue(string(b))
				}
			case 8:
				t.SetValue("")
			case 9:
				t.SetValue(a.PaneID)
			case 10:
				if profile != nil {
					t.SetValue(profile.Instructions)
				}
			}
		} else {
			switch i {
			case 2:
				t.SetValue(agentIconChoices[0].id)
			case 3:
				t.SetValue(agentIconChoices[0].color)
			case 6:
				t.SetValue("")
			case 4:
				t.SetValue(m.invocation.FocusedPaneAgent)
			case 9:
				t.SetValue("")
			}
		}
		if i == 0 {
			t.Focus()
		}
		m.edit[i] = t
	}
}
func (m *Model) editorKey(k tea.KeyPressMsg) tea.Cmd {
	if m.choosingWorkspace {
		return m.workspaceKey(k)
	}
	if m.editorChoosingProvider {
		choices := editorProviders()
		switch strings.ToLower(k.String()) {
		case "esc":
			m.editorChoosingProvider = false
		case "up", "k":
			m.editProviderCursor = max(0, m.editProviderCursor-1)
		case "down", "j":
			m.editProviderCursor = min(len(choices)-1, m.editProviderCursor+1)
		case "enter":
			m.edit[4].SetValue(choices[m.editProviderCursor])
			m.editorChoosingProvider = false
			m.err = ""
		}
		return nil
	}
	if m.editorChoosingPane {
		if m.startPaneCreating {
			return nil
		}
		switch strings.ToLower(k.String()) {
		case "esc":
			m.editorChoosingPane = false
		case "up", "k":
			if m.startPaneCursor > 0 {
				m.startPaneCursor--
			}
		case "down", "j":
			if m.startPaneCursor < len(m.startPanes)-1 {
				m.startPaneCursor++
			}
		case "n":
			m.openWorkspacePicker(true)
			return nil
		case "r":
			return m.loadStartPanes()
		case "enter":
			if len(m.startPanes) == 0 {
				m.err = "No Herdr panes found"
				return nil
			}
			pane := m.startPanes[m.startPaneCursor]
			if !m.editorPaneAvailable(pane) {
				m.err = "Cannot link this pane: Agent Hub pane or already used by another profile"
				return nil
			}
			m.edit[9].SetValue(pane.ID)
			if pane.CWD != "" && (m.editingID == "" || m.edit[6].Value() == "") {
				m.edit[6].SetValue(pane.CWD)
			}
			for _, live := range m.liveAgents {
				if live.PaneID == pane.ID {
					if strings.TrimSpace(m.edit[0].Value()) == "" {
						m.edit[0].SetValue(live.Name)
					}
					if m.editingID == "" && (m.edit[4].Value() == "" || m.edit[4].Value() == m.invocation.FocusedPaneAgent) {
						m.edit[4].SetValue(live.Provider)
					}
					if m.editingID == "" && (m.edit[6].Value() == "" || m.edit[6].Value() == m.invocation.FocusedPaneCWD || m.edit[6].Value() == m.invocation.WorkspaceCWD) {
						m.edit[6].SetValue(live.CWD)
					}
					break
				}
			}
			m.editorChoosingPane = false
			m.err = ""
		}
		return nil
	}
	if k.String() == "esc" {
		m.view = "dashboard"
		return nil
	}
	if k.String() == "ctrl+a" {
		m.editAdvanced = !m.editAdvanced
		if !m.editAdvanced && (m.editField == 5 || m.editField == 7) {
			m.edit[m.editField].Blur()
			m.editField = 6
			return m.edit[m.editField].Focus()
		}
		return nil
	}
	if k.String() == "ctrl+k" {
		m.editorChoosingProvider = true
		m.editProviderCursor = 0
		for i, provider := range editorProviders() {
			if provider == m.edit[4].Value() {
				m.editProviderCursor = i
				break
			}
		}
		return nil
	}
	if k.String() == "ctrl+s" {
		return m.saveEditor()
	}
	if m.editField == 2 {
		key := strings.ToLower(k.String())
		current := m.edit[2].Value()
		index := iconChoiceIndex(current)
		switch key {
		case "up", "left", "k":
			if index < 0 {
				index = 0
			} else {
				index = (index - 1 + len(agentIconChoices)) % len(agentIconChoices)
			}
			m.selectIcon(index)
			return nil
		case "down", "right", "j":
			index = (index + 1) % len(agentIconChoices)
			m.selectIcon(index)
			return nil
		default:
			if len(key) == 1 && key[0] >= '1' && key[0] <= '8' {
				index = int(key[0] - '1')
				m.selectIcon(index)
				return nil
			}
			if len(key) == 1 || k.Text != "" || key == "backspace" || key == "delete" {
				return nil
			}
		}
	}
	if k.String() == "ctrl+p" {
		m.editorChoosingPane = true
		m.startPaneCursor = 0
		for i, pane := range m.startPanes {
			if pane.ID == m.edit[9].Value() {
				m.startPaneCursor = i
				break
			}
		}
		return nil
	}
	if k.String() == "shift+tab" && m.editField > 0 {
		return m.moveEditorField(-1)
	}
	if k.String() == "tab" || k.String() == "enter" {
		if m.editField == 10 {
			if k.String() == "enter" {
				return m.saveEditor()
			}
			return nil
		}
		return m.moveEditorField(1)
	}
	var cmd tea.Cmd
	m.edit[m.editField], cmd = m.edit[m.editField].Update(k)
	return cmd
}
func (m Model) editorView() string {
	if m.editorChoosingPane {
		return m.editorAgentPickerView()
	}
	if m.editorChoosingProvider {
		return m.providerPickerView()
	}
	width := max(28, m.width-4)
	title := "CREATE AGENT PROFILE"
	if m.editingID != "" {
		title = "EDIT AGENT PROFILE"
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.Green).Render("◈ AGENT HUB / " + title),
		lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("Reusable agent template. Choose Herdr workspace when starting a session.", width)),
	}
	if m.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, width)))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Purple).Bold(true).Render("TEMPLATE IDENTITY"))
	for _, i := range m.editorFields() {
		if i == 5 || i == 6 || i == 7 || i == 9 {
			if (i == 5 && m.editAdvanced) || (i == 6 && !m.editAdvanced) {
				lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Blue).Bold(true).Render("HERDR SETUP"))
			}
			if i == 7 && m.editAdvanced {
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render("Advanced launch metadata; no command runs when saving."))
			}
		}
		marker := "  "
		if i == m.editField {
			marker = lipgloss.NewStyle().Foreground(theme.Green).Render("› ")
		}
		switch i {
		case 2:
			choice := "Custom"
			if index := iconChoiceIndex(m.edit[2].Value()); index >= 0 {
				choice = agentIconChoices[index].name
			}
			lines = append(lines, marker+"Avatar: "+choice)
			if m.editField == 2 {
				lines = append(lines, m.iconPalette()...)
			}
		case 4:
			lines = append(lines, marker+truncate(m.edit[i].View()+"  [Ctrl+K choose]", width-2))
		case 9:
			label := "None · choose with Ctrl+P"
			for _, pane := range m.startPanes {
				if pane.ID == m.edit[9].Value() {
					label = readablePaneLabel(pane)
					break
				}
			}
			if m.edit[9].Value() != "" && label == "None · choose with Ctrl+P" {
				label = "Saved pane unavailable · Ctrl+P to choose"
			}
			lines = append(lines, marker+truncate("Herdr pane: "+label, width-2))
		default:
			lines = append(lines, marker+truncate(m.edit[i].View(), width-2))
		}
	}
	if m.startPaneLoading {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render("Checking Herdr panes…"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("Tab/Enter next · Shift+Tab back · Ctrl+S save · Ctrl+P link session · Ctrl+K provider · Ctrl+A advanced · Esc cancel", width)))
	return lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(strings.Join(lines, "\n"))
}

func iconChoiceIndex(id string) int {
	for i, choice := range agentIconChoices {
		if choice.id == id {
			return i
		}
	}
	return -1
}

func (m *Model) selectIcon(index int) {
	if index < 0 || index >= len(agentIconChoices) {
		return
	}
	choice := agentIconChoices[index]
	m.edit[2].SetValue(choice.id)
	m.edit[3].SetValue(choice.color)
}

func (m Model) iconPalette() []string {
	selected := m.edit[2].Value()
	lines := []string{lipgloss.NewStyle().Foreground(theme.Muted).Render("  Pick a pixel avatar · ↑↓/jk or 1–8")}
	if m.height >= 25 {
		avatar := components.Avatar("custom", m.edit[3].Value(), selected)
		lines = append(lines, avatar...)
	}
	var options []string
	for i, choice := range agentIconChoices {
		label := fmt.Sprintf("%d %s", i+1, choice.name)
		if choice.id == selected {
			label = lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("[" + label + "]")
		} else {
			label = lipgloss.NewStyle().Foreground(lipgloss.Color(choice.color)).Render(label)
		}
		options = append(options, label)
		if len(options) == 4 || i == len(agentIconChoices)-1 {
			lines = append(lines, "  "+strings.Join(options, "  "))
			options = nil
		}
	}
	return lines
}
func (m *Model) startAgent() {
	m.view = "starter"
	m.err = ""
	m.status = ""
	m.startField = 0
	m.startConfirm = false
	m.startLaunching = false
	m.startPaneCreating = false
	launch := m.launchWorkspace()
	m.startWorkspaceID = launch.WorkspaceID
	m.startWorkdir = launch.CWD
	m.startSelecting = len(m.config.Profiles) > 0
	m.startProfileCursor = 0
	m.startProfileName = ""
	m.startProfileID = ""
	m.startInstructions = ""
	m.startInputs = make([]textinput.Model, 5)
	labels := []string{"Agent name (lowercase)", "Provider / kind", "Selected Herdr pane", "Arguments (JSON string array)", "Identity instructions (sent as first prompt)"}
	for i, label := range labels {
		input := textinput.New()
		input.Prompt = label + ": "
		input.Placeholder = label
		input.CharLimit = 256
		if i == 3 {
			input.SetValue("[]")
		}
		if i == 4 {
			input.CharLimit = 4000
		}
		if i == 0 {
			input.Focus()
		}
		m.startInputs[i] = input
	}
}

func (m Model) launchWorkspace() storage.LaunchWorkspace {
	workspace, err := storage.LoadLaunchWorkspace(m.invocation.StateDir, m.invocation.SocketPath)
	if err != nil {
		return storage.LaunchWorkspace{}
	}
	return workspace
}

func (m *Model) loadStartPanes() tea.Cmd {
	lister, ok := m.client.(herdr.PaneLister)
	if !ok {
		return nil
	}
	m.startPaneLoading = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		panes, err := lister.Panes(ctx)
		var workspaces []herdr.Workspace
		var workspaceErr error
		if workspaceLister, ok := m.client.(herdr.WorkspaceLister); ok {
			workspaces, workspaceErr = workspaceLister.Workspaces(ctx)
		}
		return panesLoaded{panes: panes, workspaces: workspaces, err: err, workspaceErr: workspaceErr}
	}
}

func (m *Model) openWorkspacePicker(create bool) {
	m.choosingWorkspace = true
	m.workspaceCreate = create
	m.workspaceCursor = 0
	wanted := m.startWorkspaceID
	if m.view == "editor" && len(m.edit) > 8 {
		wanted = m.edit[8].Value()
	}
	for i, workspace := range m.workspaces {
		if workspace.ID == wanted {
			m.workspaceCursor = i
			break
		}
	}
	m.err = ""
}

func (m *Model) workspaceKey(key tea.KeyPressMsg) tea.Cmd {
	switch strings.ToLower(key.String()) {
	case "esc":
		m.choosingWorkspace = false
		m.workspaceCreate = false
	case "up", "k":
		if m.workspaceCursor > 0 {
			m.workspaceCursor--
		}
	case "down", "j":
		if m.workspaceCursor < len(m.workspaces)-1 {
			m.workspaceCursor++
		}
	case "r":
		return m.loadStartPanes()
	case "enter":
		if len(m.workspaces) == 0 {
			m.err = "No Herdr workspaces found; press R to refresh"
			return nil
		}
		workspace := m.workspaces[m.workspaceCursor]
		m.startWorkspaceID, m.startWorkdir = workspace.ID, workspace.CWD
		if m.view == "editor" {
			m.edit[8].SetValue(workspace.ID)
			m.edit[6].SetValue(workspace.CWD)
		}
		m.choosingWorkspace = false
		m.err = ""
		if m.workspaceCreate {
			m.workspaceCreate = false
			return m.createStartPane()
		}
	}
	return nil
}

func (m Model) workspacePickerView() string {
	width := max(32, m.width-4)
	if m.width == 0 {
		width = 76
	}
	lines := []string{
		lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render("CHOOSE HERDR WORKSPACE"),
		"", "New shell tab will open in selected workspace and its directory.", "",
	}
	for i, workspace := range m.workspaces {
		marker := "  "
		if i == m.workspaceCursor {
			marker = "› "
		}
		lines = append(lines, truncate(marker+firstNonempty(workspace.Label, workspace.ID)+"  ·  "+firstNonempty(workspace.CWD, "directory unavailable"), width))
	}
	if len(m.workspaces) == 0 {
		lines = append(lines, "No workspaces available from Herdr snapshot.")
	}
	lines = append(lines, "", "↑↓/jk select · Enter use workspace · R refresh · Esc back")
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, width)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) workspaceLabel(id string) string {
	for _, workspace := range m.workspaces {
		if workspace.ID == id {
			return firstNonempty(workspace.Label, workspace.ID) + " · " + firstNonempty(workspace.CWD, "directory unavailable")
		}
	}
	return id
}

func (m *Model) createStartPane() tea.Cmd {
	creator, ok := m.client.(herdr.PaneCreator)
	if !ok {
		m.err = "This Herdr client cannot create tabs"
		return nil
	}
	m.startPaneCreating = true
	m.err = ""
	workspaceID, cwd := m.startWorkspaceID, m.startWorkdir
	label := "Agent Hub"
	if m.view == "editor" && len(m.edit) > 8 {
		workspaceID = strings.TrimSpace(m.edit[8].Value())
		cwd = strings.TrimSpace(m.edit[6].Value())
	}
	if strings.TrimSpace(workspaceID) == "" {
		m.err = "Choose a Herdr workspace before creating a tab"
		m.startPaneCreating = false
		return nil
	}
	if m.view == "starter" && len(m.startInputs) > 0 {
		if name := strings.TrimSpace(m.startInputs[0].Value()); name != "" {
			label = "Agent · " + name
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		pane, err := creator.CreateShellTab(ctx, workspaceID, cwd, label)
		return paneCreated{pane, err}
	}
}

func readablePaneLabel(pane herdr.Pane) string {
	name := "Pane"
	if pane.Number > 0 {
		name = fmt.Sprintf("Pane %d", pane.Number)
	}
	if pane.Location != "" {
		name += " · " + pane.Location
	}
	if pane.Title != "" {
		name += " · " + pane.Title
	}
	context := []string{}
	if pane.Workspace != "" {
		context = append(context, pane.Workspace)
	}
	if pane.Tab != "" {
		context = append(context, pane.Tab)
	}
	if pane.CWD != "" {
		context = append(context, pane.CWD)
	}
	if pane.Focused {
		context = append(context, "focused")
	}
	if len(context) > 0 {
		name += " · " + strings.Join(context, " · ")
	}
	return name
}

func (m Model) panePickerView() string {
	width := m.width
	if width < 32 {
		width = 100
	}
	width -= 4
	title := "CHOOSE AGENT PANE"
	subtitle := "+ N  Create a fresh shell tab"
	hint := "  Choose workspace and folder before tab creation."
	if m.view == "editor" {
		title = "LINK PROFILE TO HERDR PANE"
		subtitle = "+ N  Create an empty shell tab for a future agent"
		hint = "  Choose a live agent to name it, or a ready shell to start later."
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.White).Render(title),
		"",
		lipgloss.NewStyle().Foreground(theme.Green).Bold(true).Render(truncate(subtitle, width)),
		lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate(hint, width)),
		"",
	}
	if m.startPaneCreating {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render("● Creating shell tab…"), "")
	}
	if m.startPaneLoading {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render("● Checking Herdr panes…"), "")
	}
	if len(m.startPanes) == 0 && !m.startPaneLoading {
		lines = append(lines, "No existing panes available. Press N to create one.")
	}
	limit, offset := len(m.startPanes), 0
	if m.height > 0 {
		limit = max(1, (m.height-12)/3)
		if len(m.startPanes) > limit {
			offset = min(max(0, m.startPaneCursor-limit+1), len(m.startPanes)-limit)
		}
	}
	for i := offset; i < min(len(m.startPanes), offset+limit); i++ {
		pane := m.startPanes[i]
		marker := "  "
		if i == m.startPaneCursor {
			marker = "› "
		}
		state := "UNKNOWN"
		stateColor := theme.Yellow
		if pane.ProcessError != "" {
			state = "CHECK FAILED"
			stateColor = theme.Red
		}
		if pane.ProcessKnown {
			state = "BUSY"
			stateColor = theme.Muted
		}
		if pane.ShellAvailable {
			state = "READY"
			stateColor = theme.Green
		}
		if m.profilePaneBusy(pane.ID) || pane.Agent != "" {
			state = "AGENT ACTIVE"
			stateColor = theme.Yellow
		}
		if m.view == "editor" {
			if !m.editorPaneAvailable(pane) {
				state = "UNAVAILABLE"
				stateColor = theme.Red
			} else if m.paneHasLiveAgent(pane) {
				state = "LIVE AGENT"
				stateColor = theme.Purple
			} else {
				state = "READY SHELL"
				stateColor = theme.Green
			}
		}
		name := fmt.Sprintf("Pane %d", pane.Number)
		if pane.Number == 0 {
			name = "Pane"
		}
		if pane.Location != "" {
			name += " · " + pane.Location
		}
		if pane.Title != "" {
			name += " · " + truncate(displayText(pane.Title), 26)
		}
		if m.view == "editor" {
			if agentName := m.editorPaneAgentName(pane.ID); agentName != "" {
				name += " · " + truncate(displayText(agentName), 24)
			}
		}
		if pane.Focused {
			name += " · focused"
		}
		stateText := lipgloss.NewStyle().Foreground(stateColor).Bold(true).Render("[" + state + "]")
		lines = append(lines, marker+truncate(name, max(20, width-19))+"  "+stateText)
		context := firstNonempty(pane.Workspace, "workspace") + " / " + firstNonempty(pane.Tab, "tab")
		if pane.CWD != "" {
			context += " · " + pane.CWD
		}
		process := pane.ProcessName
		if process == "" {
			process = "process unavailable"
		}
		lines = append(lines, "  "+truncate(displayText(context), width), "  "+lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("Foreground: "+displayText(process), width)), "")
	}
	if len(m.startPanes) > limit {
		lines = append(lines, fmt.Sprintf("Showing %d–%d of %d panes", offset+1, min(offset+limit, len(m.startPanes)), len(m.startPanes)))
	}
	if len(m.startPanes) > 0 && m.startPaneCursor < len(m.startPanes) {
		if reason := m.startPanes[m.startPaneCursor].ProcessError; reason != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Yellow).Render("Process check: "+truncate(displayText(reason), width)))
		}
	}
	footer := "N new tab → workspace · ↑↓/jk choose · Enter use READY · R refresh · Esc back"
	if m.view == "editor" {
		footer = "N new shell · ↑↓/jk choose · Enter link · R refresh · Esc back"
	}
	lines = append(lines, "", truncate(footer, width))
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(displayText(m.err), width)))
	}
	return strings.Join(lines, "\n")
}

func uncertainStart(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "agent_not_ready") || strings.Contains(message, "deadline exceeded") || strings.Contains(message, "timeout")
}

func (m *Model) starterKey(key tea.KeyPressMsg) tea.Cmd {
	pressed := strings.ToLower(key.String())
	if m.choosingWorkspace {
		return m.workspaceKey(key)
	}
	if m.startChoosingPane {
		if m.startPaneCreating {
			return nil
		}
		switch pressed {
		case "esc":
			m.startChoosingPane = false
		case "up", "k":
			if m.startPaneCursor > 0 {
				m.startPaneCursor--
			}
		case "down", "j":
			if m.startPaneCursor < len(m.startPanes)-1 {
				m.startPaneCursor++
			}
		case "n":
			m.openWorkspacePicker(true)
			return nil
		case "r":
			return m.loadStartPanes()
		case "enter":
			if len(m.startPanes) == 0 {
				m.err = "No Herdr panes found"
				return nil
			}
			pane := m.startPanes[m.startPaneCursor]
			if !m.paneLaunchable(pane) {
				m.err = "Pane unavailable: " + m.paneUnavailableReason(pane) + ". Press N for a fresh shell tab."
				return nil
			}
			m.startInputs[2].SetValue(pane.ID)
			m.startWorkspaceID, m.startWorkdir = pane.WorkspaceID, pane.CWD
			m.startChoosingPane = false
			m.err = ""
		}
		return nil
	}
	if m.startSelecting {
		switch pressed {
		case "esc":
			m.view = "dashboard"
			m.startSelecting = false
		case "up", "k":
			if m.startProfileCursor > 0 {
				m.startProfileCursor--
			}
		case "down", "j":
			if m.startProfileCursor < len(m.config.Profiles) {
				m.startProfileCursor++
			}
		case "enter":
			if m.startProfileCursor == len(m.config.Profiles) {
				m.startSelecting = false
				m.startProfileName = "Manual setup"
				return m.startInputs[0].Focus()
			}
			profile := m.config.Profiles[m.startProfileCursor]
			m.prefillStartProfile(profile)
			m.startSelecting = false
			m.err = ""
			return m.startInputs[0].Focus()
		}
		return nil
	}
	if pressed == "ctrl+w" {
		m.openWorkspacePicker(false)
		return nil
	}
	if m.startConfirm {
		if m.startLaunching {
			return nil
		}
		switch pressed {
		case "esc", "n":
			m.startConfirm = false
			m.view = "dashboard"
			return nil
		case "y", "enter":
			starter, ok := m.client.(herdr.Starter)
			if !ok {
				m.err = "This Herdr client cannot start agents"
				m.startConfirm = false
				m.view = "dashboard"
				return nil
			}
			name := m.startInputs[0].Value()
			kind := m.startInputs[1].Value()
			pane := m.startInputs[2].Value()
			var argv []string
			if err := json.Unmarshal([]byte(m.startInputs[3].Value()), &argv); err != nil {
				m.err = "Arguments must be a JSON string array"
				m.startConfirm = false
				return nil
			}
			if err := herdr.ValidateStart(name, kind, pane, argv); err != nil {
				m.err = err.Error()
				m.startConfirm = false
				return nil
			}
			if !m.selectedPaneReady(pane) {
				m.err = "Choose a pane marked SHELL READY with Ctrl+P"
				m.startConfirm = false
				return nil
			}
			m.startLaunching = true
			instructions := strings.TrimSpace(m.startInstructions)
			if len(m.startInputs) > 4 && strings.TrimSpace(m.startInputs[4].Value()) != "" {
				instructions = strings.TrimSpace(m.startInputs[4].Value())
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			profileID := m.startProfileID
			return func() tea.Msg {
				defer cancel()
				if err := starter.Start(ctx, name, kind, pane, argv); err != nil {
					return agentStarted{err: err, name: name, pane: pane, provider: kind, profileID: profileID}
				}
				if instructions == "" {
					return agentStarted{name: name, pane: pane, provider: kind, profileID: profileID}
				}
				prompter, ok := m.client.(herdr.Prompter)
				if !ok {
					return agentStarted{promptErr: fmt.Errorf("Herdr client does not support agent prompts"), name: name, pane: pane, provider: kind, profileID: profileID}
				}
				agent := agents.Agent{Name: name, PaneID: pane}
				prompt := "Use the following as your role and behavior for this session. Briefly acknowledge it, then wait for a task. These are identity instructions, not a request to perform unrelated work:\n\n" + instructions
				return agentStarted{promptErr: prompter.Prompt(ctx, agent, prompt), name: name, pane: pane, provider: kind, profileID: profileID}
			}
		default:
			return nil
		}
	}
	if pressed == "esc" {
		m.view = "dashboard"
		return nil
	}
	if pressed == "ctrl+p" && !m.startConfirm {
		m.startChoosingPane = true
		m.startPaneCursor = 0
		selected := m.startInputs[2].Value()
		for i, pane := range m.startPanes {
			if pane.ID == selected {
				m.startPaneCursor = i
				break
			}
		}
		return nil
	}
	if pressed == "shift+tab" && m.startField > 0 {
		m.startInputs[m.startField].Blur()
		m.startField--
		if m.startField == 2 {
			m.startField--
		}
		return m.startInputs[m.startField].Focus()
	}
	if pressed == "tab" || pressed == "enter" {
		if m.startField == 1 {
			m.startField = 3
			return m.startInputs[3].Focus()
		}
		if m.startField < len(m.startInputs)-1 {
			m.startInputs[m.startField].Blur()
			m.startField++
			return m.startInputs[m.startField].Focus()
		}
		var argv []string
		if err := json.Unmarshal([]byte(m.startInputs[3].Value()), &argv); err != nil {
			m.err = "Arguments must be a JSON string array"
			return nil
		}
		if err := herdr.ValidateStart(m.startInputs[0].Value(), m.startInputs[1].Value(), m.startInputs[2].Value(), argv); err != nil {
			m.err = err.Error()
			return nil
		}
		if !m.selectedPaneReady(m.startInputs[2].Value()) {
			m.err = "Choose a pane marked SHELL READY with Ctrl+P"
			return nil
		}
		m.startConfirm = true
		return nil
	}
	var cmd tea.Cmd
	m.startInputs[m.startField], cmd = m.startInputs[m.startField].Update(key)
	return cmd
}

func (m Model) profilePaneBusy(paneID string) bool {
	if paneID == "" {
		return false
	}
	for _, agent := range m.agents {
		if agent.PaneID == paneID && agent.Status != "OFFLINE" {
			return true
		}
	}
	return false
}

func (m *Model) prefillStartProfile(profile storage.Profile) {
	name := uniqueAgentName(agentNameSlug(profile.Name), m.agents)
	provider := profile.Provider
	if provider == "" {
		provider = m.invocation.FocusedPaneAgent
	}
	pane := ""
	args, _ := json.Marshal(profile.Args)
	if profile.Args == nil {
		args = []byte("[]")
	}
	m.startInputs[0].SetValue(name)
	m.startInputs[1].SetValue(provider)
	m.startInputs[2].SetValue(pane)
	m.startInputs[3].SetValue(string(args))
	m.startInputs[4].SetValue(profile.Instructions)
	m.startInstructions = profile.Instructions
	m.startProfileName = profile.Name
	m.startProfileID = profile.ID
}

func agentNameSlug(name string) string {
	var slug strings.Builder
	separator := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			slug.WriteRune(r)
			separator = false
		} else if (r == '_' || r == '-') && slug.Len() > 0 && !separator {
			slug.WriteRune(r)
			separator = true
		} else if slug.Len() > 0 && !separator {
			slug.WriteByte('-')
			separator = true
		}
	}
	value := strings.Trim(slug.String(), "-_")
	if value == "" {
		value = "agent"
	}
	if value[0] < 'a' || value[0] > 'z' {
		value = "agent-" + value
	}
	if len(value) > 32 {
		value = strings.TrimRight(value[:32], "-_")
	}
	return value
}

func uniqueAgentName(base string, live []agents.Agent) string {
	used := func(name string) bool {
		for _, agent := range live {
			if agent.Name == name {
				return true
			}
		}
		return false
	}
	if !used(base) {
		return base
	}
	for suffix := 2; ; suffix++ {
		tail := fmt.Sprintf("-%d", suffix)
		prefix := strings.TrimRight(base[:min(len(base), 32-len(tail))], "-_")
		candidate := prefix + tail
		if !used(candidate) {
			return candidate
		}
	}
}

func (m Model) starterView() string {
	if m.startChoosingPane {
		return m.panePickerView()
	}
	if m.startSelecting {
		width := max(28, m.width-4)
		if m.width == 0 {
			width = 80
		}
		lines := []string{
			lipgloss.NewStyle().Bold(true).Foreground(theme.Green).Render("◈ AGENT HUB / START AN AGENT"),
			lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("Choose a template; select workspace and shell pane next.", width)),
			"", sectionTitle("AGENT TEMPLATES", fmt.Sprintf("[%d]", len(m.config.Profiles)), width), "",
		}
		limit := len(m.config.Profiles) + 1
		if m.height > 0 {
			limit = max(1, (m.height-8)/5)
		}
		offset := min(max(0, m.startProfileCursor-limit+1), max(0, len(m.config.Profiles)+1-limit))
		for i := offset; i < min(len(m.config.Profiles)+1, offset+limit); i++ {
			marker := "  "
			if i == m.startProfileCursor {
				marker = lipgloss.NewStyle().Foreground(theme.Green).Render("› ")
			}
			if i == len(m.config.Profiles) {
				lines = append(lines, marker+lipgloss.NewStyle().Foreground(theme.Blue).Bold(i == m.startProfileCursor).Render("+ Start without template"),
					"  Configure provider and identity for this session.", "")
				continue
			}
			profile := m.config.Profiles[i]
			state, stateColor := "TEMPLATE", theme.Purple
			nextStep := "Choose workspace and shell pane when starting"
			icon := components.Avatar(profile.Provider, profile.Color, profile.Icon)
			name := truncate(displayText(profile.Name), max(8, width-32))
			badge := lipgloss.NewStyle().Foreground(stateColor).Render("[" + state + "]")
			lines = append(lines, marker+icon[0]+"  "+lipgloss.NewStyle().Bold(i == m.startProfileCursor).Render(name)+"  "+badge)
			meta := displayText(firstNonempty(profile.Provider, "provider unset"))
			if profile.Description != "" {
				meta += " · " + displayText(profile.Description)
			}
			lines = append(lines, "  "+icon[1]+"  "+truncate(meta, max(8, width-15)),
				"  "+icon[2]+"  "+lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate(nextStep, max(8, width-15))),
				"  "+icon[3], "")
		}
		lines = append(lines, fmt.Sprintf("%d/%d · ↑↓/jk select · Enter continue · Esc back", m.startProfileCursor+1, len(m.config.Profiles)+1))
		if m.err != "" {
			lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Red).Render(truncate(m.err, width)))
		}
		return lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(strings.Join(lines, "\n"))
	}
	title := "START AGENT IN HERDR"
	if m.startProfileName != "" {
		title += " · " + m.startProfileName
	}
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.White).Render(title),
		"",
		"Herdr starts the installed provider in a shell pane.",
		"Ctrl+P: choose pane or create a fresh shell tab with N. Ctrl+W: workspace.",
		"",
	}
	if m.startWorkspaceID != "" {
		lines = append(lines, "Workspace for new tab: "+displayText(m.workspaceLabel(m.startWorkspaceID)))
	} else {
		lines = append(lines, "Workspace for new tab: choose with Ctrl+W or when creating tab")
	}
	lines = append(lines, "")
	if m.status != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(theme.Blue).Render(m.status), "")
	}
	for i, input := range m.startInputs {
		if i == 2 {
			width := m.width
			if width < 32 {
				width = 100
			}
			label := "Choose a pane with Ctrl+P"
			context := ""
			found := false
			for _, pane := range m.startPanes {
				if pane.ID == input.Value() {
					found = true
					label = fmt.Sprintf("Pane %d", pane.Number)
					if pane.Location != "" {
						label += " · " + pane.Location
					}
					if pane.Title != "" {
						label += " · " + displayText(pane.Title)
					}
					if m.paneLaunchable(pane) {
						label += " · SHELL READY"
					} else {
						label += " · UNAVAILABLE"
					}
					context = firstNonempty(pane.Workspace, "workspace") + " / " + firstNonempty(pane.Tab, "tab")
					if pane.CWD != "" {
						context += " · " + pane.CWD
					}
					break
				}
			}
			if input.Value() == "" {
				label = "No pane selected · Ctrl+P to choose or create"
			} else if !found {
				label = "Saved pane unavailable · Ctrl+P to choose or create"
			}
			lines = append(lines, truncate("  Herdr pane: "+label, width-2))
			if context != "" {
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render(truncate("    "+displayText(context), width-2)))
			}
			continue
		}
		if i == m.startField && !m.startConfirm {
			lines = append(lines, input.View())
		} else {
			lines = append(lines, "  "+input.View())
		}
	}
	if m.startConfirm {
		if m.startLaunching {
			lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Blue).Render("● Starting agent in Herdr… This can take up to 30 seconds."))
		} else {
			if len(m.startInputs) > 4 && strings.TrimSpace(m.startInputs[4].Value()) != "" {
				lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Purple).Render("First prompt · session identity:"), truncate("  "+displayText(m.startInputs[4].Value()), max(12, m.width-4)))
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.Muted).Render("This is a normal first prompt, not a provider system prompt; it is not guaranteed to persist across sessions."))
			}
			lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Yellow).Render("Start in selected pane? Y or Enter confirm · Esc cancel"))
		}
	} else {
		lines = append(lines, "", "Tab/Enter: next · Shift+Tab: previous · Ctrl+P: pane · Ctrl+W: workspace · Esc: cancel")
	}
	if m.err != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(theme.Red).Render(m.err))
	}
	return strings.Join(lines, "\n")
}

func (m Model) paneLabel(id string) string {
	for _, pane := range m.startPanes {
		if pane.ID == id {
			label := fmt.Sprintf("Pane %d", pane.Number)
			if pane.Location != "" {
				label += " · " + pane.Location
			}
			if pane.Tab != "" {
				label += " · " + pane.Tab
			}
			return label
		}
	}
	return ""
}

func (m Model) selectedPaneReady(id string) bool {
	for _, pane := range m.startPanes {
		if pane.ID == id {
			return m.paneLaunchable(pane)
		}
	}
	return false
}

func (m Model) paneLaunchable(pane herdr.Pane) bool {
	return pane.ShellAvailable && pane.Agent == "" && !m.profilePaneBusy(pane.ID) && pane.ID != m.invocation.PluginPaneID
}

func (m Model) paneUnavailableReason(pane herdr.Pane) string {
	if pane.ID == m.invocation.PluginPaneID {
		return "Agent Hub is running here"
	}
	if pane.Agent != "" || m.profilePaneBusy(pane.ID) {
		return "agent already active"
	}
	if pane.ProcessError != "" {
		return "process check failed"
	}
	if pane.ProcessKnown {
		return "foreground process is " + firstNonempty(pane.ProcessName, "not a shell")
	}
	return "Herdr could not confirm an idle shell"
}

func (m Model) terminalViewport() int {
	return max(1, m.height-10)
}

func (m *Model) scrollTerminal(delta int) {
	maxTop := max(0, len(m.terminalLines)-m.terminalViewport())
	m.terminalTop = min(maxTop, max(0, m.terminalTop+delta))
}

func (m *Model) scrollTerminalScreen(delta int) {
	if m.terminalScreen == nil || m.terminalScreen.IsAltScreen() {
		return
	}
	maxOffset := m.terminalScreen.ScrollbackLen()
	m.terminalScrollOffset = min(maxOffset, max(0, m.terminalScrollOffset+delta))
}

func (m Model) hasLocalProfile(a agents.Agent) bool {
	for _, p := range m.config.Profiles {
		if p.ID == a.ID || (p.PaneID != "" && p.PaneID == a.PaneID) {
			return true
		}
	}
	return false
}

func (m *Model) beginDelete(a agents.Agent) {
	m.selected = a
	if !m.hasLocalProfile(a) {
		m.confirm = false
		m.err = "This is a live Herdr agent with no local profile to delete. Its pane remains in Herdr."
		return
	}
	m.err = ""
	m.confirm = true
}

func (m *Model) deleteSelected() tea.Cmd {
	for i, p := range m.config.Profiles {
		if p.ID != m.selected.ID && (p.PaneID == "" || p.PaneID != m.selected.PaneID) {
			continue
		}
		next := m.config
		next.Profiles = append(append([]storage.Profile(nil), m.config.Profiles[:i]...), m.config.Profiles[i+1:]...)
		// Keep favorites for a still-live pane; remove orphaned local favorites.
		live := false
		for _, a := range m.liveAgents {
			if a.ID == m.selected.ID || (p.PaneID != "" && a.PaneID == p.PaneID) {
				live = true
				break
			}
		}
		if !live {
			id := p.ID
			if p.PaneID != "" {
				id = p.PaneID
			}
			next.Favorites = nil
			for _, favorite := range m.config.Favorites {
				if favorite != id && favorite != m.favoriteRef(id) {
					next.Favorites = append(next.Favorites, favorite)
				}
			}
		}
		if err := storage.Save(m.cfgDir, next); err != nil {
			m.err = "Could not delete local profile: " + err.Error()
			m.confirm = false
			return nil
		}
		m.config = next
		m.setLiveAgents(m.liveAgents)
		m.keepCursorVisible()
		m.confirm = false
		m.err = ""
		m.status = "Local profile deleted; Herdr session unchanged"
		return nil
	}
	m.confirm = false
	m.err = "No local profile is attached to this Herdr agent"
	return nil
}
func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func displayText(s string) string {
	var clean strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			if r == '\n' || r == '\r' || r == '\t' {
				clean.WriteByte(' ')
			}
			continue
		}
		clean.WriteRune(r)
	}
	return strings.Join(strings.Fields(clean.String()), " ")
}
