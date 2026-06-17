package healcomponents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jfrog/jfrog-client-go/xray/services"
)

type mockClient struct {
	callCount int
	lastReq   services.ComponentResolutionRequest
	resp      services.ComponentResolutionResponse
}

func (m *mockClient) HealComponents(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, error) {
	m.callCount++
	m.lastReq = req
	return &m.resp, nil
}

type mockTool struct {
	root      string
	lockfiles []Lockfile
	ensureErr error
}

func (m mockTool) ToolName() string           { return "npm" }
func (m mockTool) RelevantCommands() []string { return []string{"install", "ci"} }
func (m mockTool) ProjectRoot(_ string) (string, error) {
	return m.root, nil
}
func (m mockTool) EnsureLockfiles(_ context.Context, _, _ string, _ CommandRunner, _ ...string) ([]string, error) {
	if m.ensureErr != nil {
		return nil, m.ensureErr
	}
	return nil, nil
}
func (m mockTool) DiscoverLockfiles(_ string) ([]Lockfile, error) {
	return m.lockfiles, nil
}

func TestIsComponentResolutionDisabled(t *testing.T) {
	t.Run("enabled by default", func(t *testing.T) {
		t.Setenv(ComponentResolutionDisabledEnvVar, "")
		assert.False(t, IsComponentResolutionDisabled())
	})
	t.Run("disabled when env true", func(t *testing.T) {
		t.Setenv(ComponentResolutionDisabledEnvVar, "true")
		assert.True(t, IsComponentResolutionDisabled())
	})
	t.Run("not disabled for other values", func(t *testing.T) {
		t.Setenv(ComponentResolutionDisabledEnvVar, "false")
		assert.False(t, IsComponentResolutionDisabled())
	})
}

func TestRunIfEnabled_WritesHealedLockfiles(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{
		Content: json.RawMessage(`"healed"`),
		Changes: []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
	}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, client.callCount)
	assert.Equal(t, "npm", client.lastReq.BuildTool)
	assert.Equal(t, "npm-virtual", client.lastReq.Repo)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, `"healed"`, string(data))
}

func TestRunIfEnabled_SkipsWhenDisabled(t *testing.T) {
	t.Setenv(ComponentResolutionDisabledEnvVar, "true")
	client := &mockClient{}
	tool := mockTool{}
	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_SkipsWhenNoChanges(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{
		Content: json.RawMessage(`"orig"`),
		Changes: nil,
	}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "orig", string(data))
}

func TestRunIfEnabled_LoopsPerDiscoveredFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.lock"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.lock"), []byte("b"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{Content: json.RawMessage(`"x"`), Changes: nil}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{
		{Path: "a.lock", Content: []byte("a")},
		{Path: "b.lock", Content: []byte("b")},
	}}
	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, client.callCount)
}

func TestRunIfEnabled_SkipsIrrelevantCommand(t *testing.T) {
	client := &mockClient{}
	tool := mockTool{}
	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "version", t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_PropagatesEnsureLockfilesError(t *testing.T) {
	client := &mockClient{}
	tool := mockTool{ensureErr: errors.New("package-lock.json is required for npm ci")}
	_, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "ci", t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package-lock.json")
	assert.Equal(t, 0, client.callCount)
}
