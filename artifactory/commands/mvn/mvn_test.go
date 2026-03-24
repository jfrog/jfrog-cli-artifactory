package mvn

import (
	"encoding/json"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestIsDeploymentRequested(t *testing.T) {
	tests := []struct {
		name     string
		goals    []string
		expected bool
	}{
		{
			name:     "install goal",
			goals:    []string{"install"},
			expected: true,
		},
		{
			name:     "deploy goal",
			goals:    []string{"deploy"},
			expected: true,
		},
		{
			name:     "deploy:deploy-file goal",
			goals:    []string{"deploy:deploy-file"},
			expected: true,
		},
		{
			name:     "deploy:deploy goal",
			goals:    []string{"deploy:deploy"},
			expected: true,
		},
		{
			name:     "install:install-file goal",
			goals:    []string{"install:install-file"},
			expected: true,
		},
		{
			name:     "package goal",
			goals:    []string{"package"},
			expected: false,
		},
		{
			name:     "verify goal",
			goals:    []string{"verify"},
			expected: false,
		},
		{
			name:     "clean install goals",
			goals:    []string{"clean", "install"},
			expected: true,
		},
		{
			name:     "clean deploy:deploy-file goals",
			goals:    []string{"clean", "deploy:deploy-file"},
			expected: true,
		},
		{
			name:     "compile test goals",
			goals:    []string{"compile", "test"},
			expected: false,
		},
		{
			name:     "deploy:help goal",
			goals:    []string{"deploy:help"},
			expected: false,
		},
		{
			name:     "install:help goal",
			goals:    []string{"install:help"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := &MvnCommand{goals: tt.goals}
			result := mc.isDeploymentRequested()
			assert.Equal(t, tt.expected, result, "Expected isDeploymentRequested() to return %v for goals %v", tt.expected, tt.goals)
		})
	}
}

func TestUpdateBuildInfoArtifactsWithTargetRepo(t *testing.T) {
	vConfig := viper.New()
	vConfig.Set(build.DeployerPrefix+build.SnapshotRepo, "snapshots")
	vConfig.Set(build.DeployerPrefix+build.ReleaseRepo, "releases")

	tempDir := t.TempDir()
	assert.NoError(t, io.CopyDir(filepath.Join("testdata", "buildinfo_files"), tempDir, true, nil))

	buildName := "buildName"
	buildNumber := "1"
	mc := MvnCommand{
		configuration: build.NewBuildConfiguration(buildName, buildNumber, "", ""),
	}

	buildInfoFilePath := filepath.Join(tempDir, "buildinfo1")

	err := mc.updateBuildInfoArtifactsWithDeploymentRepo(vConfig, buildInfoFilePath)
	assert.NoError(t, err)

	buildInfoContent, err := os.ReadFile(buildInfoFilePath)
	assert.NoError(t, err)

	var buildInfo entities.BuildInfo
	assert.NoError(t, json.Unmarshal(buildInfoContent, &buildInfo))

	assert.Len(t, buildInfo.Modules, 2)
	modules := buildInfo.Modules

	firstModule := modules[0]
	assert.Len(t, firstModule.Artifacts, 0)
	excludedArtifacts := firstModule.ExcludedArtifacts
	assert.Len(t, excludedArtifacts, 2)
	assert.Equal(t, "snapshots", excludedArtifacts[0].OriginalDeploymentRepo)
	assert.Equal(t, "snapshots", excludedArtifacts[1].OriginalDeploymentRepo)

	secondModule := modules[1]
	assert.Len(t, secondModule.ExcludedArtifacts, 0)
	artifacts := secondModule.Artifacts
	assert.Len(t, artifacts, 2)
	assert.Equal(t, "releases", artifacts[0].OriginalDeploymentRepo)
	assert.Equal(t, "releases", artifacts[1].OriginalDeploymentRepo)
}
