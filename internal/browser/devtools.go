package browser

import (
	"fmt"
	"strconv"
	"strings"
)

const devToolsBanner = "DevTools listening on "

// parseDevToolsLine extracts the browser's DevTools WebSocket address from the
// line Chrome prints to stderr at startup.
func parseDevToolsLine(line string) (string, bool) {
	index := strings.Index(line, devToolsBanner)
	if index < 0 {
		return "", false
	}
	address := strings.TrimSpace(line[index+len(devToolsBanner):])
	if !strings.HasPrefix(address, "ws://") {
		return "", false
	}
	return address, true
}

// parseDevToolsActivePort builds the DevTools WebSocket address from the
// DevToolsActivePort file Chrome writes into its profile directory: the port on
// the first line and the browser endpoint path on the second.
func parseDevToolsActivePort(content string) (string, error) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("DevToolsActivePort is incomplete: %q", content)
	}

	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port <= 0 || port > 65535 {
		return "", fmt.Errorf("DevToolsActivePort has an invalid port: %q", lines[0])
	}

	path := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(path, "/devtools/browser/") {
		return "", fmt.Errorf("DevToolsActivePort has an unexpected endpoint: %q", path)
	}

	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, path), nil
}
