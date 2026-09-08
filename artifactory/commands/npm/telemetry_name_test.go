package npm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNpmTelemetryCommandName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"install", "rt_npm_install"},
		{"i", "rt_npm_install"},
		{"isntall", "rt_npm_install"},
		{"add", "rt_npm_install"},
		{"ci", "rt_npm_ci"},
		{"clean-install", "rt_npm_ci"},
		{"publish", "rt_npm_publish"},
		{"p", "rt_npm_publish"},
		{"run", "rt_npm_run"},
		{"run-script", "rt_npm_run"},
		{"view", "rt_npm_view"},
		{"show", "rt_npm_view"},
		{"info", "rt_npm_view"},
		{"version", "rt_npm_version"},
		{"config", "rt_npm_config"},
		{"c", "rt_npm_config"},
		{"test", "rt_npm_test"},
		{"dist-tag", "rt_npm_dist-tag"},
		{"dist-tags", "rt_npm_dist-tag"},
		{"create", "rt_npm_init"},
		{"x", "rt_npm_exec"},
		{"", "rt_npm"},
		{"   ", "rt_npm"},
		{"localhost:8083", "rt_npm"},
		{"repo21", "rt_npm"},
		{"https://example.com", "rt_npm"},
		{"@scope/pkg", "rt_npm"},
		{"not-a-verb", "rt_npm"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, npmTelemetryCommandName(tc.in))
		})
	}
}

func TestNewNpmCommandUsesSanitizedTelemetryName(t *testing.T) {
	cmd := NewNpmCommand("localhost:8083", false)
	assert.Equal(t, "localhost:8083", cmd.cmdName)
	assert.Equal(t, "rt_npm", cmd.CommandName())

	install := NewNpmCommand("install", true)
	assert.Equal(t, "install", install.cmdName)
	assert.Equal(t, "rt_npm_install", install.CommandName())
}
