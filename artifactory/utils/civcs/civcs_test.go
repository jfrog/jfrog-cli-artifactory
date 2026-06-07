package civcs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCIVcsPropsString_UrlRevisionBranch(t *testing.T) {
	info := cienv.CIVcsInfo{
		Provider: "github", Org: "jfrog", Repo: "jfrog-cli",
		Url: "https://github.com/jfrog/jfrog-cli", Revision: "abc123", Branch: "main",
	}
	result := BuildCIVcsPropsString(info)
	assert.Contains(t, result, "vcs.url=https://github.com/jfrog/jfrog-cli")
	assert.Contains(t, result, "vcs.revision=abc123")
	assert.Contains(t, result, "vcs.branch=main")
	assert.True(t, strings.HasPrefix(result, "vcs.provider=github;vcs.org=jfrog;vcs.repo=jfrog-cli;"))
}

func TestGetCIVcsPropsString(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "not in CI",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "GitHub Actions with all fields",
			envVars: map[string]string{
				"CI":                      "true",
				"GITHUB_ACTIONS":          "true",
				"GITHUB_WORKFLOW":         "test",
				"GITHUB_RUN_ID":           "123",
				"GITHUB_REPOSITORY_OWNER": "myorg",
				"GITHUB_REPOSITORY":       "myorg/myrepo",
			},
			expected: "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name: "CI without GitHub Actions",
			envVars: map[string]string{
				"CI": "true",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear CI-related env vars
			clearCIEnvVars(t)

			// Set test env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			result := GetCIVcsPropsString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeWithUserProps(t *testing.T) {
	tests := []struct {
		name      string
		userProps string
		ciProps   string
		expected  string
	}{
		{
			name:      "no user props, no CI props",
			userProps: "",
			ciProps:   "",
			expected:  "",
		},
		{
			name:      "user props only, no CI",
			userProps: "foo=bar",
			ciProps:   "",
			expected:  "foo=bar",
		},
		{
			name:      "CI props only, no user props",
			userProps: "",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "both user and CI props",
			userProps: "foo=bar",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "foo=bar;vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "user already has vcs.provider - adds other CI props",
			userProps: "vcs.provider=custom",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=custom;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "user already has vcs.org - adds other CI props",
			userProps: "foo=bar;vcs.org=customorg",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "foo=bar;vcs.org=customorg;vcs.provider=github;vcs.repo=myrepo",
		},
		{
			name:      "user has all vcs props - no CI props added",
			userProps: "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo",
			ciProps:   "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
			expected:  "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo",
		},
	}

	nonGitDir := t.TempDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearCIEnvVars(t)

			if tt.ciProps != "" {
				setupGitHubActionsEnv(t)
			}

			result := MergeWithUserProps(tt.userProps, nonGitDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeVcsProps(t *testing.T) {
	info := cienv.CIVcsInfo{
		Provider: "github", Org: "myorg", Repo: "myrepo",
		Url: "https://github.com/myorg/myrepo", Revision: "abc123", Branch: "main",
	}

	tests := []struct {
		name      string
		userProps string
		expected  string
	}{
		{
			name:      "adds all props to empty user props",
			userProps: "",
			expected:  "vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo;vcs.url=https://github.com/myorg/myrepo;vcs.revision=abc123;vcs.branch=main",
		},
		{
			name:      "respects user precedence for provider",
			userProps: "vcs.provider=custom",
			expected:  "vcs.provider=custom;vcs.org=myorg;vcs.repo=myrepo;vcs.url=https://github.com/myorg/myrepo;vcs.revision=abc123;vcs.branch=main",
		},
		{
			name:      "respects user precedence for local git props",
			userProps: "vcs.url=custom;vcs.revision=sha;vcs.branch=dev",
			expected:  "vcs.url=custom;vcs.revision=sha;vcs.branch=dev;vcs.provider=github;vcs.org=myorg;vcs.repo=myrepo",
		},
		{
			name:      "no new props when user has everything",
			userProps: "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo;vcs.url=custom;vcs.revision=sha;vcs.branch=dev",
			expected:  "vcs.provider=custom;vcs.org=customorg;vcs.repo=customrepo;vcs.url=custom;vcs.revision=sha;vcs.branch=dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, MergeVcsProps(tt.userProps, info))
		})
	}
}

func TestMergeWithUserProps_LocalGitFallback(t *testing.T) {
	repoDir, expectedURL, expectedRev, _ := setupGitRepoFixture(t, "git_test_.git_suffix")
	nonGitDir := t.TempDir()

	testCases := []struct {
		name      string
		userProps string
		setupCI   bool
		searchDir string
		contains  []string
		excludes  []string
	}{
		{
			name:      "local git props when not in CI",
			userProps: "",
			setupCI:   false,
			searchDir: repoDir,
			contains:  []string{"vcs.url=" + expectedURL, "vcs.revision=" + expectedRev},
		},
		{
			name:      "CI props plus local git props",
			userProps: "",
			setupCI:   true,
			searchDir: repoDir,
			contains: []string{
				"vcs.provider=github", "vcs.org=myorg", "vcs.repo=myrepo",
				"vcs.url=" + expectedURL, "vcs.revision=" + expectedRev,
			},
		},
		{
			name:      "user local git props prevent duplicate git lookup values",
			userProps: "vcs.url=custom;vcs.revision=sha;vcs.branch=dev",
			setupCI:   false,
			searchDir: repoDir,
			contains:  []string{"vcs.url=custom", "vcs.revision=sha", "vcs.branch=dev"},
			excludes:  []string{"vcs.url=" + expectedURL, "vcs.revision=" + expectedRev},
		},
		{
			name:      "no git repo leaves user props unchanged",
			userProps: "foo=bar",
			setupCI:   false,
			searchDir: nonGitDir,
			contains:  []string{"foo=bar"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			clearCIEnvVars(t)
			if tt.setupCI {
				setupGitHubActionsEnv(t)
			}

			result := MergeWithUserProps(tt.userProps, tt.searchDir)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
			for _, excluded := range tt.excludes {
				assert.NotContains(t, result, excluded)
			}
		})
	}
}

func TestSetCIVcsPropsToConfig_LocalGitFallback(t *testing.T) {
	repoDir, expectedURL, expectedRev, expectedBranch := setupGitRepoFixture(t, "git_test_.git_suffix")

	clearCIEnvVars(t)
	setupGitHubActionsEnv(t)

	vConfig := viper.New()
	SetCIVcsPropsToConfig(vConfig, repoDir)

	assert.Equal(t, "github", vConfig.GetString(VcsProviderKey))
	assert.Equal(t, "myorg", vConfig.GetString(VcsOrgKey))
	assert.Equal(t, "myrepo", vConfig.GetString(VcsRepoKey))
	assert.Equal(t, expectedURL, vConfig.GetString(VcsUrlKey))
	assert.Equal(t, expectedRev, vConfig.GetString(VcsRevisionKey))
	if expectedBranch != "" {
		assert.Equal(t, expectedBranch, vConfig.GetString(VcsBranchKey))
	}
}

func TestSetCIVcsPropsToConfig_SkipsLocalGitWhenAlreadySet(t *testing.T) {
	clearCIEnvVars(t)
	setupGitHubActionsEnv(t)

	vConfig := viper.New()
	vConfig.Set(VcsUrlKey, "https://example.com/org/repo")
	vConfig.Set(VcsRevisionKey, "deadbeef")
	vConfig.Set(VcsBranchKey, "main")

	SetCIVcsPropsToConfig(vConfig, t.TempDir())

	assert.Equal(t, "github", vConfig.GetString(VcsProviderKey))
	assert.Equal(t, "https://example.com/org/repo", vConfig.GetString(VcsUrlKey))
	assert.Equal(t, "deadbeef", vConfig.GetString(VcsRevisionKey))
	assert.Equal(t, "main", vConfig.GetString(VcsBranchKey))
}

func TestSetCIVcsPropsToConfig_Disabled(t *testing.T) {
	clearCIEnvVars(t)
	setupGitHubActionsEnv(t)
	t.Setenv(CIVcsPropsDisabledEnvVar, "true")

	vConfig := viper.New()
	SetCIVcsPropsToConfig(vConfig, ".")

	assert.False(t, vConfig.IsSet(VcsProviderKey))
}

func TestDeriveSearchDirFromFileSpec(t *testing.T) {
	existingDir := t.TempDir()
	existingFile := filepath.Join(existingDir, "artifact.jar")
	require.NoError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	tests := []struct {
		name     string
		fileSpec *spec.File
		expected string
	}{
		{
			name:     "regexp pattern uses current directory",
			fileSpec: &spec.File{Pattern: "repo/.*", Regexp: "true"},
			expected: ".",
		},
		{
			name:     "wildcard uses prefix before wildcard",
			fileSpec: &spec.File{Pattern: "repo/path/*.jar"},
			expected: "repo/path",
		},
		{
			name:     "wildcard at start uses current directory",
			fileSpec: &spec.File{Pattern: "*.jar"},
			expected: ".",
		},
		{
			name:     "plain file path uses parent directory",
			fileSpec: &spec.File{Pattern: "repo/path/file.jar"},
			expected: "repo/path",
		},
		{
			name:     "existing directory path is preserved",
			fileSpec: &spec.File{Pattern: existingDir},
			expected: filepath.ToSlash(existingDir),
		},
		{
			name:     "existing file path uses parent directory",
			fileSpec: &spec.File{Pattern: existingFile},
			expected: filepath.ToSlash(existingDir),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, DeriveSearchDirFromFileSpec(tt.fileSpec))
		})
	}
}

