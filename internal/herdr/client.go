package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
)

type HerdrClient interface {
	Snapshot(context.Context) ([]agents.Agent, error)
	Focus(context.Context, agents.Agent) error
}

// PaneLister is implemented by clients that can enumerate live Herdr panes.
type PaneLister interface {
	Panes(context.Context) ([]Pane, error)
}

type WorkspaceLister interface {
	Workspaces(context.Context) ([]Workspace, error)
}

type Workspace struct {
	ID, Label, CWD string
}

// PaneCreator creates a fresh Herdr tab with its root shell pane.
type PaneCreator interface {
	CreateShellTab(context.Context, string, string, string) (Pane, error)
}

// Pane contains only public session.snapshot fields needed to label a pane.
type Pane struct {
	ID, WorkspaceID, Workspace, Tab, CWD, Title, Agent, Status, Location string
	ProcessName                                                          string
	ProcessError                                                         string
	ShellAvailable, ProcessKnown                                         bool
	Number                                                               int
	Focused                                                              bool
}
type Client = HerdrClient
type Reader interface {
	Read(context.Context, agents.Agent) (string, error)
}
type Prompter interface {
	Prompt(context.Context, agents.Agent, string) error
}
type KeySender interface {
	SendKey(context.Context, agents.Agent, string) error
}
type Starter interface {
	Start(context.Context, string, string, string, []string) error
}
type CLI struct {
	Bin     string
	Timeout time.Duration
}

func NewCLI() CLI {
	b := os.Getenv("HERDR_BIN_PATH")
	if b == "" {
		b = "herdr"
	}
	return CLI{Bin: b, Timeout: 5 * time.Second}
}
func (c CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runTimeout(ctx, c.Timeout, args...)
}

func (c CLI) runTimeout(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	t := timeout
	if t <= 0 {
		t = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var response struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(errout.Bytes(), &response) == nil && response.Error.Code != "" {
			return nil, fmt.Errorf("Herdr %s: %s", response.Error.Code, response.Error.Message)
		}
		command := "command"
		if len(args) > 0 {
			command = args[0]
		}
		if len(args) > 1 {
			command += " " + args[1]
		}
		return nil, fmt.Errorf("herdr %s: %w: %s", command, err, strings.TrimSpace(errout.String()))
	}
	return out.Bytes(), nil
}

// Snapshot uses documented `herdr api snapshot`, a bootstrap read of session.snapshot.
func (c CLI) Snapshot(ctx context.Context) ([]agents.Agent, error) {
	b, err := c.run(ctx, "api", "snapshot")
	if err != nil {
		return nil, err
	}
	return ParseSnapshot(b)
}

func (c CLI) Workspaces(ctx context.Context) ([]Workspace, error) {
	b, err := c.run(ctx, "api", "snapshot")
	if err != nil {
		return nil, err
	}
	return ParseSnapshotWorkspaces(b)
}

func ParseSnapshotWorkspaces(b []byte) ([]Workspace, error) {
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("decode herdr snapshot: %w", err)
	}
	if r, ok := root["result"].(map[string]any); ok {
		root = r
	}
	if s, ok := root["snapshot"].(map[string]any); ok {
		root = s
	}
	rows, _ := root["workspaces"].([]any)
	out := make([]Workspace, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if id := str(m, "workspace_id", "id"); id != "" {
			out = append(out, Workspace{ID: id, Label: str(m, "label", "name"), CWD: str(m, "cwd")})
		}
	}
	return out, nil
}

// Panes reads the public session snapshot, including layout position labels.
func (c CLI) Panes(ctx context.Context) ([]Pane, error) {
	b, snapshotErr := c.run(ctx, "api", "snapshot")
	if snapshotErr == nil {
		panes, parseErr := ParseSnapshotPanes(b)
		if parseErr == nil && len(panes) > 0 {
			return c.inspectPanes(ctx, panes), nil
		}
		if parseErr == nil {
			snapshotErr = errors.New("Herdr snapshot returned no usable pane IDs")
		} else {
			snapshotErr = parseErr
		}
	}
	listOutput, listErr := c.run(ctx, "pane", "list")
	if listErr == nil {
		panes, parseErr := ParsePaneList(listOutput)
		if parseErr == nil {
			return c.inspectPanes(ctx, panes), nil
		}
		listErr = parseErr
	}
	return nil, fmt.Errorf("session snapshot: %v; pane list: %w", snapshotErr, listErr)
}

