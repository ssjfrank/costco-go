package costco

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/eshaffer321/costco-go/internal/browser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wrapSignIn builds a sign-in block the way the console snippet does.
func wrapSignIn(t *testing.T, payload string) string {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	var lines []string
	for len(encoded) > 64 {
		lines = append(lines, encoded[:64])
		encoded = encoded[64:]
	}
	lines = append(lines, encoded)
	return signInBlockBegin + "\n" + strings.Join(lines, "\n") + "\n" + signInBlockEnd
}

const sampleSignIn = `{"id_token":"id-abc","refresh_token":"refresh-xyz","refresh_token_expires_in":7776000}`

func TestParseSignIn_Block(t *testing.T) {
	tokens, err := ParseSignIn(wrapSignIn(t, sampleSignIn))

	require.NoError(t, err)
	assert.Equal(t, "id-abc", tokens.IDToken)
	assert.Equal(t, "refresh-xyz", tokens.RefreshToken)
	assert.Equal(t, 7776000, tokens.RefreshTokenExpiresIn)
}

func TestParseSignIn_BlockSurvivesCopyAndPaste(t *testing.T) {
	block := wrapSignIn(t, sampleSignIn)
	// Consoles prefix the output with a source location, terminals and editors
	// indent or switch to CRLF, and the snippet's own message follows the block.
	mangled := "VM1234:1 " + strings.ReplaceAll(block, "\n", "\r\n    ") + "\r\n\r\nCopied. Now run costco-cli."

	tokens, err := ParseSignIn(mangled)

	require.NoError(t, err)
	assert.Equal(t, "refresh-xyz", tokens.RefreshToken)
}

func TestParseSignIn_RawTokenResponse(t *testing.T) {
	tokens, err := ParseSignIn("  " + sampleSignIn + "\n")

	require.NoError(t, err)
	assert.Equal(t, "refresh-xyz", tokens.RefreshToken)
}

func TestParseSignIn_RejectsTruncatedBlock(t *testing.T) {
	block := wrapSignIn(t, sampleSignIn)
	truncated := block[:len(block)/2]

	_, err := ParseSignIn(truncated)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestParseSignIn_RejectsUnusableInput(t *testing.T) {
	for _, input := range []string{
		"",
		"hello",
		wrapSignIn(t, `{"id_token":"only-an-id"}`),
		signInBlockBegin + "\n!!!not base64!!!\n" + signInBlockEnd,
	} {
		_, err := ParseSignIn(input)
		assert.Error(t, err, "input %q should be rejected", input)
	}
}

func TestConsoleSnippet_TargetsCostco(t *testing.T) {
	snippet := ConsoleSnippet()

	assert.Contains(t, snippet, TokenEndpoint)
	assert.Contains(t, snippet, ClientID)
	assert.NotContains(t, snippet, "\n", "a single line pastes cleanly into any console")
}

// The guides tell people to paste the snippet, so they must never drift from it.
func TestDocsShowTheCurrentConsoleSnippet(t *testing.T) {
	for _, doc := range []string{"../../README.md", "../../docs/GUIDE.zh-CN.md"} {
		content, err := os.ReadFile(doc)
		require.NoError(t, err)
		assert.Contains(t, string(content), ConsoleSnippet(), "%s shows an outdated console snippet", doc)
	}
}

// msalSite serves a page whose storage holds MSAL cache entries, plus a token
// endpoint on the same origin. It records the refresh requests it receives.
type msalSite struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []url.Values
}

func newMSALSite(t *testing.T, storageScript string, tokenStatus int, tokenBody string) *msalSite {
	t.Helper()
	site := &msalSite{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, "<!doctype html><title>Orders</title><script>%s</script>", storageScript)
	})
	mux.HandleFunc("/policy/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		site.mu.Lock()
		site.requests = append(site.requests, r.PostForm)
		site.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(tokenStatus)
		_, _ = w.Write([]byte(tokenBody))
	})
	site.server = httptest.NewServer(mux)
	t.Cleanup(site.server.Close)
	return site
}

// msalEntries renders a script that stores MSAL 2.x cache records.
func msalEntries(storage, clientID, idToken, refreshToken string) string {
	account := "uid.tenant-b2c_1a_sso_wcs_signup_signin_209"
	entries := map[string]map[string]string{
		account + "-signin.costco.com-idtoken-" + clientID + "-tenant---": {
			"credentialType": "IdToken", "homeAccountId": account, "clientId": clientID, "secret": idToken,
		},
		account + "-signin.costco.com-refreshtoken-" + clientID + "--": {
			"credentialType": "RefreshToken", "homeAccountId": account, "clientId": clientID, "secret": refreshToken,
		},
		"msal.token.keys." + clientID: {"note": "not a credential"},
	}
	var script strings.Builder
	for key, value := range entries {
		encoded, _ := json.Marshal(value)
		fmt.Fprintf(&script, "%s.setItem(%q, %q);", storage, key, string(encoded))
	}
	return script.String()
}

