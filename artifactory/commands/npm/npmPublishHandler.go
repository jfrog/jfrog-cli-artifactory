package npm

import (
	"errors"
	"fmt"
	buildinfo "github.com/jfrog/build-info-go/entities"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
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
		target := fmt.Sprintf("%s/%s", npu.repo, npu.packageInfo.GetDeployPath())

		// If requested, perform a Xray binary scan before deployment. If a FailBuildError is returned, skip the deployment.
		if npu.xrayScan {
			if err = performXrayScan(packedFilePath, npu.repo, npu.serverDetails, npu.scanOutputFormat); err != nil {
				return
			}
		}
		err = errors.Join(err, npu.publishPackage(npu.executablePath, packedFilePath, npu.serverDetails, target))
	}
	return err
}

func (npu *npmPublish) getBuildArtifacts() ([]buildinfo.Artifact, error) {
	return specutils.ConvertArtifactsSearchDetailsToBuildInfoArtifacts(npu.artifactsDetailsReader)
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
		buildProps, err := npu.getBuildPropsForArtifact()
		if err != nil {
			return err
		}
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Pattern: target,
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
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
			log.Warn("Error occurred while setting build properties: ", err)
			log.Warn("This may cause build to not properly link with artifact, please add build name and build number properties on the tarball artifact manually.")
		}
		npu.artifactsDetailsReader = searchReader
	}
	return nil
}
