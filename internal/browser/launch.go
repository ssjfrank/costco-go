package browser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	startupTimeout = 30 * time.Second
	closeTimeout   = 5 * time.Second

	// DevTools messages are small, but the library default of 32 KiB is too
	// tight for some events, such as responses carrying large headers.
	maxMessageSize = 32 << 20
)

// LaunchOptions configures the browser process.
type LaunchOptions struct {
	// Path is the browser executable. Empty means FindExecutable.
	Path string

	// Headless runs without a window. Only useful for tests: a person has to
	// see the window to sign in.
	Headless bool

	// Args are extra command line flags.
	Args []string
}

// Browser is a running browser with a private, temporary profile. The profile
// holds the session of whoever signs in, so Close deletes it.
type Browser struct {
	cmd        *exec.Cmd
	profileDir string
	conn       *websocket.Conn
	stderr     *stderrWatcher
	exited     chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// Launch starts a browser with a fresh profile and connects to its DevTools
// endpoint. The window opens on a blank page; Capture navigates it.
func Launch(ctx context.Context, opts LaunchOptions) (*Browser, error) {
	path := opts.Path
	if path == "" {
		found, err := FindExecutable()
		if err != nil {
			return nil, err
		}
		path = found
	}

	profileDir, err := os.MkdirTemp("", "costco-login-")
	if err != nil {
		return nil, fmt.Errorf("creating a temporary browser profile: %w", err)
	}

	args := []string{
		// Port 0 lets the browser pick a free port and report it back, so two
		// sign-ins can never collide and no fixed port is left open.
		"--remote-debugging-port=0",
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if opts.Headless {
		args = append(args, "--headless=new")
	}
	args = append(args, opts.Args...)
	args = append(args, "about:blank")

	watcher := newStderrWatcher()
	cmd := exec.Command(path, args...)
	cmd.Stderr = watcher
	// Helper processes inherit stderr and can outlive the browser for a moment;
	// don't let them hold Wait open.
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profileDir)
		return nil, fmt.Errorf("starting browser %s: %w", path, err)
	}

	b := &Browser{cmd: cmd, profileDir: profileDir, stderr: watcher, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(b.exited)
	}()

	address, err := b.waitForDevTools(ctx)
	if err != nil {
		_ = b.Close()
		return nil, err
	}

	conn, _, err := websocket.Dial(ctx, address, nil)
	if err != nil {
		_ = b.Close()
		return nil, fmt.Errorf("connecting to the browser: %w", err)
	}
	conn.SetReadLimit(maxMessageSize)
	b.conn = conn

	return b, nil
}

// ProfileDir returns the temporary profile directory that Close removes.
func (b *Browser) ProfileDir() string { return b.profileDir }

// Capture opens watch.StartURL and blocks until the watched response arrives,
// the user closes the browser, or ctx ends.
func (b *Browser) Capture(ctx context.Context, watch ResponseWatch) ([]byte, error) {
	return capture(ctx, wsTransport{b.conn}, watch)
}

// Close shuts the browser down and deletes its profile. It is safe to call more
// than once.
func (b *Browser) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.shutdown()
	})
	return b.closeErr
}

func (b *Browser) shutdown() error {
	asked := false
	if b.conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		asked = b.conn.Write(ctx, websocket.MessageText, []byte(`{"id":1000000000,"method":"Browser.close","params":{}}`)) == nil
		cancel()
	}

	if asked {
		select {
		case <-b.exited:
		case <-time.After(closeTimeout):
		}
	}
	select {
	case <-b.exited:
	default:
		_ = b.cmd.Process.Kill()
		<-b.exited
	}

	if b.conn != nil {
		_ = b.conn.CloseNow()
	}
	return removeProfile(b.profileDir)
}

// waitForDevTools returns the DevTools address once the browser reports it,
// either on stderr or in the DevToolsActivePort file, whichever comes first.
func (b *Browser) waitForDevTools(ctx context.Context) (string, error) {
	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()

	portFile := filepath.Join(b.profileDir, "DevToolsActivePort")
	for {
		select {
		case address := <-b.stderr.address:
			return address, nil
		case <-poll.C:
			if content, err := os.ReadFile(portFile); err == nil {
				if address, err := parseDevToolsActivePort(string(content)); err == nil {
					return address, nil
				}
			}
		case <-b.exited:
			return "", fmt.Errorf("the browser exited during startup%s", b.stderr.summary())
		case <-deadline.C:
			return "", fmt.Errorf("the browser did not start within %s%s", startupTimeout, b.stderr.summary())
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// removeProfile deletes the profile, retrying briefly because Windows keeps
// files locked for a moment after the browser exits.
func removeProfile(dir string) error {
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("deleting temporary browser profile %s: %w", dir, err)
}

type wsTransport struct {
	conn *websocket.Conn
}

func (t wsTransport) Read(ctx context.Context) ([]byte, error) {
	_, data, err := t.conn.Read(ctx)
	return data, err
}

func (t wsTransport) Write(ctx context.Context, data []byte) error {
	return t.conn.Write(ctx, websocket.MessageText, data)
}

// stderrWatcher receives the browser's stderr. It reports the DevTools address
// from the startup banner and keeps the tail of the output for error messages.
type stderrWatcher struct {
	mu       sync.Mutex
	pending  []byte
	tail     []byte
	reported bool
	address  chan string
}

const stderrTailSize = 2048

func newStderrWatcher() *stderrWatcher {
	return &stderrWatcher{address: make(chan string, 1)}
}

func (w *stderrWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.tail = append(w.tail, p...)
	if len(w.tail) > stderrTailSize {
		w.tail = append([]byte(nil), w.tail[len(w.tail)-stderrTailSize:]...)
	}

	if w.reported {
		return len(p), nil
	}
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		line := string(w.pending[:end])
		w.pending = w.pending[end+1:]
		if address, ok := parseDevToolsLine(line); ok {
			w.reported = true
			w.pending = nil
			w.address <- address
			break
		}
	}
	if len(w.pending) > 64*1024 {
		w.pending = nil
	}
	return len(p), nil
}

// summary renders the last few stderr lines as a suffix for an error message.
func (w *stderrWatcher) summary() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	text := strings.TrimSpace(string(w.tail))
	if text == "" {
		return ""
	}
	return ": " + text
}
