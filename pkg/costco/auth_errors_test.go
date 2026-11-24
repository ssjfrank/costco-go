package costco

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenIfNeeded_WithoutTokensIsNotAuthenticated(t *testing.T) {
	client := &Client{config: Config{TokenRefreshBuffer: 5 * time.Minute}}

	err := client.refreshTokenIfNeeded()

	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func expiredTokenClient(serverURL string) *Client {
	return &Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: serverURL}},
		config:     Config{WarehouseNumber: "847", TokenRefreshBuffer: 5 * time.Minute},
		token: &TokenResponse{
			IDToken:      generateTestJWT(time.Now().Add(-1 * time.Hour).Unix()),
			RefreshToken: "revoked-refresh-token",
		},
		tokenExpiry: time.Now().Add(-1 * time.Hour),
	}
}

func TestRefreshToken_RejectedGrantIsNotAuthenticated(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"AADB2C90080: The provided grant has expired."}`))
	}))
	defer server.Close()

	_, err := expiredTokenClient(server.URL).GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestRefreshToken_ServerOutageIsNotAnAuthFailure(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := expiredTokenClient(server.URL).GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotAuthenticated, "an outage should be retried, not trigger a new sign-in")
}

func TestExecuteGraphQL_UnauthorizedIsNotAuthenticated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := seedValidToken(&Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: server.URL}},
		config:     Config{WarehouseNumber: "847", TokenRefreshBuffer: 5 * time.Minute},
	})

	_, err := client.GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestRefreshToken_BeingRateLimitedIsNotAnAuthFailure(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	for _, status := range []int{http.StatusTooManyRequests, http.StatusRequestTimeout} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(status)
		}))

		_, err := expiredTokenClient(server.URL).GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)
		server.Close()

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotAuthenticated, "status %d says nothing about the refresh token", status)
		assert.False(t, isFatal(err), "status %d should be retried", status)
	}
}

func TestRefreshToken_KeepsTheRefreshTokenWhenTheResponseHasNone(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	refreshExpiry := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	require.NoError(t, SaveTokens(&StoredTokens{RefreshToken: "revoked-refresh-token", RefreshTokenExpiresAt: refreshExpiry}))

	newIDToken := generateTestJWT(time.Now().Add(time.Hour).Unix())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": newIDToken})
	}))
	defer server.Close()

	client := expiredTokenClient(server.URL)
	require.NoError(t, client.refreshToken())

	assert.Equal(t, newIDToken, client.token.IDToken)
	assert.Equal(t, "revoked-refresh-token", client.token.RefreshToken, "the token in use is still the only way to refresh")

	saved, err := LoadTokens()
	require.NoError(t, err)
	assert.Equal(t, newIDToken, saved.IDToken)
	assert.Equal(t, "revoked-refresh-token", saved.RefreshToken)
	assert.True(t, saved.RefreshTokenExpiresAt.Equal(refreshExpiry), "an unchanged refresh token keeps its expiry, got %s", saved.RefreshTokenExpiresAt)
}

func TestRefreshToken_ResponseWithoutAnIDTokenIsAnError(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := expiredTokenClient(server.URL)
	previous := client.token.IDToken
	err := client.refreshToken()

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotAuthenticated)
	assert.Equal(t, previous, client.token.IDToken, "an empty response must not replace the tokens in use")
	saved, loadErr := LoadTokens()
	require.NoError(t, loadErr)
	assert.Nil(t, saved, "nothing is written to disk")
}

// refusingAPI answers GraphQL requests with 401 unless they carry accept as
// their bearer token, and hands out freshIDToken at the token endpoint.
type refusingAPI struct {
	accept       string
	freshIDToken string
	refreshCode  int

	graphQLCalls int
	refreshCalls int
	bearers      []string
}

func (a *refusingAPI) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
			a.refreshCalls++
			if a.refreshCode != 0 {
				w.WriteHeader(a.refreshCode)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TokenResponse{
				IDToken:               a.freshIDToken,
				RefreshToken:          "rotated-refresh-token",
				RefreshTokenExpiresIn: 7776000,
			})
			return
		}

		a.graphQLCalls++
		bearer := strings.TrimPrefix(r.Header.Get(HeaderAuthorization), "Bearer ")
		a.bearers = append(a.bearers, bearer)
		if bearer != a.accept {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeGraphQLData(w, map[string]interface{}{
			"getOnlineOrders": []OnlineOrdersResponse{{TotalNumberOfRecords: 1, BCOrders: []OnlineOrder{{OrderNumber: "ORD-001"}}}},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestExecuteGraphQL_RefreshesOnceWhenTheIDTokenIsRefused(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	fresh := generateTestJWT(time.Now().Add(2 * time.Hour).Unix())
	api := &refusingAPI{accept: fresh, freshIDToken: fresh}
	// The local clock still considers this token valid; Costco does not.
	client := seedValidToken(&Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: api.start(t).URL}},
		config:     Config{WarehouseNumber: "847", TokenRefreshBuffer: 5 * time.Minute},
	})

	orders, err := client.GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.NoError(t, err, "the refresh token is still good, so no new sign-in is needed")
	require.Len(t, orders.BCOrders, 1)
	assert.Equal(t, 1, api.refreshCalls)
	require.Len(t, api.bearers, 2)
	assert.Equal(t, fresh, api.bearers[1], "the request is repeated with the new ID token")
}

func TestExecuteGraphQL_StillRefusedAfterARefreshIsNotAuthenticated(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	api := &refusingAPI{accept: "never issued", freshIDToken: generateTestJWT(time.Now().Add(2 * time.Hour).Unix())}
	client := seedValidToken(&Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: api.start(t).URL}},
		config:     Config{WarehouseNumber: "847", TokenRefreshBuffer: 5 * time.Minute},
	})

	_, err := client.GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.ErrorIs(t, err, ErrNotAuthenticated)
	assert.Equal(t, 1, api.refreshCalls, "one refresh is tried, not a loop")
	assert.Equal(t, 2, api.graphQLCalls)
}

func TestExecuteGraphQL_RefusedDuringARefreshOutageIsNotAnAuthFailure(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()

	api := &refusingAPI{accept: "never issued", refreshCode: http.StatusServiceUnavailable}
	client := seedValidToken(&Client{
		httpClient: &http.Client{Transport: &testTransport{baseURL: api.start(t).URL}},
		config:     Config{WarehouseNumber: "847", TokenRefreshBuffer: 5 * time.Minute},
	})

	_, err := client.GetOnlineOrders(context.Background(), "2025-01-01", "2025-01-31", 1, 10)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotAuthenticated, "the sign-in may be fine; the refresh just could not be checked")
}
