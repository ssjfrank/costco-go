package browser

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func onlyExisting(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, path := range paths {
		set[path] = true
	}
	return func(path string) bool { return set[path] }
}

func lookPathFinding(names map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := names[name]; ok {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestFindExecutable_LinuxSearchesPath(t *testing.T) {
	path, err := findExecutable("linux", envFrom(nil),
		lookPathFinding(map[string]string{"chromium": "/usr/bin/chromium"}), onlyExisting())

	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/chromium", path)
}

func TestFindExecutable_LinuxPrefersChrome(t *testing.T) {
	path, err := findExecutable("linux", envFrom(nil),
		lookPathFinding(map[string]string{
			"chromium":      "/usr/bin/chromium",
			"google-chrome": "/usr/bin/google-chrome",
		}), onlyExisting())

	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/google-chrome", path)
}

func TestFindExecutable_MacFallsBackToEdge(t *testing.T) {
	edge := "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"

	path, err := findExecutable("darwin", envFrom(map[string]string{"HOME": "/Users/me"}),
		lookPathFinding(nil), onlyExisting(edge))

	require.NoError(t, err)
	assert.Equal(t, edge, path)
}

func TestFindExecutable_MacChecksUserApplications(t *testing.T) {
	chrome := "/Users/me/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

	path, err := findExecutable("darwin", envFrom(map[string]string{"HOME": "/Users/me"}),
		lookPathFinding(nil), onlyExisting(chrome))

	require.NoError(t, err)
	assert.Equal(t, chrome, path)
}

func TestFindExecutable_WindowsPrefersPerUserChrome(t *testing.T) {
	env := envFrom(map[string]string{
		"LOCALAPPDATA":      "/local",
		"PROGRAMFILES":      "/pf",
		"PROGRAMFILES(X86)": "/pf86",
	})
	perUser := filepath.Join("/local", "Google", "Chrome", "Application", "chrome.exe")
	machine := filepath.Join("/pf", "Google", "Chrome", "Application", "chrome.exe")

	path, err := findExecutable("windows", env, lookPathFinding(nil), onlyExisting(perUser, machine))

	require.NoError(t, err)
	assert.Equal(t, perUser, path)
}

func TestFindExecutable_WindowsFallsBackToEdge(t *testing.T) {
	env := envFrom(map[string]string{"PROGRAMFILES(X86)": "/pf86"})
	edge := filepath.Join("/pf86", "Microsoft", "Edge", "Application", "msedge.exe")

	path, err := findExecutable("windows", env, lookPathFinding(nil), onlyExisting(edge))

	require.NoError(t, err)
	assert.Equal(t, edge, path)
}

func TestFindExecutable_NothingInstalled(t *testing.T) {
	_, err := findExecutable("linux", envFrom(nil), lookPathFinding(nil), onlyExisting())

	assert.True(t, errors.Is(err, ErrNoBrowser))
}
