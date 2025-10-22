package buildinfo

import (
	buildinfo "github.com/jfrog/build-info-go/entities"
	"strconv"
	"testing"
	"time"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

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
