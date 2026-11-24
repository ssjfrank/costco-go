package costco

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eshaffer321/costco-go/internal/browser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsCostcoTokenURL(t *testing.T) {
	cases := map[string]bool{
		TokenEndpoint: true,
		"https://signin.costco.com/e0714dd4-784d-46d6-a278-3e29553483eb/b2c_1a_sso_wcs_signup_signin_300/oauth2/v2.0/token": true,
		"https://SIGNIN.costco.com/tenant/policy/oauth2/v2.0/token?p=x":                                                     true,
		"http://signin.costco.com/tenant/policy/oauth2/v2.0/token":                                                          false,
		"https://signin.costco.com.attacker.example/tenant/policy/oauth2/v2.0/token":                                        false,
		"https://www.costco.com/oauth2/v2.0/token":                                                                          false,
		"https://signin.costco.com/tenant/policy/oauth2/v2.0/authorize":                                                     false,
		"not a url": false,
	}

	for raw, want := range cases {
		assert.Equal(t, want, isCostcoTokenURL(raw), raw)
	}
}

func TestHasSignInTokens(t *testing.T) {
	assert.True(t, hasSignInTokens([]byte(`{"id_token":"a","refresh_token":"b"}`)))
	assert.False(t, hasSignInTokens([]byte(`{"id_token":"a"}`)), "a response without a refresh token cannot be reused later")
	assert.False(t, hasSignInTokens([]byte(`{"error":"invalid_grant"}`)))
	assert.False(t, hasSignInTokens([]byte(``)), "CORS preflights have no body")
	assert.False(t, hasSignInTokens([]byte(`<html>`)))
}

func TestLoginWithBrowser_ExplainsAlternativesWhenNoBrowserIsInstalled(t *testing.T) {
	_, err := LoginWithBrowser(context.Background(), BrowserLoginOptions{
		findBrowser: func() (string, error) { return "", browser.ErrNoBrowser },
	})

	require.ErrorIs(t, err, browser.ErrNoBrowser)
	assert.Contains(t, err.Error(), "-browser")
	assert.Contains(t, err.Error(), "import-token")
}

// requireTestBrowser skips unless a real browser can be launched.
func requireTestBrowser(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("launches a real browser")
	}
	path, err := browser.FindExecutable()
	if err != nil {
		t.Skipf("no browser available: %v", err)
	}
	return path
}

// fakeCostcoSignIn serves a start page that leaves for a sign-in page and, on
// its way back, redeems a code at the token endpoint, as costco.com does. Like a
// cautious sign-in page, it refuses browsers that report themselves as
// automated. When tokenBody is empty the sign-in never completes.
func fakeCostcoSignIn(t *testing.T, tokenBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		script := `if (navigator.webdriver) { document.title = "automated browsers are not allowed"; }
else if (!location.search.includes("code=")) { location.href = "/signin"; }
else { fetch("/policy/oauth2/v2.0/token", {method: "POST"}); }`
		if tokenBody == "" {
			script = ``
		}
		_, _ = fmt.Fprintf(w, "<!doctype html><title>Costco</title><script>%s</script>", script)
	})
	mux.HandleFunc("/signin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/policy/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenBody))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func testLoginOptions(path string, site *httptest.Server) BrowserLoginOptions {
	return BrowserLoginOptions{
		BrowserPath: path,
		startURL:    site.URL + "/",
		isTokenURL: func(raw string) bool {
			return strings.HasPrefix(raw, site.URL) && strings.HasSuffix(raw, "/oauth2/v2.0/token")
		},
		headless:    true,
		browserArgs: []string{"--no-sandbox", "--disable-dev-shm-usage"},
	}
}

