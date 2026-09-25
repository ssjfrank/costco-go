package costco

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eshaffer321/costco-go/internal/browser"
)

// SignInStartURL is the page the sign-in window opens on: the order history in
// Costco's account app. Signed out, it sends the user straight to the sign-in
// page and back again afterwards. The account app then fetches the OAuth tokens
// this library uses. Signing in from the home page lands elsewhere, and that
// token request never happens.
const SignInStartURL = "https://www.costco.com/myaccount/#/app/" + WCSClientID + "/ordersandpurchases"

const defaultSignInTimeout = 10 * time.Minute

// BrowserLoginOptions configures LoginWithBrowser.
type BrowserLoginOptions struct {
	// BrowserPath is the Chrome, Edge, Chromium or Brave executable to use.
	// Empty means the first one found in the usual install locations.
	BrowserPath string

	// Timeout bounds how long to wait for the user to finish signing in
	// (default 10 minutes).
	Timeout time.Duration

	// Progress, when set, receives instructions to show the user.
	Progress func(message string)

	// Overridable in tests, which serve their own sign-in pages.
	startURL    string
	isTokenURL  func(string) bool
	headless    bool
	browserArgs []string
	findBrowser func() (string, error)
}

func (o BrowserLoginOptions) withDefaults() BrowserLoginOptions {
	if o.Timeout <= 0 {
		o.Timeout = defaultSignInTimeout
	}
	if o.startURL == "" {
		o.startURL = SignInStartURL
	}
	if o.isTokenURL == nil {
		o.isTokenURL = isCostcoTokenURL
	}
	if o.findBrowser == nil {
		o.findBrowser = browser.FindExecutable
	}
	return o
}

func (o BrowserLoginOptions) report(message string) {
	if o.Progress != nil {
		o.Progress(message)
	}
}

// LoginWithBrowser opens costco.com in a new browser window with a private,
// temporary profile and waits while the user signs in there, using any method
// Costco offers (password, passkey, security key, two-step verification). When
// costco.com receives its OAuth tokens, LoginWithBrowser returns the same
// response, closes the window, and deletes the profile.
//
// The library never sees the user's credentials: it only reads the token
// response the site itself receives, exactly as the manual DevTools import does.
// Pass the result to ImportTokenResponse and SaveTokens to persist it.
func LoginWithBrowser(ctx context.Context, opts BrowserLoginOptions) (*TokenResponse, error) {
	opts = opts.withDefaults()

	path := opts.BrowserPath
	if path == "" {
		found, err := opts.findBrowser()
		if err != nil {
			return nil, fmt.Errorf("%w. Install Chrome or Edge, pass the browser's location with -browser, "+
				"or copy a token by hand with 'costco-cli -cmd import-token'", err)
		}
		path = found
	}

	signInCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	session, err := browser.Launch(signInCtx, browser.LaunchOptions{
		Path:     path,
		Headless: opts.headless,
		// Chrome reports any window with a DevTools port as automated
		// (navigator.webdriver), and sign-in pages may refuse automated browsers.
		// A person does the signing in here; the tool only reads the response.
		Args: append([]string{"--disable-blink-features=AutomationControlled"}, opts.browserArgs...),
	})
	if err != nil {
		return nil, signInError(ctx, opts, err)
	}
	defer session.Close()

	opts.report("A browser window has opened on Costco's sign-in page. Sign in there with any method you normally use;\n" +
		"the window closes by itself once your order history starts loading.")

	body, err := session.Capture(signInCtx, browser.ResponseWatch{
		StartURL: opts.startURL,
		Match:    opts.isTokenURL,
		Accept:   hasSignInTokens,
	})
	if err != nil {
		return nil, signInError(ctx, opts, err)
	}

	var tokens TokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("reading the sign-in response: %w", err)
	}

	if err := session.Close(); err != nil {
		opts.report("Warning: " + err.Error())
	}
	return &tokens, nil
}

// signInError distinguishes the sign-in timing out from the caller cancelling,
// which must be passed through untouched.
func signInError(parent context.Context, opts BrowserLoginOptions, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("timed out after %s waiting for sign-in to finish", opts.Timeout)
	}
	return err
}

// isCostcoTokenURL matches the OAuth token endpoint on Costco's sign-in host.
// Any policy segment is accepted because Costco has renamed it before.
func isCostcoTokenURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	endpoint, err := url.Parse(TokenEndpoint)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" &&
		strings.EqualFold(parsed.Host, endpoint.Host) &&
		strings.HasSuffix(parsed.Path, "/oauth2/v2.0/token")
}

// hasSignInTokens accepts only a token response that can be imported: one with
// both an ID token and a refresh token. CORS preflights and error responses from
// the same endpoint are skipped.
func hasSignInTokens(body []byte) bool {
	var response TokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}
	return response.IDToken != "" && response.RefreshToken != ""
}
