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

func TestListNativePlugins_Claude(t *testing.T) {
	withClaudePluginListJSON(t, func() ([]byte, error) {
		return []byte(`[
  {"id": "gopls-lsp@claude-plugins-official", "version": "1.0.0", "scope": "user", "enabled": true, "installPath": "/home/u/.claude/plugins/cache/claude-plugins-official/gopls-lsp/1.0.0"},
  {"id": "jfrog-plugin-test@local", "version": "1.0.0", "scope": "user", "enabled": true, "installPath": "/home/u/.claude/plugins/cache/local/jfrog-plugin-test/1.0.0"}
]`), nil
	})

	plugins, err := ListNativePlugins("claude")
	require.NoError(t, err)
	require.Len(t, plugins, 2)
	assert.Equal(t, NativePluginInfo{
		Slug: "gopls-lsp", Repo: "claude-plugins-official", Version: "1.0.0",
		Path: "/home/u/.claude/plugins/cache/claude-plugins-official/gopls-lsp/1.0.0",
	}, plugins[0])
	assert.Equal(t, NativePluginInfo{
		Slug: "jfrog-plugin-test", Repo: "local", Version: "1.0.0",
		Path: "/home/u/.claude/plugins/cache/local/jfrog-plugin-test/1.0.0",
	}, plugins[1])
}

func TestListNativePlugins_Codex(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`{
  "installed": [
    {"pluginId": "jfrog-plugin-timepass@buk-plugins-2", "name": "jfrog-plugin-timepass", "marketplaceName": "buk-plugins-2", "version": "1.0.2", "source": {"source": "local", "path": "/home/u/.agents/marketplaces/buk-plugins-2/plugins/jfrog-plugin-timepass"}}
  ],
  "available": []
}`), nil
	})

	plugins, err := ListNativePlugins("codex")
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.Equal(t, NativePluginInfo{
		Slug: "jfrog-plugin-timepass", Repo: "buk-plugins-2", Version: "1.0.2",
		Path: "/home/u/.agents/marketplaces/buk-plugins-2/plugins/jfrog-plugin-timepass",
	}, plugins[0])
}

func TestListNativePlugins_CodexFallsBackToPluginIDWhenNameFieldsMissing(t *testing.T) {
	withCodexPluginListJSON(t, func() ([]byte, error) {
		return []byte(`{"installed": [{"pluginId": "web@repo", "version": "1.0.0"}], "available": []}`), nil
	})

	plugins, err := ListNativePlugins("codex")
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	assert.Equal(t, "web", plugins[0].Slug)
	assert.Equal(t, "repo", plugins[0].Repo)
}

func TestListNativePlugins_UnsupportedAgent(t *testing.T) {
	_, err := ListNativePlugins("cursor")
	require.Error(t, err)
}

func TestSplitPluginID(t *testing.T) {
	slug, repo := splitPluginID("web@repo")
	assert.Equal(t, "web", slug)
	assert.Equal(t, "repo", repo)

	slug, repo = splitPluginID("no-at-sign")
	assert.Equal(t, "no-at-sign", slug)
	assert.Empty(t, repo)
}
