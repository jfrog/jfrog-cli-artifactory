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
	callCount    int
	lastReq      services.ComponentResolutionRequest
	resp         services.ComponentResolutionResponse
	disabled     bool
	version      string
	versionErr   error
	remediateErr error
}

func (m *mockClient) GetVersion() (string, error) {
	if m.versionErr != nil {
		return "", m.versionErr
	}
	if m.version != "" {
		return m.version, nil
	}
	return ZeroTouchRemediationMinXrayVersion, nil
}

func (m *mockClient) ZeroTouchRemediation(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	m.callCount++
	m.lastReq = req
	if m.remediateErr != nil {
		return nil, false, m.remediateErr
	}
	return &m.resp, m.disabled, nil
}

type mockTool struct {
	name         string
	commands     []string
	root         string
	lockfiles    []Lockfile
	ensureErr    error
	discoverErr  error
	bootstrapped []string
	discoverRoot *string
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
	return m.bootstrapped, nil
}
func (m mockTool) DiscoverLockfiles(projectRoot string) ([]Lockfile, error) {
	if m.discoverRoot != nil {
		*m.discoverRoot = projectRoot
	}
	if m.discoverErr != nil {
		return nil, m.discoverErr
	}
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
	t.Run("enabled for ParseBool truthy values", func(t *testing.T) {
		for _, v := range []string{"TRUE", "True", "1", "t", "T"} {
			t.Setenv(ZtrComponentsEnabledEnvVar, v)
			assert.True(t, IsComponentResolutionEnabled(), v)
		}
	})
	t.Run("not enabled for other values", func(t *testing.T) {
		t.Setenv(ZtrComponentsEnabledEnvVar, "false")
		assert.False(t, IsComponentResolutionEnabled())
		t.Setenv(ZtrComponentsEnabledEnvVar, "yes")
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

func TestRunIfEnabled_DiscoversLockfilesFromProjectRoot(t *testing.T) {
	enableZTR(t)
	workingDir := filepath.Join(t.TempDir(), "packages", "app")
	projectRoot := t.TempDir()
	var discoveredFrom string
	tool := mockTool{root: projectRoot, discoverRoot: &discoveredFrom}

	_, _, err := RunIfEnabled(context.Background(), &mockClient{}, "npm-virtual", tool, "install", workingDir, nil)

	require.NoError(t, err)
	assert.Equal(t, projectRoot, discoveredFrom)
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

func TestRunIfEnabled_SkipsEnsureLockfilesError(t *testing.T) {
	enableZTR(t)
	client := &mockClient{}
	tool := mockTool{ensureErr: errors.New("package-lock.json is required for npm ci")}
	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "ci", t.TempDir(), nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_SkipsGetVersionError(t *testing.T) {
	enableZTR(t)
	client := &mockClient{versionErr: errors.New("xray unavailable")}
	tool := mockTool{root: t.TempDir(), lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}
	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", t.TempDir(), nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)
}

func TestRunIfEnabled_SkipsRemediateError(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("orig"), 0644))

	client := &mockClient{remediateErr: errors.New("connection refused")}
	tool := mockTool{root: dir, lockfiles: []Lockfile{{Path: "package-lock.json", Content: []byte("orig")}}}
	_, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 1, client.callCount)

	data, readErr := os.ReadFile(lockPath)
	require.NoError(t, readErr)
	assert.Equal(t, "orig", string(data))
}

func TestRunIfEnabled_DiscoverErrorRestoresBootstrapped(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("generated"), 0644))

	client := &mockClient{}
	tool := mockTool{
		root:         dir,
		discoverErr:  errors.New("discover failed"),
		bootstrapped: []string{"package-lock.json"},
	}
	restore, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.callCount)
	assert.FileExists(t, lockPath)
	require.NoError(t, restore())
	assert.NoFileExists(t, lockPath)
}

func TestRunIfEnabled_RemediateErrorRestoresBootstrapped(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("generated"), 0644))

	client := &mockClient{remediateErr: errors.New("connection refused")}
	tool := mockTool{
		root:         dir,
		lockfiles:    []Lockfile{{Path: "package-lock.json", Content: []byte("generated")}},
		bootstrapped: []string{"package-lock.json"},
	}
	restore, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.FileExists(t, lockPath)
	require.NoError(t, restore())
	assert.NoFileExists(t, lockPath)
}

func TestRunIfEnabled_ApplyErrorRestoresLeftoverBootstrapped(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	changedPath := filepath.Join(dir, "package-lock.json")
	leftoverPath := filepath.Join(dir, "npm-shrinkwrap.json")
	require.NoError(t, os.WriteFile(changedPath, []byte("generated-a"), 0644))
	require.NoError(t, os.WriteFile(leftoverPath, []byte("generated-b"), 0644))

	origWrite := writeFile
	t.Cleanup(func() { writeFile = origWrite })
	writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("disk full")
	}

	client := &mockClient{resp: services.ComponentResolutionResponse{
		Lockfile: "remediated-a",
		Changes:  []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}},
	}}
	tool := mockTool{
		root: dir,
		lockfiles: []Lockfile{
			{Path: "package-lock.json", Content: []byte("generated-a")},
		},
		bootstrapped: []string{"package-lock.json", "npm-shrinkwrap.json"},
	}
	restore, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.FileExists(t, leftoverPath)
	require.NoError(t, restore())
	assert.NoFileExists(t, leftoverPath)
	assert.NoFileExists(t, changedPath)
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

