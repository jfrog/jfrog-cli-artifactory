package buildinfo

import (
	"errors"
	"strconv"
	"testing"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

func (m *mockServicesManager) SetProps(params services.PropsParams) (int, error) {
	args := m.Called(params)
	return args.Int(0), args.Error(1)
}

func TestSetCIVcsPropsOnArtifacts(t *testing.T) {
	// 1. Setup environment
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKFLOW", "test")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "jfrog")
	t.Setenv("GITHUB_REPOSITORY", "jfrog/jfrog-cli")

	// 2. Mock services manager
	mockSM := new(mockServicesManager)
	expectedProps := "vcs.provider=github;vcs.org=jfrog;vcs.repo=jfrog-cli"

	// Expect SetProps to be called for the artifact
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(1, nil)

	// 3. Setup build info
	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					{
						Name:                   "file.jar",
						Path:                   "com/example/file.jar",
						OriginalDeploymentRepo: "libs-release",
					},
				},
			},
		},
	}

	// 4. Run command
	bpc := NewBuildPublishCommand()
	bpc.setCIVcsPropsOnArtifacts(mockSM, bi)

	// 5. Verify
	mockSM.AssertExpectations(t)
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

func TestExtractArtifactPathsWithWarnings(t *testing.T) {
	tests := []struct {
		name            string
		buildInfo       *buildinfo.BuildInfo
		expectedPaths   []string
		expectedSkipped int
	}{
		{
			name: "artifacts with repo path",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{Name: "file1.jar", Path: "com/example/file1.jar", OriginalDeploymentRepo: "libs-release"},
							{Name: "file2.jar", Path: "com/example/file2.jar", OriginalDeploymentRepo: "libs-release"},
						},
					},
				},
			},
			expectedPaths:   []string{"libs-release/com/example/file1.jar", "libs-release/com/example/file2.jar"},
			expectedSkipped: 0,
		},
		{
			name: "artifacts without repo path",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{Name: "file1.jar", Path: "com/example/file1.jar"},
						},
					},
				},
			},
			expectedPaths:   nil,
			expectedSkipped: 1,
		},
		{
			name: "mixed artifacts",
			buildInfo: &buildinfo.BuildInfo{
				Modules: []buildinfo.Module{
					{
						Artifacts: []buildinfo.Artifact{
							{Name: "file1.jar", Path: "com/example/file1.jar", OriginalDeploymentRepo: "libs-release"},
							{Name: "file2.jar", Path: "com/example/file2.jar"},
						},
					},
				},
			},
			expectedPaths:   []string{"libs-release/com/example/file1.jar"},
			expectedSkipped: 1,
		},
		{
			name:            "empty build info",
			buildInfo:       &buildinfo.BuildInfo{},
			expectedPaths:   nil,
			expectedSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, skipped := extractArtifactPathsWithWarnings(tt.buildInfo)
			assert.Equal(t, tt.expectedPaths, paths)
			assert.Equal(t, tt.expectedSkipped, skipped)
		})
	}
}

func TestConstructArtifactPath(t *testing.T) {
	tests := []struct {
		name     string
		artifact buildinfo.Artifact
		expected string
	}{
		{
			name:     "with path",
			artifact: buildinfo.Artifact{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
			expected: "libs-release/com/example/file.jar",
		},
		{
			name:     "with name only",
			artifact: buildinfo.Artifact{Name: "file.jar", OriginalDeploymentRepo: "libs-release"},
			expected: "libs-release/file.jar",
		},
		{
			name:     "no repo",
			artifact: buildinfo.Artifact{Name: "file.jar", Path: "com/example/file.jar"},
			expected: "",
		},
		{
			name:     "empty artifact",
			artifact: buildinfo.Artifact{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructArtifactPath(tt.artifact)
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
