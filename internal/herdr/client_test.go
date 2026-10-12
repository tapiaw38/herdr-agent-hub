package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tapiaw38/herdr-agent-hub/internal/agents"
)

func TestFakeClient(t *testing.T) {
	f := &Fake{Agents: []agents.Agent{{ID: "a"}}}
	got, e := f.Snapshot(context.Background())
	if e != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, e)
	}
	if e = f.Focus(context.Background(), got[0]); e != nil || f.Focused != "a" {
		t.Fatalf("focus %q %v", f.Focused, e)
	}
}

func TestCLISendKeyUsesValidatedAgentCommand(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$#" -eq 4 ] || exit 11
[ "$1" = agent ] && [ "$2" = send-keys ] && [ "$3" = w3:p2 ] && [ "$4" = enter ] || exit 12
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	client := CLI{Bin: script}
	agent := agents.Agent{PaneID: "w3:p2"}
	if err := client.SendKey(context.Background(), agent, "enter"); err != nil {
		t.Fatal(err)
	}
	if err := client.SendKey(context.Background(), agent, "--help"); err == nil {
		t.Fatal("non-key input should be rejected")
	}
}

func TestCLIRenamePaneTargetsOnlyGivenPane(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$#" -eq 4 ] || exit 11
[ "$1" = pane ] && [ "$2" = rename ] && [ "$3" = w3:p2 ] && [ "$4" = Agents ] || exit 12
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	client := CLI{Bin: script}
	if err := client.RenamePane(context.Background(), "w3:p2", "Agents"); err != nil {
		t.Fatal(err)
	}
	if err := client.RenamePane(context.Background(), "", "Agents"); err == nil {
		t.Fatal("empty pane ID should not be renamed")
	}
}

func TestParseSnapshotWorkspaces(t *testing.T) {
	b := []byte(`{"result":{"snapshot":{"workspaces":[{"workspace_id":"w3","label":"project","cwd":"/home/user/project"}]}}}`)
	workspaces, err := ParseSnapshotWorkspaces(b)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != "w3" || workspaces[0].CWD != "/home/user/project" {
		t.Fatalf("workspaces=%+v err=%v", workspaces, err)
	}
}

func TestValidateStart(t *testing.T) {
	if err := ValidateStart("reviewer_1", "codex", "w2:p4", []string{"--model", "gpt-test"}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, kind, pane string }{
		{"BadName", "codex", "w2:p4"},
		{"reviewer", "unknown", "w2:p4"},
		{"reviewer", "codex", ""},
	} {
		if err := ValidateStart(test.name, test.kind, test.pane, nil); err == nil {
			t.Fatalf("expected invalid start params to fail: %+v", test)
		}
	}
	if err := ValidateStart("reviewer", "codex", "wproject-42:p1", nil); err != nil {
		t.Fatalf("opaque Herdr workspace ID should be accepted: %v", err)
	}
}

func TestParsePaneProcessInfoRequiresIdleShell(t *testing.T) {
	tests := []struct {
		name, json, process string
		ready               bool
	}{
		{"bash prompt", `{"result":{"pane_id":"w1:p1","shell_pid":10,"foreground_process_group_id":10,"foreground_processes":[{"pid":10,"name":"bash"}]}}`, "bash", true},
		{"agent running", `{"result":{"process_info":{"shell_pid":10,"foreground_process_group_id":22,"foreground_processes":[{"pid":22,"name":"claude"}]}}}`, "claude", false},
		{"different process in shell group", `{"result":{"process_info":{"shell_pid":10,"foreground_process_group_id":10,"foreground_processes":[{"pid":11,"name":"bash"}]}}}`, "bash", false},
		{"missing process data", `{"result":{"pane_id":"w1:p1"}}`, "unknown process", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ready, known := ParsePaneProcessInfo([]byte(tt.json))
			if name != tt.process || ready != tt.ready || known != (tt.name != "missing process data") {
				t.Fatalf("got (%q, %v, %v)", name, ready, known)
			}
		})
	}
}

func TestCLICreateShellTabUsesRootPaneAndChecksPrompt(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
if [ "$1" = tab ] && [ "$2" = create ]; then
  [ "$3" = --workspace ] && [ "$4" = w3 ] || exit 11
  [ "$5" = --cwd ] && [ "$6" = /repo ] || exit 12
  [ "$7" = --label ] && [ "$8" = "Agent · reviewer" ] || exit 13
  [ "$9" = --no-focus ] || exit 14
  printf '%s\n' '{"result":{"root_pane":{"pane_id":"w3:p9"}}}'
  exit 0
fi
if [ "$1" = pane ] && [ "$2" = process-info ] && [ "$3" = --pane ] && [ "$4" = w3:p9 ]; then
  printf '%s\n' '{"result":{"process_info":{"pane_id":"w3:p9","shell_pid":10,"foreground_process_group_id":10,"foreground_processes":[{"pid":10,"name":"zsh"}]}}}'
  exit 0
fi
exit 15
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	pane, err := (CLI{Bin: script}).CreateShellTab(context.Background(), "w3", "/repo", "Agent · reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if pane.ID != "w3:p9" || !pane.ShellAvailable || pane.ProcessName != "zsh" {
		t.Fatalf("created pane: %+v", pane)
	}
}

