package npm

import (
	"errors"
	"fmt"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

type npmRtUpload struct {
	*NpmPublishCommand
}

func (nru *npmRtUpload) upload() (err error) {
	for _, packedFilePath := range nru.packedFilePaths {
		if err = nru.readPackageInfoFromTarball(packedFilePath); err != nil {
			return
		}
		target := fmt.Sprintf("%s/%s", nru.repo, nru.packageInfo.GetDeployPath())

		// If requested, perform a Xray binary scan before deployment. If a FailBuildError is returned, skip the deployment.
		if nru.xrayScan {
			if err = performXrayScan(packedFilePath, nru.repo, nru.serverDetails, nru.scanOutputFormat); err != nil {
				return
			}
		}
		err = errors.Join(err, nru.doDeploy(target, nru.serverDetails, packedFilePath))
	}
	return
}

func (nru *npmRtUpload) getBuildArtifacts() []buildinfo.Artifact {
	return ConvertArtifactsDetailsToBuildInfoArtifacts(nru.artifactsDetailsReader, specutils.ConvertArtifactsDetailsToBuildInfoArtifacts)
}

func (nru *npmRtUpload) doDeploy(target string, artDetails *config.ServerDetails, packedFilePath string) error {
	servicesManager, err := utils.CreateServiceManager(artDetails, -1, 0, false)
	if err != nil {
		return err
	}
	up := services.NewUploadParams()
	up.CommonParams = &specutils.CommonParams{Pattern: packedFilePath, Target: target}
	if err = nru.addDistTagIfSet(up.CommonParams); err != nil {
		return err
	}
	var totalFailed int
	if nru.collectBuildInfo || nru.detailedSummary {
		if nru.collectBuildInfo {
			up.BuildProps, err = nru.getBuildPropsForArtifact()
			if err != nil {
				return err
			}
		}
		summary, err := servicesManager.UploadFilesWithSummary(artifactory.UploadServiceOptions{}, up)
		if err != nil {
			return err
		}
		totalFailed = summary.TotalFailed
		if nru.collectBuildInfo {
			nru.artifactsDetailsReader = append(nru.artifactsDetailsReader, summary.ArtifactsDetailsReader)
		} else {
			err = summary.ArtifactsDetailsReader.Close()
			if err != nil {
				return err
			}
		}
		if nru.detailedSummary {
			if err = nru.setDetailedSummary(summary); err != nil {
				return err
			}
		} else {
			if err = summary.TransferDetailsReader.Close(); err != nil {
				return err
			}
		}
	} else {
		_, totalFailed, err = servicesManager.UploadFiles(artifactory.UploadServiceOptions{}, up)
		if err != nil {
			return err
		}
	}

	// We are deploying only one Artifact which have to be deployed, in case of failure we should fail
	if totalFailed > 0 {
		return errorutils.CheckErrorf("Failed to upload the npm package to Artifactory. See Artifactory logs for more details.")
	}
	return nil
}

func (nru *npmRtUpload) addDistTagIfSet(params *specutils.CommonParams) error {
	if nru.distTag == "" {
		return nil
	}
	props, err := specutils.ParseProperties(DistTagPropKey + "=" + nru.distTag)
	if err != nil {
		return err
	}
	params.TargetProps = props
	return nil
}

func (nru *npmRtUpload) appendReader(summary *specutils.OperationSummary) error {
	readersSlice := []*content.ContentReader{nru.result.Reader(), summary.TransferDetailsReader}
	reader, err := content.MergeReaders(readersSlice, content.DefaultKey)
	if err != nil {
		return err
	}
	nru.result.SetReader(reader)
	return nil
}

func (nru *npmRtUpload) setDetailedSummary(summary *specutils.OperationSummary) (err error) {
	nru.result.SetFailCount(nru.result.FailCount() + summary.TotalFailed)
	nru.result.SetSuccessCount(nru.result.SuccessCount() + summary.TotalSucceeded)
	if nru.result.Reader() == nil {
		nru.result.SetReader(summary.TransferDetailsReader)
	} else {
		if err = nru.appendReader(summary); err != nil {
			return
		}
	}
	return
}
