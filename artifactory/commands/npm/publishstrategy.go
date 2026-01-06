package npm

import (
	buildinfo "github.com/jfrog/build-info-go/entities"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type Publisher interface {
	upload() error
	getBuildArtifacts() []buildinfo.Artifact
}

type NpmPublishStrategy struct {
	strategy     Publisher
	strategyName string
}

// Get npm implementation
func NewNpmPublishStrategy(shouldUseNpmRc bool, npmPublishCommand *NpmPublishCommand) *NpmPublishStrategy {
	nps := NpmPublishStrategy{}
	if shouldUseNpmRc {
		nps.strategy = &npmPublish{npmPublishCommand}
		nps.strategyName = "native"
	} else {
		nps.strategy = &npmRtUpload{npmPublishCommand}
		nps.strategyName = "artifactory"
	}
	return &nps
}

func (nps *NpmPublishStrategy) Publish() error {
	log.Debug("Using strategy for publish: ", nps.strategyName)
	return nps.strategy.upload()
}

func (nps *NpmPublishStrategy) GetBuildArtifacts() []buildinfo.Artifact {
	log.Debug("Using strategy for build info: ", nps.strategyName)
	return nps.strategy.getBuildArtifacts()
}

// ConvertArtifactsDetailsToBuildInfoArtifacts converts artifact details readers to build info artifacts
// using the provided conversion function
func ConvertArtifactsDetailsToBuildInfoArtifacts(artifactsDetailsReader []*content.ContentReader, convertFunc func(*content.ContentReader) ([]buildinfo.Artifact, error)) []buildinfo.Artifact {
	buildArtifacts := make([]buildinfo.Artifact, 0, len(artifactsDetailsReader))
	for _, artifactReader := range artifactsDetailsReader {
		// Skip nil readers to avoid nil pointer dereference when converting artifacts
		if artifactReader == nil {
			log.Debug("Skipping nil artifact details reader")
			continue
		}
		buildArtifact, err := convertFunc(artifactReader)
		if err != nil {
			log.Warn("Failed converting artifact details to build info artifacts: ", err.Error())
		}
		buildArtifacts = append(buildArtifacts, buildArtifact...)
	}
	return buildArtifacts
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
