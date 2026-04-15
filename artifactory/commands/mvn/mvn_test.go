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
		// Standard Maven phases
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
		// Plugin prefix format (plugin:goal)
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
		// Full plugin name format (maven-plugin:goal)
		{
			name:     "maven-deploy-plugin:deploy goal",
			goals:    []string{"maven-deploy-plugin:deploy"},
			expected: true,
		},
		{
			name:     "maven-install-plugin:install goal",
			goals:    []string{"maven-install-plugin:install"},
			expected: true,
		},
		// Fully qualified plugin with version
		{
			name:     "org.apache.maven.plugins:maven-deploy-plugin:3.1.4:deploy goal",
			goals:    []string{"org.apache.maven.plugins:maven-deploy-plugin:3.1.4:deploy"},
			expected: true,
		},
		// Container deployment plugins
		{
			name:     "wildfly:deploy goal",
			goals:    []string{"wildfly:deploy"},
			expected: true,
		},
		{
			name:     "tomcat7:deploy goal",
			goals:    []string{"tomcat7:deploy"},
			expected: true,
		},
		// Non-deployment goals
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
			name:     "clean goal",
			goals:    []string{"clean"},
			expected: false,
		},
		{
			name:     "compile goal",
			goals:    []string{"compile"},
			expected: false,
		},
		{
			name:     "test goal",
			goals:    []string{"test"},
			expected: false,
		},
		// Multiple goals
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
		// Help goals (should be excluded)
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
		{
			name:     "maven-deploy-plugin:help goal",
			goals:    []string{"maven-deploy-plugin:help"},
			expected: false,
		},
		{
			name:     "help goal",
			goals:    []string{"help"},
			expected: false,
		},
		// Edge case: uninstall goals (should NOT trigger deployment)
		{
			name:     "sling:uninstall goal",
			goals:    []string{"sling:uninstall"},
			expected: false,
		},
		{
			name:     "osgi:uninstall goal",
			goals:    []string{"osgi:uninstall"},
			expected: false,
		},
		{
			name:     "felix:reinstall goal",
			goals:    []string{"felix:reinstall"},
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
