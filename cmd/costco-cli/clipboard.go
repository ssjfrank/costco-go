package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"time"
)

var errNoClipboard = errors.New("no clipboard tool available")

// readClipboard returns the text on the system clipboard using the tool each
// platform provides.
func readClipboard(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for _, command := range clipboardCommands(runtime.GOOS, os.Getenv) {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		output, err := exec.CommandContext(ctx, command[0], command[1:]...).Output()
		if err == nil {
			return string(output), nil
		}
	}
	return "", errNoClipboard
}

func clipboardCommands(goos string, getenv func(string) string) [][]string {
	switch goos {
	case "darwin":
		return [][]string{{"pbpaste"}}
	case "windows":
		return [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard -Raw"}}
	default:
		var commands [][]string
		if getenv("WAYLAND_DISPLAY") != "" {
			commands = append(commands, []string{"wl-paste", "--no-newline"})
		}
		if getenv("DISPLAY") != "" {
			commands = append(commands,
				[]string{"xclip", "-selection", "clipboard", "-o"},
				[]string{"xsel", "--clipboard", "--output"})
		}
		return commands
	}
}
