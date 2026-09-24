package main

import (
	"testing"

	"github.com/eshaffer321/costco-go/pkg/costco"
	"github.com/stretchr/testify/assert"
)

func TestVersionString_ReleaseBuild(t *testing.T) {
	assert.Equal(t, "costco-cli "+costco.Version+" (commit 2c606c5)", versionString("2c606c5"))
}

func TestVersionString_LocalBuild(t *testing.T) {
	assert.Equal(t, "costco-cli "+costco.Version+" (local build)", versionString(""))
}
