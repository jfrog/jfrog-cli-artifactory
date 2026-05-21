package common

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

const publishUploadExpectedFiles = 1

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
		if err := validatePublishUploadSummary(summary); err != nil {
			closePublishUploadSummaryReaders(summary)
			return nil, err
		}
		if summary.TransferDetailsReader != nil {
			_ = summary.TransferDetailsReader.Close() // read-side summary reader not returned to caller
		}
		return summary.ArtifactsDetailsReader, nil
	}

	totalUploaded, totalFailed, err := serviceManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	if err != nil {
		return nil, err
	}
	if err := validatePublishUploadCounts(totalUploaded, totalFailed); err != nil {
		return nil, err
	}
	return nil, nil
}

// validatePublishUploadSummary ensures the upload service transferred the expected number of files.
// Artifactory may return HTTP 200 while TotalSucceeded is 0 (e.g. pattern matched no files).
func validatePublishUploadSummary(summary *rtServicesUtils.OperationSummary) error {
	if summary == nil {
		return fmt.Errorf("upload finished without a summary (0 files transferred, expected %d)", publishUploadExpectedFiles)
	}
	return validatePublishUploadCounts(summary.TotalSucceeded, summary.TotalFailed)
}

func validatePublishUploadCounts(totalSucceeded, totalFailed int) error {
	if totalFailed > 0 {
		return fmt.Errorf("upload finished with %d failed file(s)", totalFailed)
	}
	if totalSucceeded < publishUploadExpectedFiles {
		return fmt.Errorf("upload finished with %d succeeded file(s), expected at least %d", totalSucceeded, publishUploadExpectedFiles)
	}
	return nil
}

func closePublishUploadSummaryReaders(summary *rtServicesUtils.OperationSummary) {
	if summary == nil {
		return
	}
	if summary.TransferDetailsReader != nil {
		_ = summary.TransferDetailsReader.Close() // best-effort on validation failure
	}
	if summary.ArtifactsDetailsReader != nil {
		_ = summary.ArtifactsDetailsReader.Close() // best-effort on validation failure
	}
}
