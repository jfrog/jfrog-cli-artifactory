package cli

import (
	"fmt"
	"github.com/jfrog/gofrog/log"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils"
	"os"
)

type execCommandFunc func(command commands.Command) error

func exec(command commands.Command) error {
	return commands.Exec(command)
}

var subjectTypes = []string{
	subjectRepoPath,
	releaseBundle,
	buildName,
	packageName,
}

func getEnvVariable(envVarName string) (string, error) {
	if key, exists := os.LookupEnv(envVarName); exists {
		return key, nil
	}
	return "", fmt.Errorf("'%s'  field wasn't provided.", envVarName)
}

func PlatformToEvidenceUrls(rtDetails *coreConfig.ServerDetails) {
	log.Debug("Converting platform URLs to evidence URLs", rtDetails.Url)
	rtDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "artifactory/"
	rtDetails.EvidenceUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "evidence/"
	rtDetails.MetadataUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "metadata/"
	log.Debug("Converted artifactory URL is", rtDetails.ArtifactoryUrl)
	log.Debug("Converted evidence URL is", rtDetails.EvidenceUrl)
}
