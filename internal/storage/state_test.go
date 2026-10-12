package storage

import "testing"

func TestHubPaneStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := SaveHubPane(dir, "/tmp/herdr-server-a.sock", "w2:p7"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadHubPane(dir, "/tmp/herdr-server-a.sock")
	if err != nil {
		t.Fatal(err)
	}
	if got != "w2:p7" {
		t.Fatalf("pane ID=%q", got)
	}
}

func TestLaunchWorkspaceRoundTripAndServerScope(t *testing.T) {
	dir := t.TempDir()
	want := LaunchWorkspace{WorkspaceID: "w3", CWD: "/home/user/project"}
	if err := SaveLaunchWorkspace(dir, "socket-a", want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLaunchWorkspace(dir, "socket-a")
	if err != nil || got != want {
		t.Fatalf("launch workspace=%+v err=%v", got, err)
	}
	if _, err := LoadLaunchWorkspace(dir, "socket-b"); err == nil {
		t.Fatal("workspace leaked between Herdr sessions")
	}
}

func TestWorkspaceHubPanesStaySeparate(t *testing.T) {
	dir := t.TempDir()
	if err := SaveWorkspaceHubPane(dir, "socket-a", "w1", "w1:p2"); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspaceHubPane(dir, "socket-a", "w2", "w2:p3"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ workspace, pane string }{{"w1", "w1:p2"}, {"w2", "w2:p3"}} {
		got, err := LoadWorkspaceHubPane(dir, "socket-a", item.workspace)
		if err != nil || got != item.pane {
			t.Fatalf("workspace=%s pane=%s err=%v", item.workspace, got, err)
		}
	}
	if _, err := LoadWorkspaceHubPane(dir, "socket-b", "w1"); err == nil {
		t.Fatal("workspace Hub pane leaked between sessions")
	}
}

func TestHubPaneStateIsScopedToHerdrServer(t *testing.T) {
	dir := t.TempDir()
	if err := SaveHubPane(dir, "socket-a", "w1:p1"); err != nil {
		t.Fatal(err)
	}
	if err := SaveHubPane(dir, "socket-b", "w1:p9"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHubPane(dir, "socket-c"); err == nil {
		t.Fatal("state from another Herdr server must not be selected")
	}
	got, err := LoadHubPane(dir, "socket-b")
	if err != nil || got != "w1:p9" {
		t.Fatalf("second server pane=%q err=%v", got, err)
	}
}
