package npm

import (
	"errors"
	"fmt"
	"strings"

	buildinfo "github.com/jfrog/build-info-go/entities"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type npmPublish struct {
	*NpmPublishCommand
}

func (npu *npmPublish) upload() (err error) {
	for _, packedFilePath := range npu.packedFilePaths {
		if err = npu.readPackageInfoFromTarball(packedFilePath); err != nil {
			return err
		}
		var repoConfig, targetRepo string
		var targetServer *config.ServerDetails
		repoConfig, err = npu.getRepoConfig()
		if err != nil {
			return err
		}
		targetRepo, err = extractRepoName(repoConfig)
		if err != nil {
			return err
		}
		targetServer, err = extractConfigServer(repoConfig)
		if err != nil {
			return err
		}
		target := fmt.Sprintf("%s/%s", targetRepo, npu.packageInfo.GetDeployPath())

		// If requested, perform a Xray binary scan before deployment. If a FailBuildError is returned, skip the deployment.
		if npu.xrayScan {
			if err = performXrayScan(packedFilePath, npu.repo, targetServer, npu.scanOutputFormat); err != nil {
				return
			}
		}
		err = errors.Join(err, npu.publishPackage(npu.executablePath, packedFilePath, targetServer, target))
	}
	return
}

func (npu *npmPublish) getBuildArtifacts() []buildinfo.Artifact {
	return ConvertArtifactsDetailsToBuildInfoArtifacts(npu.artifactsDetailsReader, utils.ConvertArtifactsSearchDetailsToBuildInfoArtifacts)
}

func (npu *npmPublish) publishPackage(executablePath, filePath string, serverDetails *config.ServerDetails, target string) error {
	npmCommand := gofrogcmd.NewCommand(executablePath, "publish", []string{filePath})
	output, cmdError, _, err := gofrogcmd.RunCmdWithOutputParser(npmCommand, true)
	if err != nil {
		log.Error("Error occurred while running npm publish: ", output, cmdError, err)
		npu.result.SetFailCount(npu.result.FailCount() + 1)
		return err
	}
	npu.result.SetSuccessCount(npu.result.SuccessCount() + 1)
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return err
	}

	if npu.collectBuildInfo {
		var buildProps string
		var searchReader *content.ContentReader

		buildProps, err = npu.getBuildPropsForArtifact()
		if err != nil {
			return err
		}
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Pattern: target,
			},
		}
		searchReader, err = servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Error("Failed to get uploaded npm package: ", err.Error())
			return err
		}

		propsParams := services.PropsParams{
			Reader: searchReader,
			Props:  buildProps,
		}
		_, err = servicesManager.SetProps(propsParams)
		if err != nil {
			log.Warn("Unable to set build properties: ", err, "\nThis may cause build to not properly link with artifact, please add build name and build number properties on the tarball artifact manually")
		}
		npu.artifactsDetailsReader = append(npu.artifactsDetailsReader, searchReader)
	}
	return nil
}

func (npu *NpmPublishCommand) getRepoConfig() (string, error) {
	var registryString string
	scope := npu.packageInfo.Scope
	if scope == "" {
		registryString = "registry"
	} else {
		registryString = scope + ":registry"
	}
	configCommand := gofrogcmd.Command{
		Executable: npu.executablePath,
		CmdName:    "config",
		CmdArgs:    []string{"get", registryString},
	}
	data, err := configCommand.RunWithOutput()
	repoConfig := string(data)
	if err != nil {
		log.Error("Error occurred while running npm config get: ", err)
		npu.result.SetFailCount(npu.result.FailCount() + 1)
		return "", err
	}
	return repoConfig, nil
}

func extractRepoName(configUrl string) (string, error) {
	url := strings.TrimSpace(configUrl)
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimSuffix(url, "/")
	if url == "" {
		return "", errors.New("npm config URL is empty")
	}
	urlParts := strings.Split(url, "/")
	if len(urlParts) < 2 {
		return "", errors.New("npm config URL is not valid")
	}
	return urlParts[len(urlParts)-1], nil
}

func extractConfigServer(configUrl string) (*config.ServerDetails, error) {
	var requiredServerDetails = &config.ServerDetails{}
	url := strings.TrimSpace(configUrl)
	allAvailableConfigs, err := config.GetAllServersConfigs()
	if err != nil {
		return requiredServerDetails, err
	}

	for _, availableConfig := range allAvailableConfigs {
		if strings.HasPrefix(url, availableConfig.ArtifactoryUrl) {
			requiredServerDetails = availableConfig
		}
	}

	if requiredServerDetails == nil {
		return requiredServerDetails, fmt.Errorf("no server details found for the URL: %s to create build info", url)
	}

	return requiredServerDetails, nil
}
