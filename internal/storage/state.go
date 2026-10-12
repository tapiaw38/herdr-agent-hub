package storage

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type runtimeState struct {
	HubPaneID string `json:"hub_pane_id"`
}

type LaunchWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	CWD         string `json:"cwd"`
}

func SaveLaunchWorkspace(dir, socketPath string, workspace LaunchWorkspace) error {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(socketPath) == "" || strings.TrimSpace(workspace.WorkspaceID) == "" {
		return nil
	}
	if strings.ContainsAny(workspace.WorkspaceID+workspace.CWD, "\x00\r\n") {
		return errors.New("invalid launch workspace")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(workspace)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".launch-workspace-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0600); err != nil {
		return err
	}
	return os.Rename(name, launchWorkspacePath(dir, socketPath))
}

func LoadLaunchWorkspace(dir, socketPath string) (LaunchWorkspace, error) {
	if strings.TrimSpace(dir) == "" {
		return LaunchWorkspace{}, errors.New("Herdr plugin state directory is unavailable")
	}
	if strings.TrimSpace(socketPath) == "" {
		return LaunchWorkspace{}, errors.New("Herdr socket path is unavailable")
	}
	data, err := os.ReadFile(launchWorkspacePath(dir, socketPath))
	if err != nil {
		return LaunchWorkspace{}, err
	}
	var workspace LaunchWorkspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return LaunchWorkspace{}, fmt.Errorf("decode launch workspace: %w", err)
	}
	if strings.TrimSpace(workspace.WorkspaceID) == "" {
		return LaunchWorkspace{}, errors.New("launch workspace ID missing")
	}
	return workspace, nil
}

func launchWorkspacePath(dir, socketPath string) string {
	return filepath.Join(dir, "launch-workspace-"+serverKey(socketPath)+".json")
}

func SaveHubPane(dir, socketPath, paneID string) error {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(socketPath) == "" || strings.TrimSpace(paneID) == "" {
		return nil
	}
	if strings.ContainsAny(paneID, "\x00\r\n") {
		return errors.New("invalid Hub pane ID")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(runtimeState{HubPaneID: paneID})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".hub-state-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0600); err != nil {
		return err
	}
	return os.Rename(name, runtimePath(dir, socketPath))
}

func LoadHubPane(dir, socketPath string) (string, error) {
	if strings.TrimSpace(socketPath) == "" {
		return "", errors.New("Herdr socket path is unavailable")
	}
	data, err := os.ReadFile(runtimePath(dir, socketPath))
	if err != nil {
		return "", err
	}
	var state runtimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("decode Agent Hub runtime state: %w", err)
	}
	paneID := state.HubPaneID
	if strings.TrimSpace(paneID) == "" {
		return "", errors.New("Hub pane ID missing from runtime state")
	}
	return paneID, nil
}

// SaveWorkspaceHubPane remembers one dashboard pane per Herdr workspace.
func SaveWorkspaceHubPane(dir, socketPath, workspaceID, paneID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	if strings.ContainsAny(workspaceID, "\x00\r\n") {
		return errors.New("invalid workspace ID")
	}
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(socketPath) == "" || strings.TrimSpace(paneID) == "" {
		return nil
	}
	if strings.ContainsAny(paneID, "\x00\r\n") {
		return errors.New("invalid Hub pane ID")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(runtimeState{HubPaneID: paneID})
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".workspace-hub-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(name, 0600); err != nil {
		return err
	}
	return os.Rename(name, workspaceHubPath(dir, socketPath, workspaceID))
}

func LoadWorkspaceHubPane(dir, socketPath, workspaceID string) (string, error) {
	if strings.TrimSpace(socketPath) == "" || strings.TrimSpace(workspaceID) == "" {
		return "", errors.New("Herdr workspace or socket is unavailable")
	}
	data, err := os.ReadFile(workspaceHubPath(dir, socketPath, workspaceID))
	if err != nil {
		return "", err
	}
	var state runtimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("decode workspace Hub pane: %w", err)
	}
	if strings.TrimSpace(state.HubPaneID) == "" {
		return "", errors.New("workspace Hub pane ID missing")
	}
	return state.HubPaneID, nil
}

func workspaceHubPath(dir, socketPath, workspaceID string) string {
	return filepath.Join(dir, "hub-"+serverKey(socketPath)+"-"+serverKey(workspaceID)+".json")
}

func serverKey(socketPath string) string {
	sum := sha256.Sum256([]byte(socketPath))
	return fmt.Sprintf("%x", sum[:])
}

func runtimePath(dir, socketPath string) string {
	return filepath.Join(dir, "runtime-"+serverKey(socketPath)+".json")
}

// ServerScope returns a stable non-reversible key for per-session plugin data.
func ServerScope(socketPath string) string {
	if strings.TrimSpace(socketPath) == "" {
		return "local"
	}
	sum := sha256.Sum256([]byte(socketPath))
	return fmt.Sprintf("%x", sum[:8])
}
