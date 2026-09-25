package main

import (
	"context"
	"errors"
	"fmt"
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