func TestIsCIVcsPropsDisabled(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "")
	assert.False(t, IsCIVcsPropsDisabled())

	t.Setenv(CIVcsPropsDisabledEnvVar, "true")
	assert.True(t, IsCIVcsPropsDisabled())
}

func setupGitHubActionsEnv(t *testing.T) {
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKFLOW", "test")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "myorg")
	t.Setenv("GITHUB_REPOSITORY", "myorg/myrepo")
}

func setupGitRepoFixture(t *testing.T, fixtureName string) (repoDir, url, revision, branch string) {
	t.Helper()
	repoDir = t.TempDir()
	src := filepath.Join("..", "..", "commands", "testdata", fixtureName)
	dst := filepath.Join(repoDir, ".git")
	require.NoError(t, biutils.CopyDir(src, dst, true, nil))

	gitManager := clientutils.NewGitManager(repoDir)
	require.NoError(t, gitManager.ReadConfig())
	return repoDir, gitManager.GetUrl(), gitManager.GetRevision(), gitManager.GetBranch()
}

func clearCIEnvVars(t *testing.T) {
	envVars := []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITHUB_WORKFLOW",
		"GITHUB_RUN_ID",
		"GITHUB_REPOSITORY_OWNER",
		"GITHUB_REPOSITORY",
		"GITHUB_SERVER_URL",
		"GITHUB_SHA",
		"GITHUB_REF",
		"GITHUB_REF_NAME",
		"GITHUB_HEAD_REF",
		"GITLAB_CI",
		"CI_PROJECT_PATH",
	}
	for _, v := range envVars {
		t.Setenv(v, "")
	}
}
