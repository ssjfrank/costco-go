package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signInBlock renders a sign-in the way the console snippet prints it.
func signInBlock(refreshToken string) string {
	payload := fmt.Sprintf(`{"id_token":"id-for-%s","refresh_token":%q,"refresh_token_expires_in":7776000}`, refreshToken, refreshToken)
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	var lines []string
	for len(encoded) > 64 {
		lines = append(lines, encoded[:64])
		encoded = encoded[64:]
	}
	lines = append(lines, encoded)
	return "-----BEGIN COSTCO SIGN-IN-----\n" + strings.Join(lines, "\n") + "\n-----END COSTCO SIGN-IN-----"
}

// clipboardSequence returns each value in turn, then keeps returning the last.
func clipboardSequence(values ...string) (func(context.Context) (string, error), *int) {
	calls := 0
	return func(context.Context) (string, error) {
		value := values[min(calls, len(values)-1)]
		calls++
		if value == "" {
			return "", errors.New("clipboard is empty")
		}
		return value, nil
	}, &calls
}

func runConsoleSignIn(t *testing.T, s consoleSignIn) (*costco.TokenResponse, string, error) {
	t.Helper()
	var out bytes.Buffer
	s.out = &out
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tokens, err := s.run(ctx)
	return tokens, out.String(), err
}

func TestConsoleSignIn_UsesTheClipboardWithoutAsking(t *testing.T) {
	clipboard, _ := clipboardSequence(signInBlock("from-clipboard"))

	tokens, out, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader("")), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "from-clipboard", tokens.RefreshToken)
	assert.NotContains(t, out, costco.ConsoleSnippet(), "no instructions are needed when the sign-in is already copied")
}

func TestConsoleSignIn_ExplainsWhatToDo(t *testing.T) {
	clipboard, _ := clipboardSequence("")

	_, out, _ := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader("")), clipboard: clipboard})

	assert.Contains(t, out, "https://www.costco.com")
	assert.Contains(t, out, "Orders & Returns")
	assert.Contains(t, out, "F12")
	assert.Contains(t, out, "allow pasting")
	assert.Contains(t, out, costco.ConsoleSnippet())
}

func TestConsoleSignIn_EnterChecksTheClipboardAgain(t *testing.T) {
	clipboard, calls := clipboardSequence("", signInBlock("copied-later"))

	tokens, _, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader("\n")), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "copied-later", tokens.RefreshToken)
	assert.Equal(t, 2, *calls)
}

func TestConsoleSignIn_EnterWithNothingCopiedKeepsWaiting(t *testing.T) {
	clipboard, _ := clipboardSequence("", "", signInBlock("third-time"))

	tokens, out, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader("\n\n")), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "third-time", tokens.RefreshToken)
	assert.Contains(t, out, "No Costco sign-in in the clipboard yet")
}

func TestConsoleSignIn_IgnoresTheSignInThatJustStoppedWorking(t *testing.T) {
	clipboard, _ := clipboardSequence(signInBlock("rejected"))
	pasted := signInBlock("pasted") + "\n"

	tokens, out, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader(pasted)), clipboard: clipboard, stale: "rejected"})

	require.NoError(t, err)
	assert.Equal(t, "pasted", tokens.RefreshToken)
	assert.Contains(t, out, costco.ConsoleSnippet(), "a rejected sign-in in the clipboard means a new one is needed")
}

func TestConsoleSignIn_AcceptsAPastedBlock(t *testing.T) {
	clipboard, _ := clipboardSequence("")
	pasted := "VM42:1 " + strings.ReplaceAll(signInBlock("pasted"), "\n", "\r\n") + "\r\n"

	tokens, _, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader(pasted)), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "pasted", tokens.RefreshToken)
}

func TestConsoleSignIn_AcceptsAPastedTokenResponse(t *testing.T) {
	clipboard, _ := clipboardSequence("")
	pasted := `{"id_token":"id","refresh_token":"raw-json","refresh_token_expires_in":7776000}` + "\n"

	tokens, _, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader(pasted)), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "raw-json", tokens.RefreshToken)
}

func TestConsoleSignIn_RecoversFromADamagedPaste(t *testing.T) {
	clipboard, _ := clipboardSequence("")
	pasted := "-----BEGIN COSTCO SIGN-IN-----\n%%%\n-----END COSTCO SIGN-IN-----\n" + signInBlock("second-try") + "\n"

	tokens, out, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader(pasted)), clipboard: clipboard})

	require.NoError(t, err)
	assert.Equal(t, "second-try", tokens.RefreshToken)
	assert.Contains(t, out, "damaged")
}

func TestConsoleSignIn_StopsAtEndOfInput(t *testing.T) {
	clipboard, _ := clipboardSequence("")

	_, _, err := runConsoleSignIn(t, consoleSignIn{lines: readLines(strings.NewReader("")), clipboard: clipboard})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sign-in")
}

func TestConsoleSignIn_StopsWhenCancelled(t *testing.T) {
	clipboard, _ := clipboardSequence("")
	reader, writer := io.Pipe()
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := consoleSignIn{lines: readLines(reader), out: io.Discard, clipboard: clipboard}.run(ctx)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestClipboardCommands(t *testing.T) {
	noEnv := func(string) string { return "" }
	desktop := func(key string) string {
		return map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}[key]
	}

	assert.Equal(t, [][]string{{"pbpaste"}}, clipboardCommands("darwin", noEnv))
	assert.Equal(t, "powershell.exe", clipboardCommands("windows", noEnv)[0][0])
	assert.Empty(t, clipboardCommands("linux", noEnv), "a machine without a desktop has no clipboard to read")

	linux := clipboardCommands("linux", desktop)
	require.Len(t, linux, 3)
	assert.Equal(t, "wl-paste", linux[0][0])
	assert.Equal(t, "xclip", linux[1][0])
	assert.Equal(t, "xsel", linux[2][0])
}
