package npm

import (
	buildinfo "github.com/jfrog/build-info-go/entities"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type Publish interface {
	upload() error
	getBuildArtifacts() ([]buildinfo.Artifact, error)
}

// Get npm implementation
func NpmPublishStrategy(shouldUseNpmRc bool, npmPublishCommand *NpmPublishCommand) Publish {

	if shouldUseNpmRc {
		return &npmPublish{npmPublishCommand}
	}

	return &npmRtUpload{npmPublishCommand}
}

func performXrayScan(filePath string, repo string, serverDetails *config.ServerDetails, scanOutputFormat format.OutputFormat) error {
	fileSpec := spec.NewBuilder().
		Pattern(filePath).
		Target(repo + "/").
		BuildSpec()
	if err := commandsutils.ConditionalUploadScanFunc(serverDetails, fileSpec, 1, scanOutputFormat); err != nil {
		return err
	}
	return nil
}