func TestCLIStartShowsHerdrErrorWithoutEchoingArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := "#!/bin/sh\nprintf '%s\\n' '{\"error\":{\"code\":\"pane_not_available\",\"message\":\"pane is busy\"}}' >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	err := (CLI{Bin: script}).Start(context.Background(), "reviewer", "codex", "w3:p9", []string{"--token", "private-value"})
	if err == nil || !strings.Contains(err.Error(), "pane_not_available") || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("unsafe or missing error: %v", err)
	}
}

func TestFakeStartRecordsExplicitArguments(t *testing.T) {
	f := &Fake{}
	args := []string{"--model", "gpt test"}
	if err := f.Start(context.Background(), "reviewer", "codex", "w2:p1", args); err != nil {
		t.Fatal(err)
	}
	if f.StartedName != "reviewer" || f.StartedKind != "codex" || f.StartedPane != "w2:p1" || len(f.StartedArgs) != 2 || f.StartedArgs[1] != "gpt test" {
		t.Fatalf("fake start request=%+v", f)
	}
}

func TestCLIStartPassesArgumentsAsArgv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$#" -eq 10 ] || exit 1
[ "$1" = agent ] || exit 2
[ "$2" = start ] || exit 3
[ "$3" = reviewer ] || exit 4
[ "$4" = --kind ] || exit 5
[ "$5" = codex ] || exit 6
[ "$6" = --pane ] || exit 7
[ "$7" = w2:p4 ] || exit 8
[ "$8" = -- ] || exit 9
[ "$9" = --model ] || exit 10
[ "${10}" = "gpt test" ] || exit 11
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	client := CLI{Bin: script}
	if err := client.Start(context.Background(), "reviewer", "codex", "w2:p4", []string{"--model", "gpt test"}); err != nil {
		t.Fatal(err)
	}
}

func TestCLIOpenPluginPaneUsesDocumentedArgv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$#" -eq 11 ] || exit 1
[ "$1" = plugin ] || exit 2
[ "$2" = pane ] || exit 3
[ "$3" = open ] || exit 4
[ "$4" = --plugin ] || exit 5
[ "$5" = community.herdr-agent-hub ] || exit 6
[ "$6" = --entrypoint ] || exit 7
[ "$7" = dashboard ] || exit 8
[ "$8" = --placement ] || exit 9
[ "$9" = zoomed ] || exit 10
[ "${10}" = --target-pane ] && [ "${11}" = w3:p2 ] || exit 11
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	if err := (CLI{Bin: script}).OpenPluginPane(context.Background(), "w3:p2"); err != nil {
		t.Fatal(err)
	}
}

func TestCLIReportsMissingBinary(t *testing.T) {
	c := CLI{Bin: "/path/that/does/not/exist"}
	if _, err := c.Snapshot(context.Background()); err == nil {
		t.Fatal("expected missing binary error")
	}
}

func TestParseSnapshotEnvelope(t *testing.T) {
	data := `{"result":{"snapshot":{"agents":[{"agent":"codex","agent_status":"blocked","name":"reviewer","pane_id":"w2:p1","workspace_id":"w2","tab_id":"w2:t1","cwd":"/repo","terminal_title_stripped":"Needs approval"}],"panes":[{"pane_id":"w2:p2"}],"workspaces":[{"workspace_id":"w2","label":"repo"}]}}}`
	got, err := ParseSnapshot([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only detected agent records, got %d", len(got))
	}
	a := got[0]
	if a.ID != "w2:p1" || a.PaneID != "w2:p1" || a.Provider != "codex" || a.Status != "WAITING" || a.Name != "reviewer" || a.Activity != "Needs approval" || a.WorkspaceName != "repo" {
		t.Fatalf("unexpected mapping: %+v", a)
	}
	if _, err = ParseSnapshot([]byte("not json")); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}

func TestParseSnapshotPanesAddsReadableContext(t *testing.T) {
	data := `{"result":{"snapshot":{"panes":[{"pane_id":"wproject-42:p3","workspace_id":"wproject-42","tab_id":"wproject-42:t1","cwd":"/repo/web","terminal_title_stripped":"shell","focused":true,"agent_status":"idle"},{"pane_id":"wproject-42:p4","workspace_id":"wproject-42","tab_id":"wproject-42:t1","cwd":"/repo/web","terminal_title_stripped":"shell","focused":false,"agent_status":"unknown"}],"tabs":[{"tab_id":"wproject-42:t1","label":"development"}],"workspaces":[{"workspace_id":"wproject-42","label":"website"}],"layouts":[{"workspace_id":"wproject-42","tab_id":"wproject-42:t1","area":{"x":0,"y":0,"width":100,"height":20},"panes":[{"pane_id":"wproject-42:p3","focused":true,"rect":{"x":0,"y":0,"width":50,"height":20}},{"pane_id":"wproject-42:p4","focused":false,"rect":{"x":50,"y":0,"width":50,"height":20}}]}]}}}`
	got, err := ParseSnapshotPanes([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "wproject-42:p3" || got[0].WorkspaceID != "wproject-42" || got[0].Workspace != "website" || got[0].Tab != "development" || got[0].CWD != "/repo/web" || got[0].Title != "shell" || got[0].Status != "idle" || !got[0].Focused || got[0].Location != "left" || got[1].Location != "right" || got[1].Number != 2 {
		t.Fatalf("unexpected pane mapping: %+v", got)
	}
}
