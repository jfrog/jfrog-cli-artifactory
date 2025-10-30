package generic

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/stretchr/testify/assert"
)

func TestNewDirectDownloadCommand(t *testing.T) {
	ddc := NewDirectDownloadCommand()
	assert.NotNil(t, ddc)
	assert.Equal(t, "rt_direct_download", ddc.CommandName())
}

func TestGetDirectDownloadParams_BasicPattern(t *testing.T) {
	file := &spec.File{
		Pattern: "repo/path/*.bin",
		Target:  "downloads/",
	}

	configuration := &utils.DownloadConfiguration{
		Threads:      3,
		MinSplitSize: 5120,
		SplitCount:   3,
		SkipChecksum: false,
	}

	params, err := getDirectDownloadParams(file, configuration)
	assert.NoError(t, err)
	assert.NotNil(t, params)

	// Check CommonParams was set
	assert.Equal(t, "repo/path/*.bin", params.CommonParams.Pattern)
	assert.Equal(t, "downloads/", params.CommonParams.Target)

	assert.Equal(t, int64(5120), params.MinSplitSize)
	assert.Equal(t, 3, params.SplitCount)
	assert.False(t, params.SkipChecksum)

	assert.True(t, params.Recursive)
	assert.False(t, params.Flat)
	assert.False(t, params.Explode)
	assert.False(t, params.IncludeDirs)
}

func TestGetDirectDownloadParams_RecursiveFlag(t *testing.T) {
	tests := []struct {
		name           string
		recursive      string
		expectedResult bool
	}{
		{"Recursive true", "true", true},
		{"Recursive false", "false", false},
		{"Recursive default (empty)", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &spec.File{
				Pattern:   "repo/*.bin",
				Recursive: tt.recursive,
			}

			configuration := &utils.DownloadConfiguration{}
			params, err := getDirectDownloadParams(file, configuration)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, params.Recursive)
		})
	}
}

func TestGetDirectDownloadParams_SkipChecksum(t *testing.T) {
	tests := []struct {
		name             string
		skipChecksum     bool
		expectedChecksum bool
	}{
		{"Skip checksum true", true, true},
		{"Skip checksum false", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &spec.File{
				Pattern: "repo/*.bin",
			}

			configuration := &utils.DownloadConfiguration{
				SkipChecksum: tt.skipChecksum,
			}

			params, err := getDirectDownloadParams(file, configuration)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedChecksum, params.SkipChecksum)
		})
	}
}

func TestGetDirectDownloadParams_CombinedFlags(t *testing.T) {
	file := &spec.File{
		Pattern:    "repo/path/**/*.jar",
		Target:     "downloads/libs/",
		Recursive:  "true",
		Flat:       "true",
		Exclusions: []string{"*-sources.jar", "*-javadoc.jar"},
	}

	configuration := &utils.DownloadConfiguration{
		Threads:      8,
		MinSplitSize: 10485760, // 10MB
		SplitCount:   5,
		SkipChecksum: false,
	}

	params, err := getDirectDownloadParams(file, configuration)
	assert.NoError(t, err)

	assert.Equal(t, "repo/path/**/*.jar", params.CommonParams.Pattern)
	assert.Equal(t, "downloads/libs/", params.CommonParams.Target)
	assert.True(t, params.Recursive)
	assert.True(t, params.Flat)
	assert.Equal(t, int64(10485760), params.MinSplitSize)
	assert.Equal(t, 5, params.SplitCount)
	assert.False(t, params.SkipChecksum)
	assert.Len(t, params.CommonParams.Exclusions, 2)
}

func TestGetDirectDownloadParams_WithExclusions(t *testing.T) {
	file := &spec.File{
		Pattern: "repo/*.bin",
		Exclusions: []string{
			"*-test.bin",
			"*-debug.bin",
			"temp/*",
		},
	}

	configuration := &utils.DownloadConfiguration{}
	params, err := getDirectDownloadParams(file, configuration)

	assert.NoError(t, err)
	assert.Len(t, params.CommonParams.Exclusions, 3)
	assert.Contains(t, params.CommonParams.Exclusions, "*-test.bin")
	assert.Contains(t, params.CommonParams.Exclusions, "*-debug.bin")
	assert.Contains(t, params.CommonParams.Exclusions, "temp/*")
}

func TestGetDirectDownloadParams_WithBuildInfo(t *testing.T) {
	file := &spec.File{
		Pattern: "repo/*.bin",
		Build:   "my-build/123",
	}

	configuration := &utils.DownloadConfiguration{}
	params, err := getDirectDownloadParams(file, configuration)

	assert.NoError(t, err)
	assert.Equal(t, "my-build/123", params.CommonParams.Build)
}

func TestGetDirectDownloadParams_EmptyPattern(t *testing.T) {
	file := &spec.File{
		Pattern: "",
		Target:  "downloads/",
	}

	configuration := &utils.DownloadConfiguration{}
	params, err := getDirectDownloadParams(file, configuration)

	assert.NoError(t, err)
	assert.Equal(t, "", params.CommonParams.Pattern)
}

func TestGetDirectDownloadParams_AllBooleanFlags(t *testing.T) {
	file := &spec.File{
		Pattern:          "repo/*.bin",
		Recursive:        "true",
		Flat:             "true",
		Explode:          "true",
		IncludeDirs:      "true",
		ExcludeArtifacts: "true",
		IncludeDeps:      "true",
		Transitive:       "true",
		Build:            "my-build",
	}

	configuration := &utils.DownloadConfiguration{}
	params, err := getDirectDownloadParams(file, configuration)

	assert.NoError(t, err)
	assert.True(t, params.Recursive)
	assert.True(t, params.Flat)
	assert.True(t, params.Explode)
	assert.True(t, params.IncludeDirs)
	assert.True(t, params.ExcludeArtifacts)
	assert.True(t, params.IncludeDeps)
	assert.True(t, params.Transitive)
}

func TestGetDirectDownloadParams_DefaultConfiguration(t *testing.T) {
	file := &spec.File{
		Pattern: "repo/*.bin",
	}

	configuration := &utils.DownloadConfiguration{}
	params, err := getDirectDownloadParams(file, configuration)

	assert.NoError(t, err)
	assert.Equal(t, int64(0), params.MinSplitSize)
	assert.Equal(t, 0, params.SplitCount)
	assert.False(t, params.SkipChecksum)
}
