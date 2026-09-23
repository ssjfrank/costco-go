// Package browser drives a locally installed Chromium-based browser (Chrome,
// Edge, Chromium or Brave) over the Chrome DevTools Protocol, just far enough to
// open a page and read one network response the page receives.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNoBrowser means no supported browser was found in the usual install locations.
var ErrNoBrowser = errors.New("no Chrome, Edge, Chromium or Brave installation found")

type candidate struct {
	path   string
	onPath bool
}

// FindExecutable returns the path of the first supported browser installed on
// this machine, preferring Chrome, then Edge, Chromium and Brave.
func FindExecutable() (string, error) {
	return findExecutable(runtime.GOOS, os.Getenv, exec.LookPath, isFile)
}

func findExecutable(goos string, getenv func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	for _, c := range candidates(goos, getenv) {
		if c.onPath {
			if path, err := lookPath(c.path); err == nil {
				return path, nil
			}
			continue
		}
		if exists(c.path) {
			return c.path, nil
		}
	}
	return "", ErrNoBrowser
}

func candidates(goos string, getenv func(string) string) []candidate {
	switch goos {
	case "darwin":
		apps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Chromium.app/Contents/MacOS/Chromium",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		roots := []string{"/Applications"}
		if home := getenv("HOME"); home != "" {
			roots = append(roots, filepath.Join(home, "Applications"))
		}

		var found []candidate
		for _, app := range apps {
			for _, root := range roots {
				found = append(found, candidate{path: filepath.Join(root, app)})
			}
		}
		return found

	case "windows":
		var bases []string
		for _, key := range []string{"LOCALAPPDATA", "PROGRAMFILES", "PROGRAMFILES(X86)"} {
			if base := getenv(key); base != "" {
				bases = append(bases, base)
			}
		}
		installs := [][]string{
			{"Google", "Chrome", "Application", "chrome.exe"},
			{"Microsoft", "Edge", "Application", "msedge.exe"},
			{"Chromium", "Application", "chrome.exe"},
			{"BraveSoftware", "Brave-Browser", "Application", "brave.exe"},
		}

		var found []candidate
		for _, install := range installs {
			for _, base := range bases {
				found = append(found, candidate{path: filepath.Join(append([]string{base}, install...)...)})
			}
		}
		return found

	default:
		names := []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"brave-browser",
		}
		found := make([]candidate, 0, len(names))
		for _, name := range names {
			found = append(found, candidate{path: name, onPath: true})
		}
		return found
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
