package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrBrowserClosed means the browser exited or dropped the DevTools
	// connection before the watched response arrived.
	ErrBrowserClosed = errors.New("the browser closed before sign-in finished")

	// ErrWindowClosed means every browser window was closed before the watched
	// response arrived. Browsers on macOS keep running with no windows open.
	ErrWindowClosed = errors.New("the browser window was closed before sign-in finished")
)

// transport carries raw DevTools protocol messages to and from the browser.
type transport interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, data []byte) error
}

// ResponseWatch describes the page to open and the network response to wait for.
type ResponseWatch struct {
	// StartURL is loaded in the first browser window.
	StartURL string

	// Match selects responses by URL. Only matching responses have their body read.
	Match func(url string) bool

	// Accept decides whether a matching body is the one being waited for. It lets
	// the caller skip CORS preflights and error responses from the same URL.
	Accept func(body []byte) bool
}

type message struct {
	ID        int64           `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	Error     *protocolError  `json:"error"`
	SessionID string          `json:"sessionId"`
}

type protocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type requestKey struct {
	session string
	request string
}

// captureState tracks one capture. Everything happens on a single goroutine: a
// command's reply is handled when it arrives, rather than by blocking for it,
// so a slow reply can never stall the event stream.
type captureState struct {
	transport transport
	watch     ResponseWatch
	nextID    int64

	seenTargets map[string]bool
	attaching   map[int64]string // attach command id -> target id
	pages       map[string]string
	navigated   bool
	navigateID  int64
	matched     map[requestKey]bool
	reading     map[int64]bool
}

// capture opens watch.StartURL and waits for a response that satisfies watch,
// following the user across navigations and into any popup windows.
func capture(ctx context.Context, t transport, watch ResponseWatch) ([]byte, error) {
	state := &captureState{
		transport:   t,
		watch:       watch,
		seenTargets: map[string]bool{},
		attaching:   map[int64]string{},
		pages:       map[string]string{},
		matched:     map[requestKey]bool{},
		reading:     map[int64]bool{},
	}

	if _, err := state.send(ctx, "", "Target.setDiscoverTargets", map[string]any{"discover": true}); err != nil {
		return nil, err
	}

	for {
		data, err := t.Read(ctx)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, fmt.Errorf("%w: %v", ErrBrowserClosed, err)
		}

		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		body, done, err := state.handle(ctx, msg)
		if err != nil {
			return nil, err
		}
		if done {
			return body, nil
		}
	}
}

func (s *captureState) send(ctx context.Context, sessionID, method string, params any) (int64, error) {
	if params == nil {
		params = map[string]any{}
	}
	s.nextID++

	payload, err := json.Marshal(struct {
		ID        int64  `json:"id"`
		Method    string `json:"method"`
		Params    any    `json:"params"`
		SessionID string `json:"sessionId,omitempty"`
	}{s.nextID, method, params, sessionID})
	if err != nil {
		return 0, err
	}

	if err := s.transport.Write(ctx, payload); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return 0, fmt.Errorf("%w: %v", ErrBrowserClosed, err)
	}
	return s.nextID, nil
}

func (s *captureState) handle(ctx context.Context, msg message) ([]byte, bool, error) {
	if msg.ID != 0 {
		return s.handleReply(ctx, msg)
	}

	switch msg.Method {
	case "Target.targetCreated", "Target.targetInfoChanged":
		var params struct {
			TargetInfo struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
			} `json:"targetInfo"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, false, nil
		}
		target := params.TargetInfo
		if target.Type != "page" || s.seenTargets[target.TargetID] {
			return nil, false, nil
		}
		s.seenTargets[target.TargetID] = true

		id, err := s.send(ctx, "", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
		if err != nil {
			return nil, false, err
		}
		s.attaching[id] = target.TargetID

	case "Target.targetDestroyed":
		var params struct {
			TargetID string `json:"targetId"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, false, nil
		}
		if _, attached := s.pages[params.TargetID]; !attached {
			return nil, false, nil
		}
		delete(s.pages, params.TargetID)
		if s.navigated && len(s.pages) == 0 {
			return nil, false, ErrWindowClosed
		}

	case "Network.responseReceived":
		var params struct {
			RequestID string `json:"requestId"`
			Response  struct {
				URL string `json:"url"`
			} `json:"response"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, false, nil
		}
		if s.watch.Match(params.Response.URL) {
			s.matched[requestKey{msg.SessionID, params.RequestID}] = true
		}

	case "Network.loadingFinished":
		var params struct {
			RequestID string `json:"requestId"`
		}
		if json.Unmarshal(msg.Params, &params) != nil {
			return nil, false, nil
		}
		key := requestKey{msg.SessionID, params.RequestID}
		if !s.matched[key] {
			return nil, false, nil
		}
		delete(s.matched, key)

		id, err := s.send(ctx, msg.SessionID, "Network.getResponseBody", map[string]any{"requestId": params.RequestID})
		if err != nil {
			return nil, false, err
		}
		s.reading[id] = true
	}

	return nil, false, nil
}

func (s *captureState) handleReply(ctx context.Context, msg message) ([]byte, bool, error) {
	if targetID, ok := s.attaching[msg.ID]; ok {
		delete(s.attaching, msg.ID)
		return nil, false, s.attached(ctx, targetID, msg)
	}

	if msg.ID == s.navigateID {
		if msg.Error != nil {
			return nil, false, fmt.Errorf("opening %s: %s", s.watch.StartURL, msg.Error.Message)
		}
		var result struct {
			ErrorText string `json:"errorText"`
		}
		if json.Unmarshal(msg.Result, &result) == nil && result.ErrorText != "" {
			return nil, false, fmt.Errorf("opening %s: %s", s.watch.StartURL, result.ErrorText)
		}
		return nil, false, nil
	}

	if s.reading[msg.ID] {
		delete(s.reading, msg.ID)
		// A body can be gone by the time it is requested, for example after a
		// redirect; that response simply was not the one being waited for.
		if msg.Error != nil {
			return nil, false, nil
		}
		var result struct {
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}
		if json.Unmarshal(msg.Result, &result) != nil {
			return nil, false, nil
		}
		body := []byte(result.Body)
		if result.Base64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(result.Body)
			if err != nil {
				return nil, false, nil
			}
			body = decoded
		}
		if s.watch.Accept(body) {
			return body, true, nil
		}
	}

	return nil, false, nil
}

// attached enables network events on a newly attached window and, if it is the
// first window, sends it to the start page. Network events are enabled first so
// that nothing the start page does can be missed.
func (s *captureState) attached(ctx context.Context, targetID string, msg message) error {
	if msg.Error != nil {
		return nil
	}
	var result struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(msg.Result, &result) != nil || result.SessionID == "" {
		return nil
	}
	s.pages[targetID] = result.SessionID

	if _, err := s.send(ctx, result.SessionID, "Network.enable", nil); err != nil {
		return err
	}
	if s.navigated {
		return nil
	}

	s.navigated = true
	id, err := s.send(ctx, result.SessionID, "Page.navigate", map[string]any{"url": s.watch.StartURL})
	if err != nil {
		return err
	}
	s.navigateID = id
	return nil
}