// inspectPanes uses the documented pane process-info command to determine
// whether each pane is sitting at its shell prompt. Unknown process state is
// never treated as launchable.
func (c CLI) inspectPanes(ctx context.Context, panes []Pane) []Pane {
	var group sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	for i := range panes {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				panes[index].ProcessError = ctx.Err().Error()
				return
			}
			b, err := c.runTimeout(ctx, 2*time.Second, "pane", "process-info", "--pane", panes[index].ID)
			if err != nil {
				panes[index].ProcessError = err.Error()
				return
			}
			name, shellAvailable, ok := ParsePaneProcessInfo(b)
			panes[index].ProcessName, panes[index].ShellAvailable, panes[index].ProcessKnown = name, shellAvailable, ok
			if !ok {
				panes[index].ProcessError = "Herdr returned incomplete process data"
			}
		}(i)
	}
	group.Wait()
	return panes
}

// CreateShellTab uses Herdr's documented tab create response and waits until
// the new root pane is at its interactive shell prompt.
func (c CLI) CreateShellTab(ctx context.Context, workspaceID, cwd, label string) (Pane, error) {
	args := []string{"tab", "create"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	args = append(args, "--no-focus")
	b, err := c.run(ctx, args...)
	if err != nil {
		return Pane{}, err
	}
	var root struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		return Pane{}, fmt.Errorf("decode new Herdr tab: %w", err)
	}
	id := root.Result.RootPane.PaneID
	if !validPaneID(id) {
		return Pane{}, errors.New("Herdr created tab without usable root pane ID")
	}
	pane := Pane{ID: id, WorkspaceID: workspaceID, Workspace: workspaceID, Tab: label, CWD: cwd, Title: label, Number: 1}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		info, checkErr := c.runTimeout(ctx, time.Second, "pane", "process-info", "--pane", id)
		if checkErr == nil {
			pane.ProcessName, pane.ShellAvailable, pane.ProcessKnown = ParsePaneProcessInfo(info)
			if pane.ShellAvailable {
				return pane, nil
			}
			pane.ProcessError = "New shell has not reached its prompt"
		} else {
			pane.ProcessError = checkErr.Error()
		}
		select {
		case <-ctx.Done():
			return pane, ctx.Err()
		case <-deadline.C:
			return pane, errors.New("new Herdr tab created, but shell is not ready; refresh pane list and try again")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// ParsePaneProcessInfo parses the documented PaneProcessInfo response. Herdr
// reports the shell pid, foreground process group, and foreground processes.
func ParsePaneProcessInfo(b []byte) (string, bool, bool) {
	var root map[string]any
	if json.Unmarshal(b, &root) != nil {
		return "", false, false
	}
	if result, ok := root["result"].(map[string]any); ok {
		root = result
	}
	if info, ok := root["process_info"].(map[string]any); ok {
		root = info
	}
	shellPID, shellOK := number(root["shell_pid"])
	fgID, fgOK := number(root["foreground_process_group_id"])
	processes, _ := root["foreground_processes"].([]any)
	name := "unknown process"
	if _, ok := root["foreground_processes"]; !ok || !shellOK || !fgOK {
		return name, false, false
	}
	if len(processes) > 0 {
		if p, ok := processes[0].(map[string]any); ok {
			if n, ok := p["name"].(string); ok && n != "" {
				name = n
			}
		}
	}
	if shellPID != fgID || len(processes) != 1 {
		return name, false, true
	}
	process, ok := processes[0].(map[string]any)
	if !ok {
		return name, false, true
	}
	processPID, ok := number(process["pid"])
	if !ok || processPID != shellPID {
		return name, false, true
	}
	return name, isShellProcess(name), true
}

func number(v any) (uint64, bool) {
	switch n := v.(type) {
	case float64:
		return uint64(n), n > 0
	case json.Number:
		x, err := n.Int64()
		return uint64(x), err == nil && x > 0
	default:
		return 0, false
	}
}

func isShellProcess(name string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Base(name), "-")) {
	case "sh", "bash", "zsh", "fish", "dash", "ksh", "tcsh", "csh", "nu", "nushell", "elvish", "pwsh", "powershell.exe":
		return true
	default:
		return false
	}
}

