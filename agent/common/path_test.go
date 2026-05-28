package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde relative path", in: "~/x/y", want: filepath.Join(home, "x/y")},
		{name: "absolute path", in: "/abs/path", want: "/abs/path"},
		{name: "tilde only unchanged", in: "~", want: "~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandHome(tt.in))
		})
	}
}

func TestValidateExistingDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ValidateExistingDir(dir))

	require.Error(t, ValidateExistingDir(filepath.Join(dir, "missing")))
}
