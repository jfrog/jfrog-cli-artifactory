package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withClaudePluginListJSON(t *testing.T, fn func() ([]byte, error)) {
	t.Helper()
	restore := claudePluginListJSON
	claudePluginListJSON = fn
	t.Cleanup(func() { claudePluginListJSON = restore })
}

func withCodexPluginListJSON(t *testing.T, fn func() ([]byte, error)) {
	t.Helper()
	restore := codexPluginListJSON
	codexPluginListJSON = fn
	t.Cleanup(func() { codexPluginListJSON = restore })
}

func TestIsRegisteredWithClaude_Present(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`[
  {"id": "jfrog-plugin-timepass@buk-plugins-2", "version": "1.0.1", "scope": "user", "enabled": true}
]`), nil
	})

	ok, err := isRegisteredWithClaude("jfrog-plugin-timepass", "buk-plugins-2")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsRegisteredWithClaude_AbsentAfterUninstall(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`[
  {"id": "jfrog-plugin-test@buk-plugins-2", "version": "1.0.0", "scope": "user", "enabled": true}
]`), nil
	})

	ok, err := isRegisteredWithClaude("jfrog-plugin-timepass", "buk-plugins-2")
	require.NoError(t, err)
	assert.False(t, ok, "a plugin removed by `claude plugin uninstall` must report as not registered")
}

func TestIsRegisteredWithClaude_EmptyListIsNotError(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`[]`), nil
	})

	ok, err := isRegisteredWithClaude("web", "repo")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestIsRegisteredWithClaude_CommandErrorReturnsError(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return nil, errors.New("exec: \"claude\": executable file not found in $PATH")
	})

	_, err := isRegisteredWithClaude("web", "repo")
	require.Error(t, err)
}

func TestIsRegisteredWithClaude_MalformedOutputReturnsError(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`not json`), nil
	})

	_, err := isRegisteredWithClaude("web", "repo")
	require.Error(t, err)
}

func TestIsRegisteredWithCodex_Present(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`{
  "installed": [
    {"pluginId": "jfrog-plugin-timepass@buk-plugins-2", "version": "1.0.1", "enabled": true}
  ],
  "available": []
}`), nil
	})

	ok, err := isRegisteredWithCodex("jfrog-plugin-timepass", "buk-plugins-2")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsRegisteredWithCodex_AbsentAfterRemove(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`{"installed": [], "available": []}`), nil
	})

	ok, err := isRegisteredWithCodex("jfrog-plugin-timepass", "buk-plugins-2")
	require.NoError(t, err,
		"a stale [plugins.\"...\"] entry left in config.toml after `codex plugin remove` "+
			"must not be read as registered — only the CLI's own installed list counts")
	assert.False(t, ok)
}

func TestIsRegisteredWithCodex_EmptyListIsNotError(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`{"installed": [], "available": []}`), nil
	})

	ok, err := isRegisteredWithCodex("web", "repo")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestIsRegisteredWithCodex_CommandErrorReturnsError(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return nil, errors.New("exec: \"codex\": executable file not found in $PATH")
	})

	_, err := isRegisteredWithCodex("web", "repo")
	require.Error(t, err)
}

func TestIsRegisteredWithCodex_MalformedOutputReturnsError(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`not json`), nil
	})

	_, err := isRegisteredWithCodex("web", "repo")
	require.Error(t, err)
}

func TestIsRegisteredWithNativeAgent_CursorAlwaysTrue(t *testing.T) {
	ok, err := IsRegisteredWithNativeAgent("cursor", "web", "repo")
	require.NoError(t, err)
	assert.True(t, ok, "agents with no native registry (cursor, --path) have nothing to invalidate against")
}

func TestIsRegisteredWithNativeAgent_DispatchesByAgentCaseInsensitive(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`[{"id": "web@repo", "version": "1.0.0"}]`), nil
	})

	ok, err := IsRegisteredWithNativeAgent("Claude", "web", "repo")
	require.NoError(t, err)
	assert.True(t, ok)
}