// ParseSnapshot accepts the JSON envelope emitted by `herdr api snapshot`.
func ParseSnapshot(b []byte) ([]agents.Agent, error) {
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("decode herdr snapshot: %w", err)
	}
	if r, ok := root["result"].(map[string]any); ok {
		root = r
	}
	if s, ok := root["snapshot"].(map[string]any); ok {
		root = s
	}
	workspaceNames := map[string]string{}
	if rows, ok := root["workspaces"].([]any); ok {
		for _, row := range rows {
			if m, ok := row.(map[string]any); ok {
				workspaceNames[str(m, "workspace_id", "id")] = str(m, "label", "name")
			}
		}
	}
	raw, ok := root["agents"].([]any)
	if !ok {
		raw = []any{}
	}
	out := []agents.Agent{}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		pane := str(m, "pane_id", "id")
		name := str(m, "name", "display_agent", "agent")
		provider := str(m, "kind", "agent", "provider")
		state := str(m, "status", "agent_status", "state")
		if state == "" {
			state = "unknown"
		}
		id := pane
		if id == "" {
			id = str(m, "agent_id", "id")
		}
		if pane == "" {
			pane = id
		}
		w := str(m, "workspace_id")
		out = append(out, agents.Agent{ID: id, PaneID: pane, WorkspaceID: w, WorkspaceName: workspaceNames[w], TabID: str(m, "tab_id"), Name: first(name, provider, pane), Provider: first(provider, "Agent"), Status: agents.MapStatus(state), CWD: str(m, "cwd", "foreground_cwd"), Activity: str(m, "message", "terminal_title_stripped", "terminal_title", "title")})
	}
	return out, nil
}

// ParseSnapshotPanes maps documented PaneInfo fields without reading terminal content.
func ParseSnapshotPanes(b []byte) ([]Pane, error) {
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("decode herdr snapshot: %w", err)
	}
	if r, ok := root["result"].(map[string]any); ok {
		root = r
	}
	if s, ok := root["snapshot"].(map[string]any); ok {
		root = s
	}
	workspaces, tabs := map[string]string{}, map[string]string{}
	if rows, ok := root["workspaces"].([]any); ok {
		for _, v := range rows {
			if m, ok := v.(map[string]any); ok {
				workspaces[str(m, "workspace_id", "id")] = str(m, "label", "name")
			}
		}
	}
	if rows, ok := root["tabs"].([]any); ok {
		for _, v := range rows {
			if m, ok := v.(map[string]any); ok {
				tabs[str(m, "tab_id", "id")] = str(m, "label", "name")
			}
		}
	}
	rows, _ := root["panes"].([]any)
	out := make([]Pane, 0, len(rows))
	locations := map[string]string{}
	if layouts, ok := root["layouts"].([]any); ok {
		for _, value := range layouts {
			layout, ok := value.(map[string]any)
			if !ok {
				continue
			}
			area, _ := layout["area"].(map[string]any)
			areaX, areaY := num(area["x"]), num(area["y"])
			areaW, areaH := num(area["width"]), num(area["height"])
			layoutPanes, _ := layout["panes"].([]any)
			for _, paneValue := range layoutPanes {
				pane, ok := paneValue.(map[string]any)
				if !ok {
					continue
				}
				rect, _ := pane["rect"].(map[string]any)
				if rect == nil {
					continue
				}
				cx := num(rect["x"]) + num(rect["width"])/2
				cy := num(rect["y"]) + num(rect["height"])/2
				rectW, rectH := num(rect["width"]), num(rect["height"])
				position := ""
				if areaW > 0 && rectW < areaW*0.8 {
					if cx < areaX+areaW/2 {
						position = "left"
					} else {
						position = "right"
					}
				}
				if areaH > 0 && rectH < areaH*0.8 {
					vertical := "top"
					if cy >= areaY+areaH/2 {
						vertical = "bottom"
					}
					if position != "" {
						position = vertical + " " + position
					} else {
						position = vertical
					}
				}
				if position != "" {
					locations[str(pane, "pane_id")] = position
				}
			}
		}
	}
	ordinals := map[string]int{}
	for _, v := range rows {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id := str(m, "pane_id")
		if !validPaneID(id) {
			continue
		}
		wid, tid := str(m, "workspace_id"), str(m, "tab_id")
		focused, _ := m["focused"].(bool)
		scope := wid + "/" + tid
		ordinals[scope]++
		out = append(out, Pane{ID: id, WorkspaceID: wid, Workspace: first(workspaces[wid], str(m, "workspace_label", "workspace_name"), wid), Tab: first(tabs[tid], str(m, "tab_label"), tid), CWD: first(str(m, "foreground_cwd"), str(m, "cwd")), Title: first(str(m, "label"), str(m, "title"), str(m, "terminal_title_stripped")), Agent: str(m, "display_agent", "agent"), Status: str(m, "agent_status"), Location: locations[id], Number: ordinals[scope], Focused: focused})
	}
	return out, nil
}

