package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tapiaw38/herdr-agent-hub/internal/herdr"
	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui"
)

func Run() error {
	cfg := os.Getenv("HERDR_PLUGIN_CONFIG_DIR")
	if cfg == "" {
		base, e := os.UserConfigDir()
		if e != nil {
			return e
		}
		cfg = filepath.Join(base, "herdr", "plugins", "community.herdr-agent-hub")
	}
	state := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	previousHubPaneID := ""
	if state != "" {
		previousHubPaneID, _ = storage.LoadHubPane(state, os.Getenv("HERDR_SOCKET_PATH"))
		if e := os.MkdirAll(state, 0700); e != nil {
			return e
		}
		if e := storage.SaveHubPane(state, os.Getenv("HERDR_SOCKET_PATH"), os.Getenv("HERDR_PANE_ID")); e != nil {
			return fmt.Errorf("save Agent Hub pane: %w", e)
		}
	}
	c, e := storage.Load(cfg)
	if e != nil {
		return fmt.Errorf("load config: %w", e)
	}
	var invocation ui.InvocationContext
	if rawContext := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); rawContext != "" {
		if err := json.Unmarshal([]byte(rawContext), &invocation); err != nil {
			return fmt.Errorf("parse Herdr plugin context: %w", err)
		}
	}
	if invocation.WorkspaceID == "" {
		invocation.WorkspaceID = os.Getenv("HERDR_WORKSPACE_ID")
	}
	if invocation.TabID == "" {
		invocation.TabID = os.Getenv("HERDR_TAB_ID")
	}
	invocation.PluginPaneID = os.Getenv("HERDR_PANE_ID")
	invocation.ServerScope = storage.ServerScope(os.Getenv("HERDR_SOCKET_PATH"))
	invocation.StateDir = state
	invocation.SocketPath = os.Getenv("HERDR_SOCKET_PATH")
	previousWorkspaceHubID, _ := storage.LoadWorkspaceHubPane(state, invocation.SocketPath, invocation.WorkspaceID)
	if state != "" && invocation.PluginPaneID != "" {
		if err := storage.SaveWorkspaceHubPane(state, invocation.SocketPath, invocation.WorkspaceID, invocation.PluginPaneID); err != nil {
			return fmt.Errorf("save workspace Agent Hub pane: %w", err)
		}
	}
	if state != "" && invocation.FocusedPaneID != "" && invocation.FocusedPaneID != invocation.PluginPaneID && invocation.FocusedPaneID != previousHubPaneID && invocation.FocusedPaneID != previousWorkspaceHubID {
		workspace, err := storage.LoadLaunchWorkspace(state, invocation.SocketPath)
		if err != nil || workspace.WorkspaceID != invocation.WorkspaceID {
			if err := storage.SaveLaunchWorkspace(state, invocation.SocketPath, storage.LaunchWorkspace{WorkspaceID: invocation.WorkspaceID, CWD: invocation.WorkspaceCWD}); err != nil {
				return fmt.Errorf("save launch workspace: %w", err)
			}
		}
	}
	if cwd := dashboardWorkingDir(invocation); cwd != "" {
		_ = os.Chdir(cwd)
	}
	if invocation.PluginPaneID != "" && os.Getenv("HERDR_ENV") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = herdr.NewCLI().RenamePane(ctx, invocation.PluginPaneID, "Agents")
		cancel()
	}
	p := tea.NewProgram(ui.NewWithContext(herdr.NewCLI(), c, cfg, invocation))
	_, e = p.Run()
	return e
}

func dashboardWorkingDir(invocation ui.InvocationContext) string {
	workspace, err := storage.LoadLaunchWorkspace(invocation.StateDir, invocation.SocketPath)
	if err == nil && workspace.WorkspaceID == invocation.WorkspaceID && workspace.CWD != "" {
		return workspace.CWD
	}
	if invocation.FocusedPaneCWD != "" && invocation.FocusedPaneID != invocation.PluginPaneID {
		return invocation.FocusedPaneCWD
	}
	return invocation.WorkspaceCWD
}

func FocusHub() error {
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir == "" {
		return fmt.Errorf("HERDR_PLUGIN_STATE_DIR is unavailable; open Agent Hub once first")
	}
	origin, err := readActionContext()
	if err != nil {
		return err
	}
	paneID, err := storage.LoadWorkspaceHubPane(stateDir, os.Getenv("HERDR_SOCKET_PATH"), origin.WorkspaceID)
	if err != nil {
		paneID, err = storage.LoadHubPane(stateDir, os.Getenv("HERDR_SOCKET_PATH"))
	}
	if err != nil {
		return fmt.Errorf("read Agent Hub pane: %w", err)
	}
	if err := saveActionWorkspace(stateDir, paneID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return herdr.NewCLI().FocusPluginPane(ctx, paneID)
}

// OpenHub opens the dashboard through Herdr's documented plugin pane CLI.
func OpenHub() error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	origin, err := readActionContext()
	if err != nil {
		return err
	}
	client := herdr.NewCLI()
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir != "" {
		paneID := ""
		if origin.WorkspaceID != "" {
			paneID, _ = storage.LoadWorkspaceHubPane(stateDir, os.Getenv("HERDR_SOCKET_PATH"), origin.WorkspaceID)
		} else {
			paneID, _ = storage.LoadHubPane(stateDir, os.Getenv("HERDR_SOCKET_PATH"))
		}
		if paneID != "" {
			if err := saveActionWorkspace(stateDir, paneID); err != nil {
				return err
			}
			if err := client.FocusPluginPane(ctx, paneID); err == nil {
				return nil
			}
		}
	}
	if err := saveActionWorkspace(stateDir, ""); err != nil {
		return err
	}
	return client.OpenPluginPane(ctx, origin.FocusedPaneID)
}

type actionContext struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceCWD   string `json:"workspace_cwd"`
	FocusedPaneID  string `json:"focused_pane_id"`
	FocusedPaneCWD string `json:"focused_pane_cwd"`
}

func readActionContext() (actionContext, error) {
	var context actionContext
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &context); err != nil {
			return actionContext{}, fmt.Errorf("parse Herdr action context: %w", err)
		}
	}
	return context, nil
}

func saveActionWorkspace(stateDir, hubPaneID string) error {
	if stateDir == "" {
		return nil
	}
	context, err := readActionContext()
	if err != nil {
		return err
	}
	if context.WorkspaceID == "" || context.FocusedPaneID == "" || context.FocusedPaneID == hubPaneID {
		return nil
	}
	return storage.SaveLaunchWorkspace(stateDir, os.Getenv("HERDR_SOCKET_PATH"), storage.LaunchWorkspace{WorkspaceID: context.WorkspaceID, CWD: context.WorkspaceCWD})
}