// runSnippet opens pageURL in a real browser and runs the snippet there the way
// the DevTools console does, returning what the snippet resolves to.
func runSnippet(t *testing.T, pageURL, snippet string) (string, bool) {
	t.Helper()
	path := requireTestBrowser(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	session, err := browser.Launch(ctx, browser.LaunchOptions{
		Path: path, Headless: true, Args: []string{"--no-sandbox", "--disable-dev-shm-usage"},
	})
	require.NoError(t, err)
	defer session.Close()

	page := dialFirstPage(t, ctx, session.ProfileDir())
	defer page.CloseNow()

	call := devtoolsCaller(t, ctx, page)
	call("Page.navigate", map[string]any{"url": pageURL})
	require.Eventually(t, func() bool {
		result := call("Runtime.evaluate", map[string]any{
			"expression": fmt.Sprintf("document.readyState === 'complete' && location.href.startsWith(%q)", pageURL), "returnByValue": true,
		})
		return strings.Contains(string(result), `"value":true`)
	}, 20*time.Second, 100*time.Millisecond)

	raw := call("Runtime.evaluate", map[string]any{
		"expression":            snippet,
		"awaitPromise":          true,
		"returnByValue":         true,
		"includeCommandLineAPI": true,
	})
	var result struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	return result.Result.Value, result.Result.Type == "string"
}

func dialFirstPage(t *testing.T, ctx context.Context, profileDir string) *websocket.Conn {
	t.Helper()
	var port string
	require.Eventually(t, func() bool {
		content, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort"))
		if err != nil {
			return false
		}
		port = strings.Split(string(content), "\n")[0]
		return port != ""
	}, 10*time.Second, 50*time.Millisecond)

	response, err := http.Get("http://127.0.0.1:" + port + "/json/list")
	require.NoError(t, err)
	defer response.Body.Close()
	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&targets))

	for _, target := range targets {
		if target.Type == "page" {
			conn, _, err := websocket.Dial(ctx, target.WebSocketDebuggerURL, nil)
			require.NoError(t, err)
			conn.SetReadLimit(16 << 20)
			return conn
		}
	}
	t.Fatal("the browser has no page")
	return nil
}

func devtoolsCaller(t *testing.T, ctx context.Context, conn *websocket.Conn) func(string, map[string]any) json.RawMessage {
	id := 0
	return func(method string, params map[string]any) json.RawMessage {
		id++
		payload, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
		require.NoError(t, conn.Write(ctx, websocket.MessageText, payload))
		for {
			_, data, err := conn.Read(ctx)
			require.NoError(t, err)
			var reply struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			_ = json.Unmarshal(data, &reply)
			if reply.ID == id {
				return reply.Result
			}
		}
	}
}

func TestConsoleSnippet_RefreshesTheSignInFoundOnThePage(t *testing.T) {
	for _, storage := range []string{"sessionStorage", "localStorage"} {
		t.Run(storage, func(t *testing.T) {
			fresh := `{"id_token":"fresh-id","refresh_token":"fresh-refresh","refresh_token_expires_in":7775000}`
			site := newMSALSite(t, msalEntries(storage, ClientID, "cached-id", "cached-refresh"), http.StatusOK, fresh)
			snippet := consoleSnippet(site.server.URL+"/policy/oauth2/v2.0/token", ClientID)

			block, ok := runSnippet(t, site.server.URL+"/", snippet)
			require.True(t, ok, "the snippet should resolve to the sign-in block")

			tokens, err := ParseSignIn(block)
			require.NoError(t, err)
			assert.Equal(t, "fresh-id", tokens.IDToken)
			assert.Equal(t, "fresh-refresh", tokens.RefreshToken)
			assert.Equal(t, 7775000, tokens.RefreshTokenExpiresIn)

			require.Len(t, site.requests, 1)
			assert.Equal(t, "refresh_token", site.requests[0].Get("grant_type"))
			assert.Equal(t, ClientID, site.requests[0].Get("client_id"))
			assert.Equal(t, "cached-refresh", site.requests[0].Get("refresh_token"))
		})
	}
}

func TestConsoleSnippet_FallsBackToCachedTokensWhenCostcoIsUnreachable(t *testing.T) {
	site := newMSALSite(t, msalEntries("sessionStorage", ClientID, "cached-id", "cached-refresh"), http.StatusOK, "{}")
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	snippet := consoleSnippet(unreachable.URL+"/policy/oauth2/v2.0/token", ClientID)

	block, ok := runSnippet(t, site.server.URL+"/", snippet)
	require.True(t, ok)

	tokens, err := ParseSignIn(block)
	require.NoError(t, err)
	assert.Equal(t, "cached-id", tokens.IDToken)
	assert.Equal(t, "cached-refresh", tokens.RefreshToken)
	assert.Positive(t, tokens.RefreshTokenExpiresIn, "an import with no lifetime would count as expired at once")
}

func TestConsoleSnippet_ReportsARefusedSignIn(t *testing.T) {
	site := newMSALSite(t, msalEntries("sessionStorage", ClientID, "cached-id", "cached-refresh"),
		http.StatusBadRequest, `{"error":"invalid_grant","error_description":"AADB2C90080: The provided grant has expired."}`)
	snippet := consoleSnippet(site.server.URL+"/policy/oauth2/v2.0/token", ClientID)

	_, ok := runSnippet(t, site.server.URL+"/", snippet)

	assert.False(t, ok, "a refused sign-in must not be copied")
}

func TestConsoleSnippet_FindsNothingWhenSignedOut(t *testing.T) {
	site := newMSALSite(t, "", http.StatusOK, "{}")
	snippet := consoleSnippet(site.server.URL+"/policy/oauth2/v2.0/token", ClientID)

	_, ok := runSnippet(t, site.server.URL+"/", snippet)

	assert.False(t, ok)
	assert.Empty(t, site.requests, "nothing is sent without a sign-in to refresh")
}

func TestConsoleSnippet_IgnoresOtherAppsSignIns(t *testing.T) {
	site := newMSALSite(t, msalEntries("localStorage", "some-other-client", "other-id", "other-refresh"), http.StatusOK, "{}")
	snippet := consoleSnippet(site.server.URL+"/policy/oauth2/v2.0/token", ClientID)

	_, ok := runSnippet(t, site.server.URL+"/", snippet)

	assert.False(t, ok, "only the order history app's tokens work with this client")
	assert.Empty(t, site.requests)
}
