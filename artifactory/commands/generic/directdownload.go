package generic

import (
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type DirectDownloadCommand struct {
	DownloadCommand
}

func NewDirectDownloadCommand() *DirectDownloadCommand {
	return &DirectDownloadCommand{
		DownloadCommand: *NewDownloadCommand(),
	}
}

func (ddc *DirectDownloadCommand) CommandName() string {
	return "rt_direct_download"
}

func (ddc *DirectDownloadCommand) Run() error {
	if err := ddc.directDownload(); err != nil {
		return err
	}
	return nil
}

func (ddc *DirectDownloadCommand) directDownload() error {
	servicesManager, err := utils.CreateServiceManager(ddc.serverDetails, -1, 0, ddc.DryRun())
	if err != nil {
		return err
	}

	var downloadParamsArray []services.DirectDownloadParams

	for i := 0; i < len(ddc.Spec().Files); i++ {
		currentSpec := ddc.Spec().Get(i)
		downloadParams := services.NewDirectDownloadParams()
		downloadParams.SetPattern(currentSpec.Pattern)
		downloadParams.SetTarget(currentSpec.Target)

		isFlat, err := currentSpec.IsFlat(false)
		if err != nil {
			return err
		}
		downloadParams.SetFlat(isFlat)

		isRecursive, err := currentSpec.IsRecursive(true)
		if err != nil {
			return err
		}
		downloadParams.SetRecursive(isRecursive)

		downloadParams.SetExclusions(currentSpec.Exclusions)
		downloadParams.SetSkipChecksum(ddc.Configuration().SkipChecksum)
		downloadParams.SetRetries(ddc.retries)
		downloadParams.SetSyncDeletesPath(ddc.SyncDeletesPath())
		downloadParams.SetQuiet(ddc.quiet)

		downloadParamsArray = append(downloadParamsArray, *downloadParams)
	}

	var summary *serviceutils.OperationSummary
	if ddc.DetailedSummary() || ddc.SyncDeletesPath() != "" {
		summary, err = servicesManager.DirectDownloadFilesWithSummary(downloadParamsArray...)
	} else {
		var totalDownloaded, totalFailed int
		totalDownloaded, totalFailed, err = servicesManager.DirectDownloadFiles(downloadParamsArray...)
		summary = &serviceutils.OperationSummary{
			TotalSucceeded: totalDownloaded,
			TotalFailed:    totalFailed,
		}
	}

	if err != nil {
		return err
	}

	ddc.result.SetSuccessCount(summary.TotalSucceeded)
	ddc.result.SetFailCount(summary.TotalFailed)

	return nil
}

func (ddc *DirectDownloadCommand) ValidateFlags() error {
	for i := 0; i < len(ddc.Spec().Files); i++ {
		fileSpec := ddc.Spec().Get(i)

		if fileSpec.SortBy != nil && len(fileSpec.SortBy) > 0 {
			return errorutils.CheckErrorf("The --sort-by flag is not supported with direct download")
		}
		if fileSpec.SortOrder != "" {
			return errorutils.CheckErrorf("The --sort-order flag is not supported with direct download")
		}
		if fileSpec.Limit > 0 {
			return errorutils.CheckErrorf("The --limit flag is not supported with direct download")
		}
		if fileSpec.Offset > 0 {
			return errorutils.CheckErrorf("The --offset flag is not supported with direct download")
		}
		if fileSpec.Props != "" {
			return errorutils.CheckErrorf("The --props flag is not supported with direct download")
		}
		if fileSpec.ExcludeProps != "" {
			return errorutils.CheckErrorf("The --exclude-props flag is not supported with direct download")
		}
		if fileSpec.ArchiveEntries != "" {
			return errorutils.CheckErrorf("The --archive-entries flag is not supported with direct download")
		}
	}

	return nil
}

func (ddc *DirectDownloadCommand) SetServerDetails(serverDetails *config.ServerDetails) *DirectDownloadCommand {
	ddc.serverDetails = serverDetails
	return ddc
}

func (ddc *DirectDownloadCommand) SetSpec(spec *spec.SpecFiles) *DirectDownloadCommand {
	ddc.DownloadCommand.SetSpec(spec)
	return ddc
}

func (ddc *DirectDownloadCommand) SetConfiguration(configuration *utils.DownloadConfiguration) *DirectDownloadCommand {
	ddc.configuration = configuration
	return ddc
}

func (ddc *DirectDownloadCommand) SetBuildConfiguration(buildConfiguration *build.BuildConfiguration) *DirectDownloadCommand {
	ddc.buildConfiguration = buildConfiguration
	return ddc
}

func (ddc *DirectDownloadCommand) SetDryRun(dryRun bool) *DirectDownloadCommand {
	ddc.dryRun = dryRun
	return ddc
}

func (ddc *DirectDownloadCommand) SetSyncDeletesPath(syncDeletes string) *DirectDownloadCommand {
	ddc.syncDeletesPath = syncDeletes
	return ddc
}

func (ddc *DirectDownloadCommand) SetQuiet(quiet bool) *DirectDownloadCommand {
	ddc.quiet = quiet
	return ddc
}

func (ddc *DirectDownloadCommand) SetDetailedSummary(detailedSummary bool) *DirectDownloadCommand {
	ddc.detailedSummary = detailedSummary
	return ddc
}

func (ddc *DirectDownloadCommand) SetRetries(retries int) *DirectDownloadCommand {
	ddc.retries = retries
	return ddc
}

func (ddc *DirectDownloadCommand) SetRetryWaitMilliSecs(retryWaitMilliSecs int) *DirectDownloadCommand {
	ddc.retryWaitTimeMilliSecs = retryWaitMilliSecs
	return ddc
}