func TestApplyLockfiles_RestoresWhenWriteTruncatesThenFails(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("original-lock"), 0644))

	origWrite := writeFile
	t.Cleanup(func() { writeFile = origWrite })
	writeFile = func(name string, data []byte, perm os.FileMode) error {
		if err := origWrite(name, nil, perm); err != nil {
			return err
		}
		return errors.New("disk full")
	}

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "package-lock.json", Content: []byte("remediated")},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, restore)
	assert.Contains(t, err.Error(), "disk full")

	data, readErr := os.ReadFile(lockPath)
	require.NoError(t, readErr)
	assert.Equal(t, "original-lock", string(data))
}

func TestApplyLockfiles_RollsBackOnLaterWriteFailure(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(firstPath, []byte("original-first"), 0644))
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not-a-dir"), 0644))

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "package-lock.json", Content: []byte("remediated-first")},
		{Path: "blocker/nested.lock", Content: []byte("remediated-second")},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, restore)

	data, readErr := os.ReadFile(firstPath)
	require.NoError(t, readErr)
	assert.Equal(t, "original-first", string(data))
}

func TestApplyLockfiles_RejectsPathEscapingProjectRoot(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "escaped.lock")
	t.Cleanup(func() { _ = os.Remove(outside) })

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "../escaped.lock", Content: []byte("pwned")},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, restore)
	assert.NoFileExists(t, outside)
}

func TestRunIfEnabled_RestoresBootstrappedLockfileWhenNoChanges(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.WriteFile(lockPath, []byte("generated"), 0644))

	client := &mockClient{resp: services.ComponentResolutionResponse{Lockfile: "generated", Changes: nil}}
	tool := mockTool{
		root:         dir,
		lockfiles:    []Lockfile{{Path: "package-lock.json", Content: []byte("generated")}},
		bootstrapped: []string{"package-lock.json"},
	}

	restore, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.FileExists(t, lockPath)
	require.NoError(t, restore())
	assert.NoFileExists(t, lockPath)
}

func TestRunIfEnabled_RestoresUnchangedBootstrappedLockfile(t *testing.T) {
	enableZTR(t)
	dir := t.TempDir()
	changedPath := filepath.Join(dir, "package-lock.json")
	unchangedPath := filepath.Join(dir, "npm-shrinkwrap.json")
	require.NoError(t, os.WriteFile(changedPath, []byte("generated-a"), 0644))
	require.NoError(t, os.WriteFile(unchangedPath, []byte("generated-b"), 0644))

	client := &selectingClient{
		resps: map[string]services.ComponentResolutionResponse{
			"generated-a": {Lockfile: "remediated-a", Changes: []services.Change{{Package: "lodash", BeforeIntegrity: "a", AfterIntegrity: "b"}}},
			"generated-b": {Lockfile: "generated-b"},
		},
	}
	tool := mockTool{
		root: dir,
		lockfiles: []Lockfile{
			{Path: "package-lock.json", Content: []byte("generated-a")},
			{Path: "npm-shrinkwrap.json", Content: []byte("generated-b")},
		},
		bootstrapped: []string{"package-lock.json", "npm-shrinkwrap.json"},
	}

	restore, remediated, err := RunIfEnabled(context.Background(), client, "npm-virtual", tool, "install", dir, nil)
	require.NoError(t, err)
	assert.True(t, remediated)
	assert.Equal(t, []byte("remediated-a"), mustRead(t, changedPath))
	assert.Equal(t, []byte("generated-b"), mustRead(t, unchangedPath))
	require.NoError(t, restore())
	assert.NoFileExists(t, changedPath)
	assert.NoFileExists(t, unchangedPath)
}

type selectingClient struct {
	resps map[string]services.ComponentResolutionResponse
}

func (s *selectingClient) GetVersion() (string, error) {
	return ZeroTouchRemediationMinXrayVersion, nil
}

func (s *selectingClient) ZeroTouchRemediation(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	resp := s.resps[req.Lockfile]
	return &resp, false, nil
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestApplyLockfiles_RejectsSymlinkParentDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "package-lock.json"), []byte("secret"), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "module")))

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: filepath.Join("module", "package-lock.json"), Content: []byte("remediated")},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, restore)

	data, readErr := os.ReadFile(filepath.Join(outside, "package-lock.json"))
	require.NoError(t, readErr)
	assert.Equal(t, "secret", string(data))
}

func TestApplyLockfiles_RejectsSymlinkLockfile(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "target.lock")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0644))
	lockPath := filepath.Join(dir, "package-lock.json")
	require.NoError(t, os.Symlink(outside, lockPath))

	restore, err := ApplyLockfiles(dir, []Lockfile{
		{Path: "package-lock.json", Content: []byte("remediated")},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, restore)

	data, readErr := os.ReadFile(outside)
	require.NoError(t, readErr)
	assert.Equal(t, "secret", string(data))
}
