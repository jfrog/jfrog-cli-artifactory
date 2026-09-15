package buildinfo

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

func (m *mockServicesManager) SetProps(params services.PropsParams) (int, error) {
	args := m.Called(params)
	return args.Int(0), args.Error(1)
}

func (m *mockServicesManager) SearchFiles(params services.SearchParams) (*content.ContentReader, error) {
	args := m.Called(params)
	reader, _ := args.Get(0).(*content.ContentReader)
	return reader, args.Error(1)
}

func createSearchReader(t *testing.T, jsonContent string) (*content.ContentReader, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-search-*.json")
	assert.NoError(t, err)
	_, err = tmpFile.WriteString(jsonContent)
	assert.NoError(t, err)
	assert.NoError(t, tmpFile.Close())
	filePath := tmpFile.Name()
	reader := content.NewContentReader(filePath, content.DefaultKey)
	return reader, func() {
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			assert.NoError(t, err)
		}
	}
}

func createTestSearchReader(t *testing.T) (*content.ContentReader, func()) {
	return createSearchReader(t, `{"results":[{"repo":"libs-release","path":"com/example","name":"file.jar","type":"file","size":0,"created":"","modified":""}]}`)
}

func createEmptySearchReader(t *testing.T) (*content.ContentReader, func()) {
	return createSearchReader(t, `{"results":[]}`)
}

func setUpCIGitHubEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKFLOW", "test")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "jfrog")
	t.Setenv("GITHUB_REPOSITORY", "jfrog/jfrog-cli")
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GITHUB_SHA", "")
	t.Setenv("GITHUB_REF", "")
	t.Setenv("GITHUB_REF_NAME", "")
	t.Setenv("GITHUB_HEAD_REF", "")
}

// TestSetVcsPropsOnArtifacts_AllPresent_DirectOnly: all artifacts have OriginalDeploymentRepo.
// Only the direct-path branch fires; build-search is never invoked.
func TestSetVcsPropsOnArtifacts_AllPresent_DirectOnly(t *testing.T) {
	setUpCIGitHubEnv(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()
	// Only pattern-based search calls (direct path) should appear.
	mockSM.On("SearchFiles", mock.MatchedBy(func(params services.SearchParams) bool {
		return params.Build == "" && params.Pattern != ""
	})).Return(searchReader, nil)
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(1, nil)

	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
					{Name: "file2.jar", Path: "com/example/file2.jar", OriginalDeploymentRepo: "libs-release"},
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	// Build-scoped SearchFiles must NOT have been called.
	for _, call := range mockSM.Calls {
		if call.Method == "SearchFiles" {
			params, ok := call.Arguments.Get(0).(services.SearchParams)
			require.True(t, ok, "unexpected type for SearchFiles argument")
			assert.Empty(t, params.Build, "build-search branch must not fire when all artifacts have OriginalDeploymentRepo")
		}
	}
}

// TestSetVcsPropsOnArtifacts_AllMissing_BuildSearchOnly: no artifact has OriginalDeploymentRepo.
// Only the build-search branch fires; no repo-listing, no direct pattern-search.
// PRIMARY regression guard for RTECO-2025.
func TestSetVcsPropsOnArtifacts_AllMissing_BuildSearchOnly(t *testing.T) {
	setUpCIGitHubEnv(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()
	// Only the build-artifacts API should be used; no AQL search.
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: searchReader}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(1, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					// No OriginalDeploymentRepo on either artifact (Gradle extractor scenario).
					{Name: "minimal-example-1.0.jar", Path: "com/example/minimal-example/1.0/minimal-example-1.0.jar"},
					{Name: "minimal-example-1.0.pom", Path: "com/example/minimal-example/1.0/minimal-example-1.0.pom"},
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, 1, *buildApiCalls, "exactly one build-artifacts API call expected regardless of artifact count")
	mockSM.AssertNotCalled(t, "SearchFiles")
}

// TestSetVcsPropsOnArtifacts_Mixed_BothPaths: some artifacts have OriginalDeploymentRepo, some don't.
// Both branches fire: direct for present, build-search for missing.
func TestSetVcsPropsOnArtifacts_Mixed_BothPaths(t *testing.T) {
	setUpCIGitHubEnv(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()
	searchReader2, cleanup2 := createTestSearchReader(t)
	defer cleanup2()

	// Direct-path call: pattern non-empty, Build empty.
	mockSM.On("SearchFiles", mock.MatchedBy(func(params services.SearchParams) bool {
		return params.Build == "" && params.Pattern != ""
	})).Return(searchReader, nil)
	// Build-search resolves through the build-artifacts API, not AQL.
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: searchReader2}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(1, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
					{Name: "missing.jar", Path: "com/example/missing.jar"}, // no repo
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, 1, *buildApiCalls, "missing-repo artifacts resolve through the build-artifacts API")
}

// TestSetVcsPropsOnArtifacts_Disabled: JFROG_CLI_CI_VCS_PROPS_DISABLED=true short-circuits everything.
func TestSetVcsPropsOnArtifacts_Disabled(t *testing.T) {
	t.Setenv("JFROG_CLI_CI_VCS_PROPS_DISABLED", "true")

	mockSM := new(mockServicesManager)
	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
			}},
		},
	}
	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(t.TempDir())
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertNotCalled(t, "SearchFiles")
	mockSM.AssertNotCalled(t, "SetProps")
}

