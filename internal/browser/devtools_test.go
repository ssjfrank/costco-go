package browser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDevToolsLine(t *testing.T) {
	address, ok := parseDevToolsLine("DevTools listening on ws://127.0.0.1:41119/devtools/browser/7d17-64")

	require.True(t, ok)
	assert.Equal(t, "ws://127.0.0.1:41119/devtools/browser/7d17-64", address)
}

func TestParseDevToolsLine_IgnoresOtherOutput(t *testing.T) {
	_, ok := parseDevToolsLine("[1234:5678:ERROR:gpu_init.cc(523)] Passthrough is not supported")

	assert.False(t, ok)
}

func TestParseDevToolsActivePort(t *testing.T) {
	address, err := parseDevToolsActivePort("41119\n/devtools/browser/7d17-64\n")

	require.NoError(t, err)
	assert.Equal(t, "ws://127.0.0.1:41119/devtools/browser/7d17-64", address)
}

func TestParseDevToolsActivePort_Malformed(t *testing.T) {
	for _, content := range []string{"", "41119", "not-a-port\n/devtools/browser/x", "41119\n/elsewhere"} {
		_, err := parseDevToolsActivePort(content)
		assert.Error(t, err, "content %q should be rejected", content)
	}
}
