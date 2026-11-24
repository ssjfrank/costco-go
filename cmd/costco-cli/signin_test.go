package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsSignIn(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	assert.True(t, needsSignIn(nil, now), "no tokens saved yet")
	assert.True(t, needsSignIn(&costco.StoredTokens{RefreshTokenExpiresAt: now.Add(-time.Minute)}, now), "refresh token expired")
	assert.True(t, needsSignIn(&costco.StoredTokens{}, now), "tokens saved without an expiry cannot be trusted")
	assert.False(t, needsSignIn(&costco.StoredTokens{RefreshTokenExpiresAt: now.Add(24 * time.Hour)}, now))
}

func TestNotSignedInError_NamesTheTokenFile(t *testing.T) {
	t.Setenv("COSTCO_TOKEN_FILE", "/secrets/tokens.json")

	err := notSignedInError()

	require.ErrorIs(t, err, costco.ErrNotAuthenticated)
	assert.Contains(t, err.Error(), "/secrets/tokens.json", "a container user needs to know which file to replace")
	assert.Contains(t, err.Error(), "-cmd login")
}

func notSignedIn() error {
	return fmt.Errorf("fetching online orders: %w", costco.ErrNotAuthenticated)
}

func TestWithSignInRetry_SignsInAndRetriesOnce(t *testing.T) {
	attempts, signIns := 0, 0

	err := withSignInRetry(context.Background(), true,
		func(context.Context) error { signIns++; return nil },
		func(context.Context) error {
			attempts++
			if attempts == 1 {
				return notSignedIn()
			}
			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 1, signIns)
}

func TestWithSignInRetry_DoesNotLoopWhenSignInDoesNotHelp(t *testing.T) {
	attempts, signIns := 0, 0

	err := withSignInRetry(context.Background(), true,
		func(context.Context) error { signIns++; return nil },
		func(context.Context) error { attempts++; return notSignedIn() })

	require.ErrorIs(t, err, costco.ErrNotAuthenticated)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 1, signIns)
}

func TestWithSignInRetry_WithoutBrowserExplainsHowToSignIn(t *testing.T) {
	t.Setenv("COSTCO_TOKEN_FILE", "/secrets/tokens.json")
	signIns := 0

	err := withSignInRetry(context.Background(), false,
		func(context.Context) error { signIns++; return nil },
		func(context.Context) error { return notSignedIn() })

	require.ErrorIs(t, err, costco.ErrNotAuthenticated)
	assert.Contains(t, err.Error(), "-cmd login")
	assert.Contains(t, err.Error(), "/secrets/tokens.json", "a container user needs to know which file to replace")
	assert.Zero(t, signIns)
}

func TestWithSignInRetry_OtherErrorsPassThrough(t *testing.T) {
	outage := errors.New("503 from Costco")
	signIns := 0

	err := withSignInRetry(context.Background(), true,
		func(context.Context) error { signIns++; return nil },
		func(context.Context) error { return outage })

	assert.ErrorIs(t, err, outage)
	assert.Zero(t, signIns, "an outage is not fixed by signing in")
}

func TestWithSignInRetry_ReportsSignInFailure(t *testing.T) {
	closed := errors.New("the browser window was closed before sign-in finished")

	err := withSignInRetry(context.Background(), true,
		func(context.Context) error { return closed },
		func(context.Context) error { return notSignedIn() })

	assert.ErrorIs(t, err, closed)
}

// fakeSignIn stands in for the three ways of getting a sign-in and records
// which of them were used.
type fakeSignIn struct {
	inClipboard  *costco.TokenResponse
	windowUsable bool
	windowErr    error
	used         []string
}

func (f *fakeSignIn) ways() signInWays {
	return signInWays{
		clipboard: func(context.Context) *costco.TokenResponse {
			f.used = append(f.used, "clipboard")
			return f.inClipboard
		},
		windowUsable: func() bool { return f.windowUsable },
		window: func(context.Context) (*costco.TokenResponse, error) {
			f.used = append(f.used, "window")
			if f.windowErr != nil {
				return nil, f.windowErr
			}
			return &costco.TokenResponse{RefreshToken: "from-window"}, nil
		},
		console: func(context.Context) (*costco.TokenResponse, error) {
			f.used = append(f.used, "console")
			return &costco.TokenResponse{RefreshToken: "from-console"}, nil
		},
	}
}

