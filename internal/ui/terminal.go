package ui

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
)

func (m *Model) currentTerminalAgent() agents.Agent {
	if m.view == "detail" {
		return m.selected
	}
	list := m.liveChats()
	if len(list) == 0 {
		return agents.Agent{}
	}
	return list[min(m.chatCursor, len(list)-1)]
}

func (m *Model) currentTerminalTarget() string {
	return m.currentTerminalAgent().PaneID
}

func (m *Model) terminalDimensions() (int, int) {
	if m.view == "chats" && m.width >= 76 {
		width := max(24, m.width-4)
		bodyHeight := max(8, m.height-11)
		listWidth := min(36, max(24, width/4))
		return max(16, width-listWidth-13), max(4, bodyHeight-9)
	}
	return max(16, m.width-8), max(4, m.height-10)
}

func (m *Model) terminalStreamSizeChanged() bool {
	cols, rows := m.terminalDimensions()
	return cols != m.terminalStreamCols || rows != m.terminalStreamRows
}

func (m *Model) startTerminalStream() tea.Cmd {
	a := m.currentTerminalAgent()
	control := m.view == "detail"
	var open func(context.Context, string, int, int) (*herdr.TerminalStream, error)
	if control {
		if client, ok := m.client.(herdr.TerminalControllerClient); ok {
			open = client.ControlTerminal
		}
	} else if client, ok := m.client.(herdr.TerminalObserverClient); ok {
		open = client.ObserveTerminal
	}
	if open == nil || a.PaneID == "" {
		m.terminalTarget = a.PaneID
		m.terminalStreamErr = "Herdr terminal stream API unavailable"
		m.terminalControllable = false
		return m.readFallbackTerminal()
	}
	if m.terminalStream != nil && m.terminalTarget == a.PaneID && m.terminalControllable == control {
		cols, rows := m.terminalDimensions()
		if cols == m.terminalStreamCols && rows == m.terminalStreamRows {
			return nil
		}
		if control {
			m.terminalStreamCols, m.terminalStreamRows = cols, rows
			if m.terminalScreen != nil {
				m.terminalScreen.Resize(cols, rows)
			}
			controller := m.terminalStream
			return func() tea.Msg { return terminalKeySent{err: controller.Resize(cols, rows)} }
		}
	}
	if m.terminalOpenCancel != nil {
		m.terminalOpenCancel()
		m.terminalOpenCancel = nil
	}
	cols, rows := m.terminalDimensions()
	m.terminalGeneration++
	generation := m.terminalGeneration
	target := a.PaneID
	old := m.terminalStream
	m.terminalStream = nil
	m.terminalControllable = false
	m.terminalTarget = target
	m.terminalScrollOffset = 0
	m.terminalStreamCols, m.terminalStreamRows = cols, rows
	m.terminalStreamErr = ""
	m.terminalScreen = nil
	m.terminalScrollOffset = 0
	m.terminalSending = false
	m.terminalInputQueue = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.terminalOpenCancel = cancel
	return func() tea.Msg {
		if old != nil {
			old.Close()
		}
		defer cancel()
		stream, err := open(ctx, target, cols, rows)
		return terminalStreamStarted{target: target, generation: generation, observer: stream, control: control, err: err}
	}
}

func (m *Model) readFallbackTerminal() tea.Cmd {
	if m.view == "chats" {
		return m.readPreviews()
	}
	return m.readSelected()
}

func (m *Model) queueTerminalInput(input pendingTerminalInput) tea.Cmd {
	if len(input.data) == 0 && input.key == "" {
		return nil
	}
	if len(m.terminalInputQueue) >= 8192 {
		m.err = "Terminal input queue is full"
		return nil
	}
	m.terminalInputQueue = append(m.terminalInputQueue, input)
	return m.sendNextTerminalInput()
}

func (m *Model) sendTerminalBytes(input []byte) tea.Cmd {
	return m.queueTerminalInput(pendingTerminalInput{data: append([]byte(nil), input...)})
}

func (m *Model) sendNextTerminalInput() tea.Cmd {
	if m.terminalSending || len(m.terminalInputQueue) == 0 {
		return nil
	}
	input := m.terminalInputQueue[0]
	m.terminalInputQueue = m.terminalInputQueue[1:]
	m.terminalSending = true
	if m.terminalStream != nil {
		controller := m.terminalStream
		return func() tea.Msg {
			return terminalKeySent{paneID: m.selected.PaneID, err: controller.SendInput(input.data)}
		}
	}
	if input.key == "" {
		m.terminalSending = false
		m.err = "Connecting to Herdr terminal…"
		return m.sendNextTerminalInput()
	}
	sender, ok := m.client.(herdr.KeySender)
	if !ok {
		m.terminalSending = false
		m.err = "Herdr terminal controller is unavailable"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return func() tea.Msg {
		defer cancel()
		return terminalKeySent{paneID: m.selected.PaneID, err: sender.SendKey(ctx, m.selected, input.key)}
	}
}

func (m *Model) stopTerminalStream() tea.Cmd {
	observer := m.terminalStream
	m.terminalGeneration++
	m.terminalStream = nil
	m.terminalControllable = false
	m.terminalTarget = ""
	m.terminalInputQueue = nil
	if m.terminalOpenCancel != nil {
		m.terminalOpenCancel()
		m.terminalOpenCancel = nil
	}
	if observer == nil {
		return nil
	}
	return func() tea.Msg { observer.Close(); return nil }
}

func (m *Model) quitWithTerminalCleanup() tea.Cmd {
	observer := m.terminalStream
	if m.terminalOpenCancel != nil {
		m.terminalOpenCancel()
		m.terminalOpenCancel = nil
	}
	return func() tea.Msg {
		if observer != nil {
			observer.Close()
		}
		return tea.QuitMsg{}
	}
}

func (m Model) terminalScreenFor(target string) string {
	if target == "" || target != m.terminalTarget || m.terminalScreen == nil {
		return ""
	}
	if m.terminalScrollOffset <= 0 || m.terminalScreen.IsAltScreen() {
		return m.terminalScreen.Render()
	}
	width, height := m.terminalScreen.Width(), m.terminalScreen.Height()
	history := m.terminalScreen.ScrollbackLen()
	if width <= 0 || height <= 0 || history == 0 {
		return m.terminalScreen.Render()
	}
	start := max(0, history-m.terminalScrollOffset)
	viewport := vt.NewEmulator(width, height)
	for y := 0; y < height; y++ {
		line := start + y
		for x := 0; x < width; x++ {
			var cell *uv.Cell
			if line < history {
				cell = m.terminalScreen.ScrollbackCellAt(x, line)
			} else if screenRow := line - history; screenRow < height {
				cell = m.terminalScreen.CellAt(x, screenRow)
			}
			if cell != nil {
				viewport.SetCell(x, y, cell.Clone())
			}
		}
	}
	return viewport.Render()
}

func nextTerminalFrame(observer *herdr.TerminalStream, target string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		if observer == nil {
			return terminalFrameReceived{target: target, generation: generation, err: io.EOF}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		frame, err := observer.Next(ctx)
		return terminalFrameReceived{target: target, generation: generation, frame: frame, err: err}
	}
}
