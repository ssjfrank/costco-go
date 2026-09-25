package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireBrowser returns an installed browser, skipping the test when there is
// none or when only fast tests were requested.
func requireBrowser(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("launches a real browser")
	}
	path, err := FindExecutable()
	if err != nil {
		t.Skipf("no browser available: %v", err)
	}
	return path
}

// testBrowserArgs are needed to run Chrome inside containers, which usually
// cannot provide Chrome's sandbox. Real sign-ins never pass them.
var testBrowserArgs = []string{"--no-sandbox", "--disable-dev-shm-usage"}

// newSignInSite imitates the shape of Costco's sign-in: the start page leaves for
// a sign-in page, which redirects back with a code, and the returning page
// redeems the code with a POST to the token endpoint.
func newSignInSite(t *testing.T, tokenBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><title>shop</title><script>
if (!location.search.includes("code=")) {
  location.href = "/signin";
} else {
  fetch("/oauth2/v2.0/token", {method: "POST", body: "grant_type=authorization_code"});
}
</script>`))
	})
	mux.HandleFunc("/signin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/?code=abc", http.StatusFound)
	})
	mux.HandleFunc("/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tokenBody))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestBrowser_CapturesResponseAcrossNavigations(t *testing.T) {
	path := requireBrowser(t)
	site := newSignInSite(t, testTokens)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser, err := Launch(ctx, LaunchOptions{Path: path, Headless: true, Args: testBrowserArgs})
	require.NoError(t, err)
	profile := browser.ProfileDir()
	defer browser.Close()

	body, err := browser.Capture(ctx, ResponseWatch{
		StartURL: site.URL + "/",
		Match:    func(url string) bool { return strings.HasSuffix(url, "/oauth2/v2.0/token") },
		Accept:   func(body []byte) bool { return strings.Contains(string(body), "refresh_token") },
	})
	require.NoError(t, err)
	assert.JSONEq(t, testTokens, string(body))

	require.NoError(t, browser.Close())
	_, statErr := os.Stat(profile)
	assert.True(t, os.IsNotExist(statErr), "the temporary profile holds session cookies and must be deleted")
}

func TestBrowser_CloseIsIdempotent(t *testing.T) {
	path := requireBrowser(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	browser, err := Launch(ctx, LaunchOptions{Path: path, Headless: true, Args: testBrowserArgs})
	require.NoError(t, err)

	require.NoError(t, browser.Close())
	assert.NoError(t, browser.Close())
}

func TestLaunch_ReportsMissingExecutable(t *testing.T) {
	_, err := Launch(context.Background(), LaunchOptions{Path: "/nonexistent/browser"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "/nonexistent/browser")
}