func TestLoginWithBrowser_ReturnsTheTokensCostcoIssues(t *testing.T) {
	path := requireTestBrowser(t)
	idToken := generateTestJWT(time.Now().Add(time.Hour).Unix())
	site := fakeCostcoSignIn(t, fmt.Sprintf(
		`{"id_token":%q,"token_type":"Bearer","refresh_token":"refresh-123","refresh_token_expires_in":7776000}`, idToken))

	var progress []string
	options := testLoginOptions(path, site)
	options.Progress = func(message string) { progress = append(progress, message) }

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tokens, err := LoginWithBrowser(ctx, options)

	require.NoError(t, err)
	assert.Equal(t, idToken, tokens.IDToken)
	assert.Equal(t, "refresh-123", tokens.RefreshToken)
	assert.Equal(t, 7776000, tokens.RefreshTokenExpiresIn)
	assert.NotEmpty(t, progress, "the user is told to sign in in the new window")
}

func TestLoginWithBrowser_GivesUpAfterTheTimeout(t *testing.T) {
	path := requireTestBrowser(t)
	site := fakeCostcoSignIn(t, "")

	options := testLoginOptions(path, site)
	options.Timeout = 3 * time.Second

	started := time.Now()
	_, err := LoginWithBrowser(context.Background(), options)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(started), 20*time.Second, "the browser must be shut down promptly")
}

func TestLoginWithBrowser_PassesThroughCallerCancellation(t *testing.T) {
	path := requireTestBrowser(t)
	site := fakeCostcoSignIn(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	options := testLoginOptions(path, site)
	options.Progress = func(string) { cancel() }

	_, err := LoginWithBrowser(ctx, options)

	assert.True(t, errors.Is(err, context.Canceled), "got %v", err)
}

// fakeCostcoSession is a sign-in site that remembers a browser by a cookie:
// a visitor without it is sent through the sign-in page, a visitor with it
// goes straight to the token request.
type fakeCostcoSession struct {
	mu          sync.Mutex
	signInPages int
}

func (s *fakeCostcoSession) start(t *testing.T, tokenBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><title>Costco</title><script>
if (document.cookie.includes("costco_session=")) { fetch("/policy/oauth2/v2.0/token", {method: "POST"}); }
else { location.href = "/signin"; }</script>`)
	})
	mux.HandleFunc("/signin", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.signInPages++
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "costco_session", Value: "kept", Path: "/", MaxAge: 3600})
		http.Redirect(w, r, "/", http.StatusFound)
	})
	mux.HandleFunc("/policy/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenBody))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (s *fakeCostcoSession) signInPageVisits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signInPages
}

func TestLoginWithBrowser_ReusesTheSessionKeptInItsProfile(t *testing.T) {
	path := requireTestBrowser(t)
	session := &fakeCostcoSession{}
	site := session.start(t, `{"id_token":"id","refresh_token":"refresh-123","refresh_token_expires_in":7776000}`)

	options := testLoginOptions(path, site)
	options.ProfileDir = filepath.Join(t.TempDir(), "browser-profile")

	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		tokens, err := LoginWithBrowser(ctx, options)
		cancel()
		require.NoError(t, err, "sign-in %d", attempt)
		assert.Equal(t, "refresh-123", tokens.RefreshToken)
	}

	assert.Equal(t, 1, session.signInPageVisits(), "the second window is already signed in and never shows the sign-in page")
	_, err := os.Stat(options.ProfileDir)
	assert.NoError(t, err, "the profile stays on disk between sign-ins")
}

func TestLoginWithBrowser_ATemporaryProfileRemembersNothing(t *testing.T) {
	path := requireTestBrowser(t)
	session := &fakeCostcoSession{}
	site := session.start(t, `{"id_token":"id","refresh_token":"refresh-123","refresh_token_expires_in":7776000}`)

	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		_, err := LoginWithBrowser(ctx, testLoginOptions(path, site))
		cancel()
		require.NoError(t, err, "sign-in %d", attempt)
	}

	assert.Equal(t, 2, session.signInPageVisits())
}

func TestBrowserProfileDir_LivesInTheConfigDirectory(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	dir, err := BrowserProfileDir()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(os.Getenv("COSTCO_TEST_CONFIG_PATH"), "browser-profile"), dir)
	assert.Contains(t, GetConfigInfo(), dir)
}
