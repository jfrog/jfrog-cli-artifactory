package civcs

import (
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set CI env vars based on ciProps
			clearCIEnvVars(t)

			if tt.ciProps != "" {
				// Setup GitHub Actions environment
				t.Setenv("CI", "true")
				t.Setenv("GITHUB_ACTIONS", "true")
				t.Setenv("GITHUB_WORKFLOW", "test")
				t.Setenv("GITHUB_RUN_ID", "123")
				t.Setenv("GITHUB_REPOSITORY_OWNER", "myorg")
				t.Setenv("GITHUB_REPOSITORY", "myorg/myrepo")
			}

			result := MergeWithUserProps(tt.userProps)
			assert.Equal(t, tt.expected, result)
		})
	}
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

type gitStub struct {
	url      string
	revision string
	branch   string
	readErr  error
}

type fakeGitManager struct {
	gitStub
}

func (s *fakeGitManager) ReadConfig() error {
	return s.readErr
}

func (s *fakeGitManager) GetUrl() string {
	return s.url
}

func (s *fakeGitManager) GetRevision() string {
	return s.revision
}

func (s *fakeGitManager) GetBranch() string {
	return s.branch
}

func withStubGitManager(t *testing.T, stub gitStub) func() {
	t.Helper()
	originalFindDotGit := findDotGitFromDirFn
	originalNewGitManager := newGitManagerFn
	findDotGitFromDirFn = func(string) (string, error) {
		return "/tmp/test/.git", nil
	}
	newGitManagerFn = func(string) gitConfigReader {
		return &fakeGitManager{gitStub: stub}
	}
	return func() {
		findDotGitFromDirFn = originalFindDotGit
		newGitManagerFn = originalNewGitManager
	}
}

func withStubLocalGitProps(t *testing.T) func() {
	t.Helper()
	originalFindDotGit := findDotGitFromDirFn
	originalNewGitManager := newGitManagerFn
	findDotGitFromDirFn = func(string) (string, error) {
		return "/tmp/test/.git", nil
	}
	newGitManagerFn = func(string) gitConfigReader {
		return &fakeGitManager{gitStub: gitStub{
			url:      "https://git.local/repo.git",
			revision: "local-sha",
			branch:   "local-branch",
		}}
	}
	return func() {
		findDotGitFromDirFn = originalFindDotGit
		newGitManagerFn = originalNewGitManager
	}
}

func TestGetLocalGitVcsInfo(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "false")
	restore := withStubGitManager(t, gitStub{
		url:      "https://github.com/acme/service.git",
		revision: "abc123",
		branch:   "main",
	})
	defer restore()

	info, err := getLocalGitVcsInfo("services/app")
	assert.NoError(t, err)
	assert.Equal(t, "vcs.url=https://github.com/acme/service.git;vcs.revision=abc123;vcs.branch=main", BuildCIVcsPropsString(info))
}

func TestGetLocalGitVcsInfo_NoGitRepo(t *testing.T) {
	originalFindDotGit := findDotGitFromDirFn
	findDotGitFromDirFn = func(string) (string, error) {
		return "", nil
	}
	defer func() { findDotGitFromDirFn = originalFindDotGit }()

	info, err := getLocalGitVcsInfo("build/output")
	assert.NoError(t, err)
	assert.Equal(t, "", BuildCIVcsPropsString(info))
}

func TestDeriveSearchDirFromUploadPattern_RegexpFallsBackToCwd(t *testing.T) {
	got := DeriveSearchDirFromUploadPattern(`(release)/.*\.jar`, UploadPatternOptions{IsRegexp: true})
	assert.Equal(t, ".", got)
}

func TestDeriveSearchDirFromUploadPattern_WildcardPrefix(t *testing.T) {
	got := DeriveSearchDirFromUploadPattern("services/app/**/*.zip", UploadPatternOptions{})
	assert.Equal(t, "services/app", got)
}

func TestDeriveSearchDirFromUploadPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		expected string
	}{
		{pattern: "services/app/**/*.zip", expected: "services/app"},
		{pattern: "dist/*.zip", expected: "dist"},
		{pattern: "*.zip", expected: "."},
		{pattern: "build/output/file.jar", expected: "build/output"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			assert.Equal(t, tt.expected, DeriveSearchDirFromUploadPattern(tt.pattern, UploadPatternOptions{}))
		})
	}
}

func TestMergeWithUserAndDetectedProps_UsesSearchDirDirectly(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "false")
	clearCIEnvVars(t)

	calledDir := ""
	originalFindDotGit := findDotGitFromDirFn
	originalNewGitManager := newGitManagerFn
	findDotGitFromDirFn = func(startDir string) (string, error) {
		calledDir = startDir
		return "/tmp/test/.git", nil
	}
	newGitManagerFn = func(string) gitConfigReader {
		return &fakeGitManager{gitStub: gitStub{
			url: "https://github.com/acme/service.git", revision: "abc123", branch: "main",
		}}
	}
	defer func() {
		findDotGitFromDirFn = originalFindDotGit
		newGitManagerFn = originalNewGitManager
	}()

	MergeWithUserAndDetectedProps("qa=true", "/abs/project/root")
	assert.Equal(t, "/abs/project/root", calledDir)
}

func TestMergeWithUserAndDetectedProps_PreferenceOrder(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "false")
	clearCIEnvVars(t)
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKFLOW", "test")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_SHA", "ci-sha")
	t.Setenv("GITHUB_REF", "refs/heads/ci-branch")
	t.Setenv("GITHUB_REPOSITORY", "acme/repo")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "acme")
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")

	restore := withStubLocalGitProps(t)
	defer restore()

	got := MergeWithUserAndDetectedProps("vcs.revision=user-sha;qa=true", "src")
	assert.Contains(t, got, "vcs.revision=user-sha")
	assert.Contains(t, got, "vcs.branch=ci-branch")
	assert.Contains(t, got, "vcs.url=https://github.com/acme/repo")
	assert.NotContains(t, got, "vcs.revision=local-sha")
}

func TestMergeWithUserAndDetectedProps_LocalGitFallback(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "false")
	clearCIEnvVars(t)

	restore := withStubGitManager(t, gitStub{
		url:      "https://github.com/acme/service.git",
		revision: "abc123",
		branch:   "main",
	})
	defer restore()

	got := MergeWithUserAndDetectedProps("qa=true", "services/app")
	assert.Contains(t, got, "qa=true")
	assert.Contains(t, got, "vcs.url=https://github.com/acme/service.git")
	assert.Contains(t, got, "vcs.revision=abc123")
	assert.Contains(t, got, "vcs.branch=main")
}

func TestSetVcsPropsToConfig_LocalGitFallback(t *testing.T) {
	t.Setenv(CIVcsPropsDisabledEnvVar, "false")
	clearCIEnvVars(t)

	restore := withStubGitManager(t, gitStub{
		url: "https://github.com/acme/lib.git", revision: "abc123", branch: "main",
	})
	defer restore()

	vConfig := viper.New()
	SetVcsPropsToConfig(vConfig, "/tmp/project")

	assert.Equal(t, "https://github.com/acme/lib.git", vConfig.GetString(VcsUrlKey))
	assert.Equal(t, "abc123", vConfig.GetString(VcsRevisionKey))
	assert.Equal(t, "main", vConfig.GetString(VcsBranchKey))
}
