package costco

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenFileEnv_RedirectsSaveLoadAndClear(t *testing.T) {
	dir := t.TempDir()
	defaultDir := filepath.Join(dir, "home", ".costco")
	tokenPath := filepath.Join(dir, "secrets", "tokens.json")
	t.Setenv("COSTCO_TEST_CONFIG_PATH", defaultDir)
	t.Setenv(TokenFileEnv, tokenPath)

	require.NoError(t, SaveTokens(&StoredTokens{IDToken: "id", RefreshToken: "refresh"}))

	_, err := os.Stat(tokenPath)
	require.NoError(t, err, "tokens should be written to the file the variable names")
	_, err = os.Stat(defaultDir)
	assert.True(t, os.IsNotExist(err), "a container's home may be read-only, so ~/.costco must not be touched")

	loaded, err := LoadTokens()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "refresh", loaded.RefreshToken)

	path, err := TokenFilePath()
	require.NoError(t, err)
	assert.Equal(t, tokenPath, path)
	assert.Contains(t, GetConfigInfo(), tokenPath)

	require.NoError(t, ClearTokens())
	_, err = os.Stat(tokenPath)
	assert.True(t, os.IsNotExist(err))
}

// Docker creates a directory in place of a bind-mounted file that does not
// exist yet, which otherwise surfaces as a baffling "is a directory".
func TestTokenFile_ExplainsADirectoryInItsPlace(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "tokens.json")
	require.NoError(t, os.Mkdir(tokenPath, 0o700))
	t.Setenv(TokenFileEnv, tokenPath)

	_, loadErr := LoadTokens()
	saveErr := SaveTokens(&StoredTokens{RefreshToken: "refresh"})

	for _, err := range []error{loadErr, saveErr} {
		require.Error(t, err)
		assert.Contains(t, err.Error(), tokenPath)
		assert.Contains(t, err.Error(), "is a directory")
		assert.Contains(t, err.Error(), "Docker")
	}
}

func TestTokenFilePath_DefaultsToConfigDirectory(t *testing.T) {
	cleanup := SetupTestConfig(t)
	defer cleanup()
	t.Setenv(TokenFileEnv, "")

	path, err := TokenFilePath()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(os.Getenv("COSTCO_TEST_CONFIG_PATH"), "tokens.json"), path)
}

// A container can mount the token file on its own. Docker cannot replace such a
// file, so saving must rewrite it rather than write a new file and rename it.
func TestSaveTokens_RewritesTheFileInPlace(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "tokens.json")
	t.Setenv(TokenFileEnv, tokenPath)

	require.NoError(t, SaveTokens(&StoredTokens{RefreshToken: "first"}))
	before, err := os.Stat(tokenPath)
	require.NoError(t, err)

	require.NoError(t, SaveTokens(&StoredTokens{RefreshToken: "second"}))
	after, err := os.Stat(tokenPath)
	require.NoError(t, err)

	assert.True(t, os.SameFile(before, after), "the token file must keep its identity across saves")
	loaded, err := LoadTokens()
	require.NoError(t, err)
	assert.Equal(t, "second", loaded.RefreshToken)
}

func TestRefreshToken_KeepsWorkingWhenTheTokenFileIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	tokenPath := filepath.Join(t.TempDir(), "tokens.json")
	t.Setenv(TokenFileEnv, tokenPath)
	require.NoError(t, SaveTokens(&StoredTokens{RefreshToken: "old-refresh-token"}))
	require.NoError(t, os.Chmod(tokenPath, 0o400))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			IDToken:               generateTestJWT(time.Now().Add(time.Hour).Unix()),
			RefreshToken:          "new-refresh-token",
			RefreshTokenExpiresIn: 7776000,
		})
	}))
	defer server.Close()

	client := expiredTokenClient(server.URL)

	require.NoError(t, client.refreshToken(), "a read-only mount only loses the saved refresh, not the run")
	assert.Equal(t, "new-refresh-token", client.token.RefreshToken)

	onDisk, err := LoadTokens()
	require.NoError(t, err)
	assert.Equal(t, "old-refresh-token", onDisk.RefreshToken)
}
