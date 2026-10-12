package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tapiaw38/herdr-agent-hub/internal/storage"
	"github.com/tapiaw38/herdr-agent-hub/internal/ui"
)

func TestActionCapturesOriginWorkspaceBeforeFocusingHub(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_SOCKET_PATH", "socket-a")
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"w3","workspace_cwd":"/project","focused_pane_id":"w3:p2"}`)
	if err := saveActionWorkspace(dir, "w1:p1"); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.LoadLaunchWorkspace(dir, "socket-a")
	if err != nil || workspace.WorkspaceID != "w3" || workspace.CWD != "/project" {
		t.Fatalf("workspace=%+v err=%v", workspace, err)
	}
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"w1","workspace_cwd":"/plugin","focused_pane_id":"w1:p1"}`)
	if err := saveActionWorkspace(dir, "w1:p1"); err != nil {
		t.Fatal(err)
	}
	workspace, err = storage.LoadLaunchWorkspace(dir, "socket-a")
	if err != nil || workspace.WorkspaceID != "w3" {
		t.Fatalf("Hub overwrote origin: %+v err=%v", workspace, err)
	}
}

func TestDashboardWorkingDirUsesCurrentWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := storage.SaveLaunchWorkspace(dir, "socket-test", storage.LaunchWorkspace{WorkspaceID: "w2", CWD: "/other"}); err != nil {
		t.Fatal(err)
	}
	context := ui.InvocationContext{StateDir: dir, SocketPath: "socket-test", WorkspaceID: "w2", WorkspaceCWD: "/project", FocusedPaneID: "w2:p1", FocusedPaneCWD: "/project/src"}
	if got := dashboardWorkingDir(context); got != "/other" {
		t.Fatalf("current workspace cwd=%q", got)
	}
	context.WorkspaceID = "w3"
	if got := dashboardWorkingDir(context); got != "/project/src" {
		t.Fatalf("stale workspace cwd leaked: %q", got)
	}
}

func TestOpenHubUsesInvokingPane(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$1" = plugin ] && [ "$2" = pane ] && [ "$3" = open ] || exit 11
[ "${10}" = --target-pane ] && [ "${11}" = w3:p2 ] || exit 12
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", script)
	t.Setenv("HERDR_SOCKET_PATH", "socket-test")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"w3","workspace_cwd":"/project","focused_pane_id":"w3:p2","focused_pane_cwd":"/project/src"}`)
	if err := OpenHub(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenHubDoesNotJumpToHubInAnotherWorkspace(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$1" = plugin ] && [ "$2" = pane ] && [ "$3" = open ] || exit 11
[ "${10}" = --target-pane ] && [ "${11}" = w2:p1 ] || exit 12
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	if err := storage.SaveWorkspaceHubPane(dir, "socket-test", "w1", "w1:p9"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", script)
	t.Setenv("HERDR_SOCKET_PATH", "socket-test")
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"w2","workspace_cwd":"/other","focused_pane_id":"w2:p1"}`)
	if err := OpenHub(); err != nil {
		t.Fatal(err)
	}
}
