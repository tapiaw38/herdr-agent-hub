package herdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type TerminalFrame struct {
	Data   []byte
	Closed bool
	Err    error
}

type TerminalControllerClient interface {
	ControlTerminal(context.Context, string, int, int) (*TerminalStream, error)
}

type TerminalObserverClient interface {
	ObserveTerminal(context.Context, string, int, int) (*TerminalStream, error)
}

type TerminalStream struct {
	cancel  context.CancelFunc
	ctxDone <-chan struct{}
	frames  chan TerminalFrame
	done    chan struct{}
	close   sync.Once
	input   io.WriteCloser
	inputMu sync.Mutex
}

func (c CLI) ControlTerminal(parent context.Context, target string, cols, rows int) (*TerminalStream, error) {
	return c.openTerminalStream(parent, target, cols, rows, true)
}

func (c CLI) ObserveTerminal(parent context.Context, target string, cols, rows int) (*TerminalStream, error) {
	return c.openTerminalStream(parent, target, cols, rows, false)
}

func (c CLI) openTerminalStream(parent context.Context, target string, cols, rows int, control bool) (*TerminalStream, error) {
	if !validPaneID(target) {
		return nil, errors.New("agent has no valid live Herdr pane")
	}
	if cols < 1 || rows < 1 {
		return nil, errors.New("terminal stream dimensions must be positive")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopParent := context.AfterFunc(parent, cancel)
	mode := "observe"
	if control {
		mode = "control"
	}
	cmd := exec.CommandContext(ctx, c.Bin, "terminal", "session", mode, target, "--cols", strconv.Itoa(cols), "--rows", strconv.Itoa(rows))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stopParent()
		cancel()
		return nil, err
	}
	var stdin io.WriteCloser
	if control {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			stopParent()
			cancel()
			return nil, err
		}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		stopParent()
		cancel()
		return nil, err
	}
	stopParent()
	stream := &TerminalStream{cancel: cancel, ctxDone: ctx.Done(), frames: make(chan TerminalFrame, 2), done: make(chan struct{}), input: stdin}
	go stream.read(cmd, stdout, &stderr)
	return stream, nil
}

func (o *TerminalStream) SendInput(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	command, err := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{Type: "terminal.input", Text: string(data)})
	if err != nil {
		return err
	}
	o.inputMu.Lock()
	defer o.inputMu.Unlock()
	if o.input == nil {
		return errors.New("Herdr terminal controller is closed")
	}
	command = append(command, '\n')
	_, err = o.input.Write(command)
	return err
}

func (o *TerminalStream) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return errors.New("terminal controller dimensions must be positive")
	}
	command, err := json.Marshal(struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}{Type: "terminal.resize", Cols: cols, Rows: rows})
	if err != nil {
		return err
	}
	o.inputMu.Lock()
	defer o.inputMu.Unlock()
	if o.input == nil {
		return errors.New("Herdr terminal controller is closed")
	}
	command = append(command, '\n')
	_, err = o.input.Write(command)
	return err
}

func (o *TerminalStream) read(cmd *exec.Cmd, stdout io.ReadCloser, stderr *bytes.Buffer) {
	defer close(o.done)
	defer close(o.frames)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		frame, ok, err := parseTerminalFrame(scanner.Bytes())
		if err != nil {
			if !o.publish(TerminalFrame{Err: err}) {
				return
			}
			continue
		}
		if !ok {
			continue
		}
		if !o.publish(frame) || frame.Closed {
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		o.publish(TerminalFrame{Err: scanErr})
	} else if waitErr != nil && cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = waitErr.Error()
		}
		o.publish(TerminalFrame{Err: fmt.Errorf("Herdr terminal observer: %s", message)})
	}
}

func (o *TerminalStream) publish(frame TerminalFrame) bool {
	select {
	case o.frames <- frame:
		return true
	case <-o.ctxDone:
		return false
	case <-o.done:
		return false
	}
}

func (o *TerminalStream) Next(ctx context.Context) (TerminalFrame, error) {
	select {
	case frame, ok := <-o.frames:
		if ok {
			return frame, nil
		}
	default:
	}
	select {
	case frame, ok := <-o.frames:
		if !ok {
			return TerminalFrame{}, io.EOF
		}
		return frame, nil
	case <-ctx.Done():
		return TerminalFrame{}, ctx.Err()
	case <-o.done:
		return TerminalFrame{}, io.EOF
	}
}

func (o *TerminalStream) Close() {
	o.close.Do(func() {
		o.inputMu.Lock()
		if o.input != nil {
			command, _ := json.Marshal(struct {
				Type string `json:"type"`
			}{Type: "terminal.release"})
			_, _ = o.input.Write(append(command, '\n'))
			_ = o.input.Close()
			o.input = nil
		}
		o.inputMu.Unlock()
		o.cancel()
		<-o.done
	})
}

func parseTerminalFrame(line []byte) (TerminalFrame, bool, error) {
	var record map[string]any
	if err := json.Unmarshal(line, &record); err != nil {
		return TerminalFrame{}, false, fmt.Errorf("invalid Herdr terminal frame: %w", err)
	}
	typeName, _ := record["type"].(string)
	if typeName == "terminal.closed" {
		return TerminalFrame{Closed: true}, true, nil
	}
	if typeName != "terminal.frame" {
		return TerminalFrame{}, false, nil
	}
	data := findFrameData(record)
	if len(data) == 0 {
		return TerminalFrame{}, false, errors.New("Herdr terminal frame has no ANSI payload")
	}
	return TerminalFrame{Data: data}, true, nil
}

func findFrameData(record map[string]any) []byte {
	var candidates [][]byte
	var walk func(any, string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			for childKey, child := range typed {
				walk(child, strings.ToLower(childKey))
			}
		case []any:
			for _, child := range typed {
				walk(child, key)
			}
		case string:
			if !strings.Contains(key, "data") && !strings.Contains(key, "ansi") && !strings.Contains(key, "frame") && !strings.Contains(key, "bytes") && !strings.Contains(key, "payload") && !strings.Contains(key, "output") && !strings.Contains(key, "content") {
				return
			}
			if decoded, err := base64.StdEncoding.DecodeString(typed); err == nil && len(decoded) > 0 {
				candidates = append(candidates, decoded)
				return
			}
			if decoded, err := base64.RawStdEncoding.DecodeString(typed); err == nil && len(decoded) > 0 {
				candidates = append(candidates, decoded)
			}
		}
	}
	walk(record, "")
	var best []byte
	for _, candidate := range candidates {
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	return best
}
