package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/require"
)

// WithJfrogHome sets JFROG_CLI_HOME_DIR to a temp directory and returns that path.
func WithJfrogHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(coreutils.HomeDir, dir)
	return dir
}

// WriteAgentConfig writes agent-config.json under home/agents/.
func WriteAgentConfig(t *testing.T, home, body string) {
	t.Helper()
	path := filepath.Join(home, "agents", "agent-config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