// TestSetVcsPropsOnArtifacts_EmptyProps: non-CI non-git dir → empty props → early return.
func TestSetVcsPropsOnArtifacts_EmptyProps(t *testing.T) {
	// Unset all CI env vars so no CI props are collected.
	for _, v := range []string{
		"CI", "GITHUB_ACTIONS", "GITHUB_WORKFLOW", "GITHUB_RUN_ID",
		"GITHUB_REPOSITORY_OWNER", "GITHUB_REPOSITORY",
		"GITLAB_CI", "JENKINS_URL", "CIRCLECI", "TRAVIS",
	} {
		t.Setenv(v, "")
	}

	mockSM := new(mockServicesManager)
	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
			}},
		},
	}
	// Use a temp dir with no .git — GetCIVcsPropsString will return "" when no CI env and no git.
	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(t.TempDir())
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertNotCalled(t, "SearchFiles")
	mockSM.AssertNotCalled(t, "SetProps")
}

// TestSetVcsPropsOnArtifacts_ProjectPropagated: project key flows into the build spec.
func TestSetVcsPropsOnArtifacts_ProjectPropagated(t *testing.T) {
	setUpCIGitHubEnv(t)
	nonGitDir := t.TempDir()

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()

	// Capture what the build-artifacts API is asked for.
	var gotName, gotNumber, gotProject string
	originalResolver := resolveBuildArtifacts
	resolveBuildArtifacts = func(_ artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) (*content.ContentReader, error) {
		gotName, gotNumber, gotProject = buildName, buildNumber, project
		return searchReader, nil
	}
	t.Cleanup(func() { resolveBuildArtifacts = originalResolver })
	mockSM.On("SetProps", mock.Anything).Return(1, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar"}, // no repo → build-search branch
			}},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "myproject")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, "mybuild", gotName)
	assert.Equal(t, "42", gotNumber)
	assert.Equal(t, "myproject", gotProject, "project key must reach the build-artifacts API")
}