// ParsePaneList accepts the documented `herdr pane list` response envelope.
func ParsePaneList(b []byte) ([]Pane, error) { return ParseSnapshotPanes(b) }

func num(value any) float64 { n, _ := value.(float64); return n }
func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
func first(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return "Unknown"
}
func (c CLI) Focus(ctx context.Context, a agents.Agent) error {
	target := a.PaneID
	if !validPaneID(target) {
		return errors.New("agent has no valid live Herdr pane")
	}
	_, err := c.run(ctx, "agent", "focus", target)
	return err
}

func (c CLI) FocusPluginPane(ctx context.Context, paneID string) error {
	if !validPaneID(paneID) {
		return errors.New("invalid Agent Hub pane ID")
	}
	_, err := c.run(ctx, "plugin", "pane", "focus", paneID)
	return err
}

// RenamePane labels only the explicit Herdr pane supplied by its caller.
func (c CLI) RenamePane(ctx context.Context, paneID, label string) error {
	if !validPaneID(paneID) {
		return errors.New("invalid Herdr pane ID")
	}
	if strings.TrimSpace(label) == "" || strings.ContainsAny(label, "\x00\r\n") {
		return errors.New("invalid Herdr pane label")
	}
	_, err := c.run(ctx, "pane", "rename", paneID, label)
	return err
}

func (c CLI) OpenPluginPane(ctx context.Context, targetPaneID string) error {
	args := []string{"plugin", "pane", "open", "--plugin", "community.herdr-agent-hub", "--entrypoint", "dashboard", "--placement", "zoomed"}
	if targetPaneID != "" {
		if !validPaneID(targetPaneID) {
			return errors.New("invalid source Herdr pane ID")
		}
		args = append(args, "--target-pane", targetPaneID)
	}
	_, err := c.run(ctx, args...)
	return err
}

func (c CLI) Read(ctx context.Context, a agents.Agent) (string, error) {
	target := a.PaneID
	if !validPaneID(target) {
		return "", errors.New("agent has no valid live Herdr pane")
	}
	out, err := c.run(ctx, "agent", "read", target, "--source", "recent", "--lines", "50", "--format", "text")
	return string(out), err
}
func (c CLI) Prompt(ctx context.Context, a agents.Agent, text string) error {
	target := a.PaneID
	if !validPaneID(target) {
		return errors.New("agent has no valid live Herdr pane")
	}
	_, err := c.run(ctx, "agent", "prompt", target, text)
	return err
}

func (c CLI) SendKey(ctx context.Context, a agents.Agent, key string) error {
	target := a.PaneID
	if !validPaneID(target) {
		return errors.New("agent has no valid live Herdr pane")
	}
	if !validAgentKey(key) {
		return errors.New("unsupported interactive key")
	}
	_, err := c.run(ctx, "agent", "send-keys", target, key)
	return err
}

