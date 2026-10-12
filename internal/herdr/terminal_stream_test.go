package herdr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIControlTerminalStreamsFrames(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$#" -eq 8 ] || exit 11
[ "$1" = terminal ] && [ "$2" = session ] && [ "$3" = control ] && [ "$4" = w3:p2 ] || exit 12
[ "$5" = --cols ] && [ "$6" = 80 ] && [ "$7" = --rows ] && [ "$8" = 24 ] || exit 13
printf '%s\n' '{"type":"terminal.frame","data_base64":"G1sySg=="}' '{"type":"terminal.closed"}'
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	observer, err := (CLI{Bin: script}).ControlTerminal(context.Background(), "w3:p2", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	frame, err := observer.Next(context.Background())
	if err != nil || string(frame.Data) != "\x1b[2J" || frame.Closed {
		t.Fatalf("frame=%+v err=%v", frame, err)
	}
	frame, err = observer.Next(context.Background())
	if err != nil || !frame.Closed {
		t.Fatalf("closed frame=%+v err=%v", frame, err)
	}
}

func TestCLIControlTerminalValidatesTargetAndSize(t *testing.T) {
	client := CLI{Bin: "unused"}
	if _, err := client.ControlTerminal(context.Background(), "w3:p2; rm", 80, 24); err == nil {
		t.Fatal("invalid pane target accepted")
	}
	if _, err := client.ControlTerminal(context.Background(), "w3:p2", 0, 24); err == nil {
		t.Fatal("invalid terminal dimensions accepted")
	}
}

func TestCLIObserveTerminalStreamsReadOnlyFrames(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$3" = observe ] || exit 11
printf '%s\n' '{"type":"terminal.frame","data_base64":"SGVsbG8="}' '{"type":"terminal.closed"}'
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	stream, err := (CLI{Bin: script}).ObserveTerminal(context.Background(), "w3:p2", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	frame, err := stream.Next(context.Background())
	if err != nil || string(frame.Data) != "Hello" {
		t.Fatalf("frame=%+v err=%v", frame, err)
	}
}

func TestCLIControlTerminalSendsInputAndResize(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "herdr-fixture")
	content := `#!/bin/sh
[ "$3" = control ] || exit 11
IFS= read -r input || exit 12
case "$input" in *'"type":"terminal.input"'*'"text":"holañ"'*) ;; *) exit 13 ;; esac
IFS= read -r resize || exit 14
case "$resize" in *'"type":"terminal.resize"'*'"cols":100'*'"rows":30'*) ;; *) exit 15 ;; esac
IFS= read -r release || exit 16
case "$release" in *'"type":"terminal.release"'*) exit 0 ;; *) exit 17 ;; esac
`
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	controller, err := (CLI{Bin: script}).ControlTerminal(context.Background(), "w3:p2", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.SendInput([]byte("holañ")); err != nil {
		t.Fatal(err)
	}
	if err := controller.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	controller.Close()
}
