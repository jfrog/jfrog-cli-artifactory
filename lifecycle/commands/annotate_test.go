package commands

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReleaseBundleAnnotateCommand_SetServerDetails(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	serverDetails := &config.ServerDetails{
		ArtifactoryUrl: "https://artifactory.example.com",
	}
	cmd.SetServerDetails(serverDetails)
	result, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestReleaseBundleAnnotateCommand_SetReleaseBundleName(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.SetReleaseBundleName("example-release-bundle")
	assert.Equal(t, "example-release-bundle", cmd.releaseBundleName)
}

func TestReleaseBundleAnnotateCommand_SetReleaseBundleVersion(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.SetReleaseBundleVersion("1.0.0")
	assert.Equal(t, "1.0.0", cmd.releaseBundleVersion)
}

func TestReleaseBundleAnnotateCommand_SetReleaseBundleProject(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.SetReleaseBundleProject("example-project")
	assert.Equal(t, "example-project", cmd.rbProjectKey)
}

func TestReleaseBundleAnnotateCommand_SetTag(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.SetTag("example-tag", true)
	assert.Equal(t, "example-tag", cmd.tag)
	assert.Equal(t, true, cmd.tagExist)
}

func TestReleaseBundleAnnotateCommand_SetProps(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.SetProps("example-props=prop-value")
	assert.Equal(t, "example-props=prop-value", cmd.props)
	assert.Equal(t, true, cmd.propsExist)
}

func TestBuildAnnotationOperationParams(t *testing.T) {
	details := services.ReleaseBundleDetails{
		ReleaseBundleName:    "example-release-bundle",
		ReleaseBundleVersion: "1.0.0",
	}
	params := services.CommonOptionalQueryParams{
		ProjectKey: "example-project",
	}
	cmd := NewReleaseBundleAnnotateCommand()
	cmd.tag = "example-tag"
	cmd.serverDetails = &config.ServerDetails{
		ArtifactoryUrl: "https://artifactory.example.com",
	}

	result := BuildAnnotationOperationParams(cmd, details, params)
	assert.Equal(t, "example-tag", result.RbTag.Tag)
	assert.Equal(t, "example-project-release-bundles-v2/example-release-bundle/1.0.0/release-bundle.json.evd", result.PropertyParams.Path)

}

func TestReleaseBundleAnnotateCommand_Run(t *testing.T) {
	cmd := NewReleaseBundleAnnotateCommand()
	serverDetails := &config.ServerDetails{
		ArtifactoryUrl: "https://artifactory.example.com",
	}
	cmd.SetServerDetails(serverDetails)
	cmd.SetReleaseBundleName("example-release-bundle")
	cmd.SetReleaseBundleVersion("1.0.0")
	cmd.SetReleaseBundleProject("example-project")
	cmd.SetTag("example-tag", true)
	cmd.SetProps("example-props=prop-value;example-props-2=prop-value-2")
	cmd.DeleteProps("example-props-2")

	// Mock validateFeatureSupportedVersion function
	cmd.validateVersionFunc = func(*config.ServerDetails, string) error {
		return nil
	}

	// Mock getPrerequisites function
	cmd.getPrerequisitesFunc = func() (*lifecycle.LifecycleServicesManager, services.ReleaseBundleDetails, services.CommonOptionalQueryParams, error) {
		testServicesManager := &lifecycle.LifecycleServicesManager{}
		testReleaseBundleDetails := services.ReleaseBundleDetails{
			ReleaseBundleName:    "example-release-bundle",
			ReleaseBundleVersion: "1.0.0",
		}
		testQueryParams := services.CommonOptionalQueryParams{
			ProjectKey: "example-project",
		}
		return testServicesManager, testReleaseBundleDetails, testQueryParams, nil
	}

	// Mock annotateReleaseBundle function
	cmd.annotateReleaseBundleFunc = func(rba *ReleaseBundleAnnotateCommand, manager *lifecycle.LifecycleServicesManager,
		details services.ReleaseBundleDetails, params services.CommonOptionalQueryParams) error {
		return nil
	}

	err := cmd.Run()
	assert.NoError(t, err)
}
