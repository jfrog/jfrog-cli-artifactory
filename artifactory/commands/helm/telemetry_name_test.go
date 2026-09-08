package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelmTelemetryCommandName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"registry", "rt_helm_registry"},
		{"repo", "rt_helm_repo"},
		{"push", "rt_helm_push"},
		{"pull", "rt_helm_pull"},
		{"fetch", "rt_helm_pull"},
		{"install", "rt_helm_install"},
		{"upgrade", "rt_helm_upgrade"},
		{"uninstall", "rt_helm_uninstall"},
		{"rm", "rt_helm_uninstall"},
		{"dependency", "rt_helm_dependency"},
		{"dep", "rt_helm_dependency"},
		{"", "rt_helm"},
		{"mychart", "rt_helm"},
		{"oci://example.com/repo", "rt_helm"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, helmTelemetryCommandName(tc.in))
		})
	}
}
