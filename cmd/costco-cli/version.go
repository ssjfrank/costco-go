package main

import (
	"fmt"

	"github.com/eshaffer321/costco-go/pkg/costco"
)

// commit is set by release builds: -ldflags "-X main.commit=<sha>".
var commit string

func versionString(commit string) string {
	if commit == "" {
		return fmt.Sprintf("costco-cli %s (local build)", costco.Version)
	}
	return fmt.Sprintf("costco-cli %s (commit %s)", costco.Version, commit)
}
