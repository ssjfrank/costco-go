package costco

import (
	"context"
	"net/http"
	"net/http/httptest"
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
