package zerotouchremediation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testsutils "github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/jfrog/jfrog-client-go/xray/services"
)

type mockClient struct {
	callCount  int
	lastReq    services.ComponentResolutionRequest
	resp       services.ComponentResolutionResponse
	disabled   bool
	version    string
	versionErr error
}

func (m *mockClient) GetVersion() (string, error) {
	if m.versionErr != nil {
		return "", m.versionErr
	}
	if m.version != "" {
		return m.version, nil
	}
	return ZeroTouchRemediationMinVersion, nil
}

func (m *mockClient) ZeroTouchRemediation(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	m.callCount++
	m.lastReq = req
	return &m.resp, m.disabled, nil
}

type mockTool struct {
	name      string
	commands  []string
	root      string
	lockfiles []Lockfile
	ensureErr error
}

func (m mockTool) ToolName() string {
	if m.name != "" {
		return m.name
	}
	return "npm"
}
func (m mockTool) RelevantCommands() []string {
	if len(m.commands) > 0 {
		return m.commands
	}
	return []string{"install", "ci"}
}
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

func enableZTR(t *testing.T) {
	t.Helper()
	t.Setenv(ZtrComponentsEnabledEnvVar, "true")
}

func TestIsComponentResolutionEnabled(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv(ZtrComponentsEnabledEnvVar, "")
		assert.False(t, IsComponentResolutionEnabled())
	})
	t.Run("enabled when env true", func(t *testing.T) {
		t.Setenv(ZtrComponentsEnabledEnvVar, "true")
		assert.True(t, IsComponentResolutionEnabled())
	})
	t.Run("not enabled for other values", func(t *testing.T) {
		t.Setenv(ZtrComponentsEnabledEnvVar, "false")
		assert.False(t, IsComponentResolutionEnabled())
	})
}

func TestRunIfEnabled_WritesRemediatedLockfiles(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{
		Lockfile: "remediated",
		Changes:  []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
	}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.True(t, remediated)
	assert.Equal(t, 1, client.callCount)
	assert.Equal(t, "npm", client.lastReq.BuildTool)
	assert.Equal(t, "npm-virtual", client.lastReq.Repo)
	assert.Equal(t, "orig", client.lastReq.Lockfile)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "remediated", string(data))
}

func TestRunIfEnabled_WritesRemediatedNpmLockAsString(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	orig := `{"lockfileVersion":3,"name":"app"}`
	require.NoError(t, os.WriteFile(lockPath, []byte(orig), 0644))

	remediated := `{"lockfileVersion":3,"name":"app","remediated":true}`
	client := &mockClient{resp: services.ComponentResolutionResponse{
		Lockfile: remediated,
		Changes:  []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
	}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte(orig)}}}

	_, remediatedFlag, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.True(t, remediatedFlag)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.JSONEq(t, remediated, string(data))
	assert.Equal(t, orig, client.lastReq.Lockfile)
}

func TestRunIfEnabled_SkipsWhenServiceDisabled(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{disabled: true}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 1, client.callCount)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "orig", string(data))
}

func TestRunIfEnabled_SkipsWhenNotEnabled(t *testing.T) {
	t.Setenv(ZtrComponentsEnabledEnvVar, "")
	client := &mockClient{}
	tool := mockTool{}
	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", t.TempDir(), nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_SkipsWhenNoChanges(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{
		Lockfile: "orig",
		Changes:  nil,
	}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "orig", string(data))
}

func TestRunIfEnabled_LoopsPerDiscoveredFile(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.lock"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.lock"), []byte("b"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{Lockfile: "x", Changes: nil}}
	tool := mockTool{root: dir, lockfiles: []Lockfile{
		{Path: "a.lock", Content: []byte("a")},
		{Path: "b.lock", Content: []byte("b")},
	}}
	_, _, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, client.callCount)
}

func TestRunIfEnabled_SkipsIrrelevantCommand(t *testing.T) {
	enableZTR(t)
	client := &mockClient{}
	tool := mockTool{}
	_, _, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "version", t.TempDir(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_PropagatesEnsureLockfilesError(t *testing.T) {
	enableZTR(t)
	client := &mockClient{}
	tool := mockTool{ensureErr: errors.New("package-lock.json is required for npm ci")}
	_, _, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "ci", t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package-lock.json")
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_PropagatesGetVersionError(t *testing.T) {
	enableZTR(t)
	client := &mockClient{versionErr: errors.New("xray unavailable")}
	tool := mockTool{root: t.TempDir(), lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}
	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xray unavailable")
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_SkipsWhenXrayVersionTooLow(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{
		version: "3.100.0",
		resp: services.ComponentResolutionResponse{
			Lockfile: "remediated",
			Changes:  []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
		},
	}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "orig", string(data))
}

func TestRunIfEnabled_AllowsXrayDevVersion(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{
		version: "3.x-dev",
		resp: services.ComponentResolutionResponse{
			Lockfile: "remediated",
			Changes:  []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
		},
	}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}

	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.True(t, remediated)
	assert.Equal(t, 1, client.callCount)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	assert.Equal(t, "remediated", string(data))
}

func TestApplyLockfiles_WritesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("a"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "app"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app/gradle.lockfile"), []byte("b"), 0644))

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "package-lock.json", Content: []byte("a-remediated")},
		{Path: "app/gradle.lockfile", Content: []byte("b-remediated")},
	}, nil)
	require.NoError(t, err)
	defer testsutils.RemoveAllAndAssert(t, dir)

	a, _ := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	b, _ := os.ReadFile(filepath.Join(dir, "app/gradle.lockfile"))
	assert.Equal(t, "a-remediated", string(a))
	assert.Equal(t, "b-remediated", string(b))

	require.NoError(t, restore())
}
