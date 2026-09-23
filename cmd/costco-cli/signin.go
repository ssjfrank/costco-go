package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// needsSignIn reports whether the saved tokens can no longer be refreshed.
func needsSignIn(tokens *costco.StoredTokens, now time.Time) bool {
	return tokens == nil || !now.Before(tokens.RefreshTokenExpiresAt)
}

// withSignInRetry runs attempt and, if Costco no longer accepts the sign-in,
// signs in once and runs attempt again. It never signs in twice: if a fresh
// sign-in is rejected too, the second error is returned.
func withSignInRetry(ctx context.Context, allowBrowser bool, signIn, attempt func(context.Context) error) error {
	err := attempt(ctx)
	if !errors.Is(err, costco.ErrNotAuthenticated) {
		return err
	}
	if !allowBrowser {
		return fmt.Errorf("%w. Run 'costco-cli -cmd login' to sign in again", err)
	}

	if err := signIn(ctx); err != nil {
		return err
	}
	return attempt(ctx)
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

	tokens, err := costco.ImportTokenResponse(response)
	if err != nil {
		return err
	}
	if err := costco.SaveTokens(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	fmt.Fprintf(out, "✓ Signed in. You won't need to sign in again until about %s.\n\n",
		tokens.RefreshTokenExpiresAt.Format("2006-01-02"))
	return nil
}
