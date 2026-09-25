package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// signInConfig selects how a sign-in happens.
type signInConfig struct {
	// BrowserLogin opens a sign-in window in a browser the tool starts itself,
	// instead of asking for the console snippet's output.
	BrowserLogin bool
	BrowserPath  string

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

// signIn gets a new sign-in the configured way and saves it.
func signIn(ctx context.Context, cfg signInConfig, out io.Writer) error {
	if cfg.NonInteractive {
		return notSignedInError()
	}
	if cfg.BrowserLogin {
		return signInWithBrowser(ctx, cfg.BrowserPath, out)
	}

	stale := ""
	if saved, _ := costco.LoadTokens(); saved != nil {
		stale = saved.RefreshToken
	}
	response, err := consoleSignIn{lines: stdinLines(), out: out, clipboard: readClipboard, stale: stale}.run(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("sign-in cancelled")
		}
		return err
	}
	return saveSignIn(response, out)
}

// signInWithBrowser opens the sign-in window and saves the tokens it yields.
func signInWithBrowser(ctx context.Context, browserPath string, out io.Writer) error {
	response, err := costco.LoginWithBrowser(ctx, costco.BrowserLoginOptions{
		BrowserPath: browserPath,
		Progress:    func(message string) { fmt.Fprintln(out, message) },
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("sign-in cancelled")
		}
		return fmt.Errorf("signing in: %w", err)
	}
	return saveSignIn(response, out)
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
