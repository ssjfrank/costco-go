package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/eshaffer321/costco-go/internal/browser"
	"github.com/eshaffer321/costco-go/pkg/costco"
)

// signInConfig selects how a sign-in happens. With neither BrowserLogin nor
// ConsoleLogin set, a window is opened where one can be, and the console
// command is the fallback.
type signInConfig struct {
	// BrowserLogin signs in through a window the tool opens, and nothing else.
	BrowserLogin bool
	BrowserPath  string

	// ConsoleLogin signs in with the console command, and never opens a window.
	ConsoleLogin bool

	// TempProfile gives the sign-in window a throwaway browser profile instead
	// of the kept one, so no Costco cookies stay on disk afterwards.
	TempProfile bool

	// NonInteractive never asks for a sign-in; commands fail with instructions.
	NonInteractive bool
}

// stdinLines is shared by every prompt in a run; see readLines.
var stdinLines = sync.OnceValue(func() <-chan string { return readLines(os.Stdin) })

// needsSignIn reports whether the saved tokens can no longer be refreshed.
func needsSignIn(tokens *costco.StoredTokens, now time.Time) bool {
	return tokens == nil || !now.Before(tokens.RefreshTokenExpiresAt)
}

// withSignInRetry runs attempt and, if Costco no longer accepts the sign-in,
// signs in once and runs attempt again. It never signs in twice: if a fresh
// sign-in is rejected too, the second error is returned.
func withSignInRetry(ctx context.Context, allowSignIn bool, signIn, attempt func(context.Context) error) error {
	err := attempt(ctx)
	if !errors.Is(err, costco.ErrNotAuthenticated) {
		return err
	}
	if !allowSignIn {
		return fmt.Errorf("%w. Replace %s with a fresh sign-in: run 'costco-cli -cmd login' on a machine with your browser",
			err, tokenFileForMessages())
	}

	if err := signIn(ctx); err != nil {
		return err
	}
	return attempt(ctx)
}

// notSignedInError explains, for runs that cannot ask, which token file needs
// replacing. In a container that is the mounted file, refreshed on the host.
func notSignedInError() error {
	return fmt.Errorf("%w: no usable sign-in in %s. Run 'costco-cli -cmd login' on a machine with your browser to create a fresh one",
		costco.ErrNotAuthenticated, tokenFileForMessages())
}

func tokenFileForMessages() string {
	path, err := costco.TokenFilePath()
	if err != nil {
		return "the token file"
	}
	return path
}

// signInWays are the ways of getting a sign-in. Tests replace them.
type signInWays struct {
	// clipboard returns a sign-in the console command has already copied, or nil.
	clipboard func(context.Context) *costco.TokenResponse

	// windowUsable reports whether a sign-in window can be opened here.
	windowUsable func() bool
	window       func(context.Context) (*costco.TokenResponse, error)

	console func(context.Context) (*costco.TokenResponse, error)
}

// signIn gets a new sign-in the configured way and saves it.
func signIn(ctx context.Context, cfg signInConfig, out io.Writer) error {
	if cfg.NonInteractive {
		return notSignedInError()
	}
	if cfg.BrowserLogin && cfg.ConsoleLogin {
		return errors.New("-browser-login and -console-login are two different ways to sign in; pass one of them")
	}

	stale := ""
	if saved, _ := costco.LoadTokens(); saved != nil {
		stale = saved.RefreshToken
	}
	console := consoleSignIn{lines: stdinLines(), out: out, clipboard: readClipboard, stale: stale}

	response, err := obtainSignIn(ctx, cfg, signInWays{
		clipboard: console.fromClipboard,
		windowUsable: func() bool {
			return canOpenWindow(runtime.GOOS, os.Getenv, cfg.BrowserPath, browser.FindExecutable)
		},
		window:  func(ctx context.Context) (*costco.TokenResponse, error) { return openSignInWindow(ctx, cfg, out) },
		console: console.run,
	}, out)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("sign-in cancelled")
		}
		return err
	}
	return saveSignIn(response, out)
}

// obtainSignIn picks the way to sign in. Left to itself it asks for as little
// as it can: a sign-in the console command already copied is used as it is,
// otherwise a window opens, and the console command is explained only where no
// window can open or the window did not work out.
func obtainSignIn(ctx context.Context, cfg signInConfig, ways signInWays, out io.Writer) (*costco.TokenResponse, error) {
	switch {
	case cfg.ConsoleLogin:
		return ways.console(ctx)
	case cfg.BrowserLogin:
		response, err := ways.window(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("signing in: %w", err)
		}
		return response, err
	}

	if response := ways.clipboard(ctx); response != nil {
		return response, nil
	}
	if ways.windowUsable() {
		response, err := ways.window(ctx)
		if err == nil {
			return response, nil
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return nil, err
		}
		fmt.Fprintf(out, "\nThe sign-in window did not work: %v.\nYou can sign in with your usual browser instead.\n\n", err)
	}
	return ways.console(ctx)
}

// canOpenWindow reports whether a sign-in window can be shown on this machine:
// there has to be a browser to start and, outside macOS and Windows, a display
// to show it on, which a server or an SSH session does not have.
func canOpenWindow(goos string, getenv func(string) string, browserPath string, findBrowser func() (string, error)) bool {
	if goos != "darwin" && goos != "windows" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	if browserPath != "" {
		return true
	}
	_, err := findBrowser()
	return err == nil
}

// openSignInWindow opens the sign-in window and returns the tokens it yields.
func openSignInWindow(ctx context.Context, cfg signInConfig, out io.Writer) (*costco.TokenResponse, error) {
	options := costco.BrowserLoginOptions{
		BrowserPath: cfg.BrowserPath,
		Progress:    func(message string) { fmt.Fprintln(out, message) },
	}
	if !cfg.TempProfile {
		profileDir, err := costco.BrowserProfileDir()
		if err != nil {
			return nil, err
		}
		options.ProfileDir = profileDir
	}
	return costco.LoginWithBrowser(ctx, options)
}

func saveSignIn(response *costco.TokenResponse, out io.Writer) error {
	tokens, err := costco.ImportTokenResponse(response)
	if err != nil {
		return err
	}
	if err := costco.SaveTokens(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	path, _ := costco.TokenFilePath()
	fmt.Fprintf(out, "✓ Signed in. You won't need to sign in again until about %s.\n  Saved to %s\n\n",
		tokens.RefreshTokenExpiresAt.Format("2006-01-02"), path)
	return nil
}