func TestObtainSignIn_OpensAWindowByDefault(t *testing.T) {
	fake := &fakeSignIn{windowUsable: true}

	tokens, err := obtainSignIn(context.Background(), signInConfig{}, fake.ways(), io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "from-window", tokens.RefreshToken)
	assert.Equal(t, []string{"clipboard", "window"}, fake.used, "nobody is asked to open a console")
}

func TestObtainSignIn_PrefersASignInAlreadyCopied(t *testing.T) {
	fake := &fakeSignIn{windowUsable: true, inClipboard: &costco.TokenResponse{RefreshToken: "from-clipboard"}}

	tokens, err := obtainSignIn(context.Background(), signInConfig{}, fake.ways(), io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "from-clipboard", tokens.RefreshToken)
	assert.Equal(t, []string{"clipboard"}, fake.used, "running the console command first still works without a window")
}

func TestObtainSignIn_FallsBackToTheConsoleWhenTheWindowFails(t *testing.T) {
	fake := &fakeSignIn{windowUsable: true, windowErr: errors.New("the browser window was closed before sign-in finished")}
	var out bytes.Buffer

	tokens, err := obtainSignIn(context.Background(), signInConfig{}, fake.ways(), &out)

	require.NoError(t, err)
	assert.Equal(t, "from-console", tokens.RefreshToken)
	assert.Equal(t, []string{"clipboard", "window", "console"}, fake.used)
	assert.Contains(t, out.String(), "window was closed", "the user is told why the window did not work")
}

func TestObtainSignIn_UsesTheConsoleWhereNoWindowCanOpen(t *testing.T) {
	fake := &fakeSignIn{windowUsable: false}

	tokens, err := obtainSignIn(context.Background(), signInConfig{}, fake.ways(), io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "from-console", tokens.RefreshToken)
	assert.Equal(t, []string{"clipboard", "console"}, fake.used)
}

func TestObtainSignIn_CancellingTheWindowDoesNotStartTheConsole(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeSignIn{windowUsable: true, windowErr: context.Canceled}

	_, err := obtainSignIn(ctx, signInConfig{}, fake.ways(), io.Discard)

	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, fake.used, "console")
}

func TestObtainSignIn_BrowserLoginUsesOnlyTheWindow(t *testing.T) {
	closed := errors.New("the browser window was closed before sign-in finished")
	fake := &fakeSignIn{windowErr: closed, inClipboard: &costco.TokenResponse{RefreshToken: "from-clipboard"}}

	_, err := obtainSignIn(context.Background(), signInConfig{BrowserLogin: true}, fake.ways(), io.Discard)

	require.ErrorIs(t, err, closed)
	assert.Equal(t, []string{"window"}, fake.used, "-browser-login neither reads the clipboard nor falls back")
}

func TestObtainSignIn_ConsoleLoginNeverOpensAWindow(t *testing.T) {
	fake := &fakeSignIn{windowUsable: true}

	tokens, err := obtainSignIn(context.Background(), signInConfig{ConsoleLogin: true}, fake.ways(), io.Discard)

	require.NoError(t, err)
	assert.Equal(t, "from-console", tokens.RefreshToken)
	assert.Equal(t, []string{"console"}, fake.used)
}

func TestSignIn_RejectsContradictoryFlags(t *testing.T) {
	err := signIn(context.Background(), signInConfig{BrowserLogin: true, ConsoleLogin: true}, io.Discard)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "-browser-login")
	assert.Contains(t, err.Error(), "-console-login")
}

func TestCanOpenWindow(t *testing.T) {
	found := func() (string, error) { return "/usr/bin/google-chrome", nil }
	missing := func() (string, error) { return "", errors.New("no browser") }
	noEnv := func(string) string { return "" }
	x11 := func(key string) string { return map[string]string{"DISPLAY": ":0"}[key] }
	wayland := func(key string) string { return map[string]string{"WAYLAND_DISPLAY": "wayland-0"}[key] }

	assert.True(t, canOpenWindow("darwin", noEnv, "", found))
	assert.True(t, canOpenWindow("windows", noEnv, "", found))
	assert.False(t, canOpenWindow("darwin", noEnv, "", missing), "no browser to open")
	assert.True(t, canOpenWindow("darwin", noEnv, "/opt/chrome", missing), "-browser names one that is not in the usual places")

	assert.True(t, canOpenWindow("linux", x11, "", found))
	assert.True(t, canOpenWindow("linux", wayland, "", found))
	assert.False(t, canOpenWindow("linux", noEnv, "", found), "a server or SSH session has no screen to show a window on")
	assert.False(t, canOpenWindow("linux", noEnv, "/opt/chrome", found))
}
