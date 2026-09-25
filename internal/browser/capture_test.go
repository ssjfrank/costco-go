package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStartURL = "https://www.costco.com/"
	testTokenURL = "https://signin.costco.com/tenant/policy/oauth2/v2.0/token"
	testTokens   = `{"id_token":"id","refresh_token":"refresh"}`
)

type sentCommand struct {
	ID        int64           `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	SessionID string          `json:"sessionId"`
}

type fakeBody struct {
	body   string
	base64 bool
}

// fakeChrome plays the browser side of the DevTools protocol. It answers the
// commands capture sends the way Chrome does, and lets each test script what the
// page does once it has been navigated.
type fakeChrome struct {
	t        *testing.T
	out      chan []byte
	commands []sentCommand
	bodies   map[string]fakeBody
	navigate func(f *fakeChrome, cmd sentCommand)
	hook     func(f *fakeChrome, cmd sentCommand)
}

func newFakeChrome(t *testing.T) *fakeChrome {
	return &fakeChrome{t: t, out: make(chan []byte, 256), bodies: map[string]fakeBody{}}
}

func (f *fakeChrome) Read(ctx context.Context) ([]byte, error) {
	select {
	case data, ok := <-f.out:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *fakeChrome) Write(_ context.Context, data []byte) error {
	var cmd sentCommand
	require.NoError(f.t, json.Unmarshal(data, &cmd))
	f.commands = append(f.commands, cmd)

	switch cmd.Method {
	case "Target.setDiscoverTargets":
		f.reply(cmd, map[string]any{})
		f.event("Target.targetCreated", "", map[string]any{
			"targetInfo": map[string]any{"targetId": "worker-1", "type": "service_worker"},
		})
		f.createPage("page-1")
	case "Target.attachToTarget":
		var params struct {
			TargetID string `json:"targetId"`
		}
		require.NoError(f.t, json.Unmarshal(cmd.Params, &params))
		f.reply(cmd, map[string]any{"sessionId": "session-" + params.TargetID})
	case "Page.navigate":
		f.reply(cmd, map[string]any{"frameId": "frame-1"})
		if f.navigate != nil {
			f.navigate(f, cmd)
		}
	case "Network.getResponseBody":
		var params struct {
			RequestID string `json:"requestId"`
		}
		require.NoError(f.t, json.Unmarshal(cmd.Params, &params))
		body, ok := f.bodies[params.RequestID]
		if !ok {
			f.fail(cmd, "No resource with given identifier found")
			break
		}
		f.reply(cmd, map[string]any{"body": body.body, "base64Encoded": body.base64})
	default:
		f.reply(cmd, map[string]any{})
	}

	if f.hook != nil {
		f.hook(f, cmd)
	}
	return nil
}

func (f *fakeChrome) send(message map[string]any) {
	data, err := json.Marshal(message)
	require.NoError(f.t, err)
	f.out <- data
}

func (f *fakeChrome) reply(cmd sentCommand, result any) {
	message := map[string]any{"id": cmd.ID, "result": result}
	if cmd.SessionID != "" {
		message["sessionId"] = cmd.SessionID
	}
	f.send(message)
}

func (f *fakeChrome) fail(cmd sentCommand, text string) {
	message := map[string]any{"id": cmd.ID, "error": map[string]any{"code": -32000, "message": text}}
	if cmd.SessionID != "" {
		message["sessionId"] = cmd.SessionID
	}
	f.send(message)
}

func (f *fakeChrome) event(method, sessionID string, params any) {
	message := map[string]any{"method": method, "params": params}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	f.send(message)
}

func (f *fakeChrome) createPage(targetID string) {
	f.event("Target.targetCreated", "", map[string]any{
		"targetInfo": map[string]any{"targetId": targetID, "type": "page", "url": "about:blank"},
	})
}

// finishResponse emits the two events Chrome sends for a completed request.
func (f *fakeChrome) finishResponse(sessionID, requestID, url string, status int) {
	f.event("Network.responseReceived", sessionID, map[string]any{
		"requestId": requestID,
		"type":      "Fetch",
		"response":  map[string]any{"url": url, "status": status},
	})
	f.event("Network.loadingFinished", sessionID, map[string]any{"requestId": requestID})
}

func (f *fakeChrome) commandsNamed(method string) []sentCommand {
	var matched []sentCommand
	for _, cmd := range f.commands {
		if cmd.Method == method {
			matched = append(matched, cmd)
		}
	}
	return matched
}

func tokenWatch() ResponseWatch {
	return ResponseWatch{
		StartURL: testStartURL,
		Match:    func(url string) bool { return url == testTokenURL },
		Accept:   func(body []byte) bool { return bytes.Contains(body, []byte("refresh_token")) },
	}
}

func captureWithin(t *testing.T, chrome *fakeChrome, watch ResponseWatch) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return capture(ctx, chrome, watch)
}

func TestCapture_ReturnsTheAcceptedResponseBody(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.bodies["preflight"] = fakeBody{body: ""}
	chrome.bodies["stylesheet"] = fakeBody{body: "body{}"}
	chrome.bodies["refused"] = fakeBody{body: `{"error":"invalid_request"}`}
	chrome.bodies["token"] = fakeBody{body: testTokens}
	chrome.navigate = func(f *fakeChrome, cmd sentCommand) {
		f.finishResponse(cmd.SessionID, "preflight", testTokenURL, 204)
		f.finishResponse(cmd.SessionID, "stylesheet", "https://www.costco.com/site.css", 200)
		f.finishResponse(cmd.SessionID, "refused", testTokenURL, 400)
		f.finishResponse(cmd.SessionID, "token", testTokenURL, 200)
	}

	body, err := captureWithin(t, chrome, tokenWatch())

	require.NoError(t, err)
	assert.JSONEq(t, testTokens, string(body))

	var fetched []string
	for _, cmd := range chrome.commandsNamed("Network.getResponseBody") {
		var params struct {
			RequestID string `json:"requestId"`
		}
		require.NoError(t, json.Unmarshal(cmd.Params, &params))
		fetched = append(fetched, params.RequestID)
	}
	assert.NotContains(t, fetched, "stylesheet", "only responses from the watched URL are read")
	assert.Contains(t, fetched, "token")
}

func TestCapture_EnablesNetworkBeforeNavigating(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.bodies["token"] = fakeBody{body: testTokens}
	chrome.navigate = func(f *fakeChrome, cmd sentCommand) {
		f.finishResponse(cmd.SessionID, "token", testTokenURL, 200)
	}

	_, err := captureWithin(t, chrome, tokenWatch())
	require.NoError(t, err)

	enableAt, navigateAt := -1, -1
	for i, cmd := range chrome.commands {
		if cmd.SessionID != "session-page-1" {
			continue
		}
		switch cmd.Method {
		case "Network.enable":
			enableAt = i
		case "Page.navigate":
			navigateAt = i
			var params struct {
				URL string `json:"url"`
			}
			require.NoError(t, json.Unmarshal(cmd.Params, &params))
			assert.Equal(t, testStartURL, params.URL)
		}
	}
	require.NotEqual(t, -1, enableAt, "network events were never enabled for the page")
	require.NotEqual(t, -1, navigateAt, "the page was never navigated")
	assert.Less(t, enableAt, navigateAt, "responses during the first navigation would be missed")

	assert.Len(t, chrome.commandsNamed("Target.attachToTarget"), 1, "non-page targets are ignored")
}

func TestCapture_DecodesBase64Bodies(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.bodies["token"] = fakeBody{body: base64.StdEncoding.EncodeToString([]byte(testTokens)), base64: true}
	chrome.navigate = func(f *fakeChrome, cmd sentCommand) {
		f.finishResponse(cmd.SessionID, "token", testTokenURL, 200)
	}

	body, err := captureWithin(t, chrome, tokenWatch())

	require.NoError(t, err)
	assert.JSONEq(t, testTokens, string(body))
}

func TestCapture_WatchesPopupWindows(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.bodies["token"] = fakeBody{body: testTokens}
	chrome.navigate = func(f *fakeChrome, _ sentCommand) {
		f.createPage("popup-1")
	}
	chrome.hook = func(f *fakeChrome, cmd sentCommand) {
		if cmd.Method == "Network.enable" && cmd.SessionID == "session-popup-1" {
			f.finishResponse(cmd.SessionID, "token", testTokenURL, 200)
		}
	}

	body, err := captureWithin(t, chrome, tokenWatch())

	require.NoError(t, err)
	assert.JSONEq(t, testTokens, string(body))
	assert.Len(t, chrome.commandsNamed("Page.navigate"), 1, "only the first window is navigated")
}

func TestCapture_ReportsNavigationFailure(t *testing.T) {
	failing := &navigateFailingChrome{fakeChrome: newFakeChrome(t), errorText: "net::ERR_INTERNET_DISCONNECTED"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := capture(ctx, failing, tokenWatch())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ERR_INTERNET_DISCONNECTED")
	assert.Contains(t, err.Error(), testStartURL)
}

// navigateFailingChrome answers Page.navigate the way Chrome does when the
// start page cannot be loaded at all.
type navigateFailingChrome struct {
	*fakeChrome
	errorText string
}

func (f *navigateFailingChrome) Write(ctx context.Context, data []byte) error {
	var cmd sentCommand
	require.NoError(f.t, json.Unmarshal(data, &cmd))
	if cmd.Method == "Page.navigate" {
		f.commands = append(f.commands, cmd)
		f.reply(cmd, map[string]any{"frameId": "frame-1", "errorText": f.errorText})
		return nil
	}
	return f.fakeChrome.Write(ctx, data)
}

func TestCapture_WindowClosedBeforeSignIn(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.navigate = func(f *fakeChrome, _ sentCommand) {
		f.event("Target.targetDestroyed", "", map[string]any{"targetId": "page-1"})
	}

	_, err := captureWithin(t, chrome, tokenWatch())

	assert.ErrorIs(t, err, ErrWindowClosed)
}

func TestCapture_BrowserExited(t *testing.T) {
	chrome := newFakeChrome(t)
	chrome.navigate = func(f *fakeChrome, _ sentCommand) {
		close(f.out)
	}

	_, err := captureWithin(t, chrome, tokenWatch())

	assert.ErrorIs(t, err, ErrBrowserClosed)
}

func TestCapture_StopsWhenContextEnds(t *testing.T) {
	chrome := newFakeChrome(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := capture(ctx, chrome, tokenWatch())

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