func validAgentKey(key string) bool {
	switch key {
	case "enter", "esc", "tab", "up", "down", "left", "right":
		return true
	}
	return len(key) == 1 && key[0] >= 0x20 && key[0] <= 0x7e
}

var SupportedKinds = []string{"pi", "claude", "codex", "gemini", "cursor", "devin", "agy", "cline", "omp", "mastracode", "opencode", "copilot", "kimi", "kiro", "droid", "amp", "grok", "hermes", "kilo", "qodercli", "qwen", "letta", "maki", "muse"}

func (c CLI) Start(ctx context.Context, name, kind, pane string, argv []string) error {
	if err := ValidateStart(name, kind, pane, argv); err != nil {
		return err
	}
	args := []string{"agent", "start", name, "--kind", kind, "--pane", pane}
	if len(argv) > 0 {
		args = append(args, "--")
		args = append(args, argv...)
	}
	_, err := c.runTimeout(ctx, 30*time.Second, args...)
	return err
}

func ValidateStart(name, kind, pane string, argv []string) error {
	if !validAgentName(name) {
		return errors.New("agent name must match [a-z][a-z0-9_-]{0,31}")
	}
	if !supportedKind(kind) {
		return fmt.Errorf("unsupported Herdr agent kind %q", kind)
	}
	if !validPaneID(pane) {
		return errors.New("pane ID must use Herdr format w<workspace>:p<pane>")
	}
	for _, arg := range argv {
		for _, r := range arg {
			if unicode.IsControl(r) {
				return errors.New("agent arguments cannot contain control characters")
			}
		}
	}
	return nil
}

func validPaneID(id string) bool {
	if !strings.HasPrefix(id, "w") {
		return false
	}
	parts := strings.SplitN(id[1:], ":p", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
				return false
			}
		}
	}
	return true
}

func validAgentName(name string) bool {
	if len(name) < 1 || len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func supportedKind(kind string) bool {
	for _, supported := range SupportedKinds {
		if kind == supported {
			return true
		}
	}
	return false
}

type Fake struct {
	Agents                                     []agents.Agent
	WorkspaceList                              []Workspace
	PaneList                                   []Pane
	CreatedPane                                Pane
	CreateErr                                  error
	CreatedWorkspace, CreatedCWD, CreatedLabel string
	Err                                        error
	Focused                                    string
	StartedName                                string
	StartedKind                                string
	StartedPane                                string
	StartedArgs                                []string
	PromptedPane                               string
	PromptedText                               string
	SentPane, SentKey                          string
}

func (f *Fake) Panes(context.Context) ([]Pane, error)           { return f.PaneList, f.Err }
func (f *Fake) Workspaces(context.Context) ([]Workspace, error) { return f.WorkspaceList, f.Err }
func (f *Fake) CreateShellTab(_ context.Context, workspaceID, cwd, label string) (Pane, error) {
	f.CreatedWorkspace, f.CreatedCWD, f.CreatedLabel = workspaceID, cwd, label
	return f.CreatedPane, f.CreateErr
}

func (f *Fake) Snapshot(context.Context) ([]agents.Agent, error) { return f.Agents, f.Err }
func (f *Fake) Focus(_ context.Context, a agents.Agent) error    { f.Focused = a.ID; return f.Err }
func (f *Fake) Prompt(_ context.Context, a agents.Agent, text string) error {
	f.PromptedPane, f.PromptedText = a.PaneID, text
	return f.Err
}
func (f *Fake) SendKey(_ context.Context, a agents.Agent, key string) error {
	if !validAgentKey(key) {
		return errors.New("unsupported interactive key")
	}
	f.SentPane, f.SentKey = a.PaneID, key
	return f.Err
}
func (f *Fake) Start(_ context.Context, name, kind, pane string, argv []string) error {
	f.StartedName, f.StartedKind, f.StartedPane, f.StartedArgs = name, kind, pane, append([]string(nil), argv...)
	return f.Err
}
