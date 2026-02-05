package pnpm

import (
	"errors"
	"fmt"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

type pnpmRtUpload struct {
	*PnpmPublishCommand
}

func (pru *pnpmRtUpload) upload() (err error) {
	for _, packedFilePath := range pru.packedFilePaths {
		if err = pru.readPackageInfoFromTarball(packedFilePath); err != nil {
			return
		}
		target := fmt.Sprintf("%s/%s", pru.repo, pru.packageInfo.GetDeployPath())

		// If requested, perform a Xray binary scan before deployment. If a FailBuildError is returned, skip the deployment.
		if pru.xrayScan {
			if err = performXrayScan(packedFilePath, pru.repo, pru.serverDetails, pru.scanOutputFormat); err != nil {
				return
			}
		}
		err = errors.Join(err, pru.doDeploy(target, pru.serverDetails, packedFilePath))
	}
	return
}

func (pru *pnpmRtUpload) getBuildArtifacts() []buildinfo.Artifact {
	return ConvertArtifactsDetailsToBuildInfoArtifacts(pru.artifactsDetailsReader, specutils.ConvertArtifactsDetailsToBuildInfoArtifacts)
}

func (pru *pnpmRtUpload) doDeploy(target string, artDetails *config.ServerDetails, packedFilePath string) error {
	servicesManager, err := utils.CreateServiceManager(artDetails, -1, 0, false)
	if err != nil {
		return err
	}
	up := services.NewUploadParams()
	up.CommonParams = &specutils.CommonParams{Pattern: packedFilePath, Target: target}
	if err = pru.addDistTagIfSet(up.CommonParams); err != nil {
		return err
	}
	// Add CI VCS properties if in CI environment
	if err = pru.addCIVcsProps(up.CommonParams); err != nil {
		return err
	}
	var totalFailed int
	if pru.collectBuildInfo || pru.detailedSummary {
		if pru.collectBuildInfo {
			up.BuildProps, err = pru.getBuildPropsForArtifact()
			if err != nil {
				return err
			}
		}
		summary, err := servicesManager.UploadFilesWithSummary(artifactory.UploadServiceOptions{}, up)
		if err != nil {
			return err
		}
		totalFailed = summary.TotalFailed
		if pru.collectBuildInfo {
			pru.artifactsDetailsReader = append(pru.artifactsDetailsReader, summary.ArtifactsDetailsReader)
		} else {
			err = summary.ArtifactsDetailsReader.Close()
			if err != nil {
				return err
			}
		}
		if pru.detailedSummary {
			if err = pru.setDetailedSummary(summary); err != nil {
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
		return errorutils.CheckErrorf("Failed to upload the pnpm package to Artifactory. See Artifactory logs for more details.")
	}
	return nil
}

func (pru *pnpmRtUpload) addDistTagIfSet(params *specutils.CommonParams) error {
	if pru.distTag == "" {
		return nil
	}
	props, err := specutils.ParseProperties(DistTagPropKey + "=" + pru.distTag)
	if err != nil {
		return err
	}
	params.TargetProps = props
	return nil
}

// addCIVcsProps adds CI VCS properties to the upload params if in CI environment.
func (pru *pnpmRtUpload) addCIVcsProps(params *specutils.CommonParams) error {
	ciProps := civcs.GetCIVcsPropsString()
	if ciProps == "" {
		return nil
	}
	if params.TargetProps == nil {
		props, err := specutils.ParseProperties(ciProps)
		if err != nil {
			return err
		}
		params.TargetProps = props
	} else {
		// Merge with existing properties
		if err := params.TargetProps.ParseAndAddProperties(ciProps); err != nil {
			return err
		}
	}
	return nil
}

func (pru *pnpmRtUpload) appendReader(summary *specutils.OperationSummary) error {
	readersSlice := []*content.ContentReader{pru.result.Reader(), summary.TransferDetailsReader}
	reader, err := content.MergeReaders(readersSlice, content.DefaultKey)
	if err != nil {
		return err
	}
	pru.result.SetReader(reader)
	return nil
}

func (pru *pnpmRtUpload) setDetailedSummary(summary *specutils.OperationSummary) (err error) {
	pru.result.SetFailCount(pru.result.FailCount() + summary.TotalFailed)
	pru.result.SetSuccessCount(pru.result.SuccessCount() + summary.TotalSucceeded)
	if pru.result.Reader() == nil {
		pru.result.SetReader(summary.TransferDetailsReader)
	} else {
		if err = pru.appendReader(summary); err != nil {
			return
		}
	}
	return
}
