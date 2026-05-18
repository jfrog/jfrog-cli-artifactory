package agentcommon

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

// UploadPublishArtifact uploads a single file to uploadTarget using the standard
// Artifactory upload service. When collectBuildInfo is true, buildConfiguration
// must be non-nil and artifact details are returned for build-info partials.
func UploadPublishArtifact(
	serverDetails *config.ServerDetails,
	zipPath, uploadTarget string,
	collectBuildInfo bool,
	buildConfiguration *build.BuildConfiguration,
) (*content.ContentReader, error) {
	serviceManager, err := utils.CreateUploadServiceManager(serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return nil, err
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = zipPath
	uploadParams.Target = uploadTarget
	uploadParams.Flat = true

	if collectBuildInfo {
		if buildConfiguration == nil {
			return nil, fmt.Errorf("build-info collection requested, but build configuration is nil")
		}
		buildProps, err := build.CreateBuildPropsFromConfiguration(buildConfiguration)
		if err != nil {
			return nil, err
		}
		uploadParams.BuildProps = buildProps

		summary, err := serviceManager.UploadFilesWithSummary(artifactory.UploadServiceOptions{}, uploadParams)
		if err != nil {
			return nil, err
		}
		if summary != nil {
			if summary.TransferDetailsReader != nil {
				_ = summary.TransferDetailsReader.Close()
			}
			return summary.ArtifactsDetailsReader, nil
		}
		return nil, nil
	}

	_, _, err = serviceManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	return nil, err
}