func TestPrintBuildInfoLink(t *testing.T) {
	timeNow := time.Now()
	buildTime := strconv.FormatInt(timeNow.UnixNano()/1000000, 10)
	var linkTypes = []struct {
		majorVersion  int
		buildTime     time.Time
		buildInfoConf *build.BuildConfiguration
		serverDetails config.ServerDetails
		expected      string
	}{
		// Test platform URL
		{5, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8081/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{6, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8081/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", ""),
			config.ServerDetails{Url: "http://localhost:8082/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=artifactory-build-info"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8082/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=cli-build-info&projectKey=cli"},

		// Test Artifactory URL
		{5, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8081/artifactory"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{6, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8081/artifactory/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", ""),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8082/artifactory"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=artifactory-build-info"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8082/artifactory/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=cli-build-info&projectKey=cli"},
	}

	for i := range linkTypes {
		buildPubConf := &BuildPublishCommand{
			linkTypes[i].buildInfoConf,
			&linkTypes[i].serverDetails,
			nil,
			true,
			nil,
			false,
			false,
			nil,
			"",
			false,
			false,
			BuildAddGitCommand{},
		}
		buildPubComService, err := buildPubConf.getBuildInfoUiUrl(linkTypes[i].majorVersion, linkTypes[i].buildTime)
		assert.NoError(t, err)
		assert.Equal(t, buildPubComService, linkTypes[i].expected)
	}
}

func TestCalculateBuildNumberFrequency(t *testing.T) {
	tests := []struct {
		name     string
		runs     *buildinfo.BuildRuns
		expected map[string]int
	}{
		{
			name: "Single build number",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{{Uri: "/1"}},
			},
			expected: map[string]int{"1": 1},
		},
		{
			name: "Single build number with special characters",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{{Uri: "/1-"}},
			},
			expected: map[string]int{"1-": 1},
		},
		{
			name: "Multiple build numbers",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{
					{Uri: "/1"},
					{Uri: "/2"},
					{Uri: "/1"},
				},
			},
			expected: map[string]int{"1": 2, "2": 1},
		},
		{
			name: "No build numbers",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{},
			},
			expected: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateBuildNumberFrequency(tt.runs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIs404Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "404 in message", err: errors.New("server returned 404"), expected: true},
		{name: "not found", err: errors.New("artifact not found"), expected: true},
		{name: "Not Found uppercase", err: errors.New("Not Found"), expected: true},
		{name: "500 error", err: errors.New("server returned 500"), expected: false},
		{name: "connection refused", err: errors.New("connection refused"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is404Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIs403Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "403 in message", err: errors.New("server returned 403"), expected: true},
		{name: "forbidden", err: errors.New("access forbidden"), expected: true},
		{name: "Forbidden uppercase", err: errors.New("Forbidden"), expected: true},
		{name: "500 error", err: errors.New("server returned 500"), expected: false},
		{name: "404 error", err: errors.New("not found 404"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is403Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCIVcsPropsString(t *testing.T) {
	tests := []struct {
		name     string
		info     cienv.CIVcsInfo
		expected string
	}{
		{
			name:     "all fields",
			info:     cienv.CIVcsInfo{Provider: "github", Org: "jfrog", Repo: "jfrog-cli"},
			expected: "vcs.provider=github;vcs.org=jfrog;vcs.repo=jfrog-cli",
		},
		{
			name:     "partial fields - provider and org",
			info:     cienv.CIVcsInfo{Provider: "github", Org: "jfrog"},
			expected: "vcs.provider=github;vcs.org=jfrog",
		},
		{
			name:     "only provider",
			info:     cienv.CIVcsInfo{Provider: "github"},
			expected: "vcs.provider=github",
		},
		{
			name:     "empty",
			info:     cienv.CIVcsInfo{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := civcs.BuildCIVcsPropsString(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExcludeDependenciesByScope(t *testing.T) {
	tests := []struct {
		name             string
		depExcludeScopes []string
		modules          []buildinfo.Module
		expectedDeps     [][]string
	}{
		{
			name:             "no scopes specified - all deps retained",
			depExcludeScopes: nil,
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep2"}},
		},
		{
			name:             "single scope exclusion",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
					{Id: "dep3", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep3"}},
		},
		{
			name:             "multiple scope exclusion",
			depExcludeScopes: []string{"test", "provided"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
					{Id: "dep3", Scopes: []string{"provided"}},
					{Id: "dep4", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep4"}},
		},
		{
			name:             "case insensitive matching",
			depExcludeScopes: []string{"Test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"test"}},
					{Id: "dep2", Scopes: []string{"TEST"}},
					{Id: "dep3", Scopes: []string{"Test"}},
					{Id: "dep4", Scopes: []string{"compile"}},
				}},
			},
			expectedDeps: [][]string{{"dep4"}},
		},
		{
			name:             "dependency with multiple scopes - excluded if any match",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile", "test"}},
					{Id: "dep2", Scopes: []string{"compile", "runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep2"}},
		},
		{
			name:             "dependencies with no scopes - never excluded",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: nil},
					{Id: "dep2", Scopes: []string{}},
					{Id: "dep3", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep2"}},
		},
		{
			name:             "multiple modules",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "mod1-dep1", Scopes: []string{"compile"}},
					{Id: "mod1-dep2", Scopes: []string{"test"}},
				}},
				{Dependencies: []buildinfo.Dependency{
					{Id: "mod2-dep1", Scopes: []string{"test"}},
					{Id: "mod2-dep2", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"mod1-dep1"}, {"mod2-dep2"}},
		},
		{
			name:             "all dependencies excluded",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"test"}},
					{Id: "dep2", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bi := &buildinfo.BuildInfo{Modules: tt.modules}
			bpc := NewBuildPublishCommand().SetDepExcludeScopes(tt.depExcludeScopes)
			bpc.excludeDependenciesByScope(bi)

			require.Equal(t, len(tt.expectedDeps), len(bi.Modules))
			for i, expectedIds := range tt.expectedDeps {
				actualIds := make([]string, len(bi.Modules[i].Dependencies))
				for j, dep := range bi.Modules[i].Dependencies {
					actualIds[j] = dep.Id
				}
				require.Equal(t, expectedIds, actualIds)
			}
		})
	}
}

func TestHasScopeMatch(t *testing.T) {
	excludeSet := map[string]struct{}{"test": {}, "provided": {}}

	tests := []struct {
		name     string
		scopes   []string
		expected bool
	}{
		{name: "matching scope", scopes: []string{"test"}, expected: true},
		{name: "no matching scope", scopes: []string{"compile"}, expected: false},
		{name: "case insensitive match", scopes: []string{"TEST"}, expected: true},
		{name: "nil scopes", scopes: nil, expected: false},
		{name: "empty scopes", scopes: []string{}, expected: false},
		{name: "multiple scopes with match", scopes: []string{"compile", "provided"}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, matchesExcludeScope(tt.scopes, excludeSet))
		})
	}
}
