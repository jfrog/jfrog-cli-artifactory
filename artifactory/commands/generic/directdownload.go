package generic

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	buildinfo "github.com/jfrog/build-info-go/entities"
	gofrog "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type DirectDownloadCommand struct {
	DownloadCommand
}

func NewDirectDownloadCommand() *DirectDownloadCommand {
	return &DirectDownloadCommand{DownloadCommand: *NewDownloadCommand()}
}

func (ddc *DirectDownloadCommand) CommandName() string {
	return "rt_direct_download"
}

func (ddc *DirectDownloadCommand) Run() error {
	return ddc.directDownload()
}

func (ddc *DirectDownloadCommand) directDownload() error {
	// Init progress bar if needed
	if ddc.progress != nil {
		ddc.progress.SetHeadlineMsg("")
		ddc.progress.InitProgressReaders()
	}

	servicesManager, err := utils.CreateDownloadServiceManager(ddc.serverDetails, ddc.configuration.Threads, ddc.retries, ddc.retryWaitTimeMilliSecs, ddc.DryRun(), ddc.progress)
	if err != nil {
		return err
	}

	// Build Info Collection:
	toCollect, err := ddc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if toCollect && !ddc.DryRun() {
		var buildName, buildNumber string
		buildName, err = ddc.buildConfiguration.GetBuildName()
		if err != nil {
			return err
		}
		buildNumber, err = ddc.buildConfiguration.GetBuildNumber()
		if err != nil {
			return err
		}
		if err = build.SaveBuildGeneralDetails(buildName, buildNumber, ddc.buildConfiguration.GetProject()); err != nil {
			return err
		}
	}

	// var downloadParamsArray []services.DirectDownloadParams
	var errorOccurred = false
	var downloadParamsArray []services.DirectDownloadParams
	// Create DownloadParams for all File-Spec groups.
	var downParams services.DirectDownloadParams
	for i := 0; i < len(ddc.Spec().Files); i++ {
		downParams, err = getDirectDownloadParams(ddc.Spec().Get(i), ddc.configuration)
		if err != nil {
			errorOccurred = true
			log.Error(err)
			continue
		}
		downloadParamsArray = append(downloadParamsArray, downParams)
	}
	// Perform download.
	// In case of build-info collection/sync-deletes operation/a detailed summary is required, we use the download service which provides results file reader,
	// otherwise we use the download service which provides only general counters.
	var totalDownloaded, totalFailed int
	var summary *serviceutils.OperationSummary
	if toCollect || ddc.SyncDeletesPath() != "" || ddc.DetailedSummary() {
		summary, err = servicesManager.DirectDownloadFilesWithSummary(downloadParamsArray...)
		if err != nil {
			errorOccurred = true
			log.Error(err)
		}
		if summary != nil {
			defer gofrog.Close(summary.ArtifactsDetailsReader, &err)
			// If 'detailed summary' was requested, then the reader should not be closed here.
			// It will be closed after it will be used to generate the summary.
			if ddc.DetailedSummary() {
				ddc.result.SetReader(summary.TransferDetailsReader)
			} else {
				defer gofrog.Close(summary.TransferDetailsReader, &err)
			}
			totalDownloaded = summary.TotalSucceeded
			totalFailed = summary.TotalFailed
		}
	} else {
		totalDownloaded, totalFailed, err = servicesManager.DirectDownloadFiles(downloadParamsArray...)
		if err != nil {
			errorOccurred = true
			log.Error(err)
		}
	}
	ddc.result.SetSuccessCount(totalDownloaded)
	ddc.result.SetFailCount(totalFailed)
	// Check for errors.
	if errorOccurred {
		return errors.New("download finished with errors, please review the logs")
	}
	if ddc.DryRun() {
		ddc.result.SetSuccessCount(totalDownloaded)
		ddc.result.SetFailCount(0)
		return err
	} else if ddc.SyncDeletesPath() != "" {
		var absSyncDeletesPath string
		absSyncDeletesPath, err = filepath.Abs(ddc.SyncDeletesPath())
		if err != nil {
			return errorutils.CheckError(err)
		}
		if _, err = os.Stat(absSyncDeletesPath); err == nil {
			// Unmarshal the local paths of the downloaded files from the results file reader
			var tmpRoot string
			tmpRoot, err = createDownloadResultEmptyTmpReflection(summary.TransferDetailsReader)
			defer func() {
				err = errors.Join(err, fileutils.RemoveTempDir(tmpRoot))
			}()
			if err != nil {
				return err
			}
			walkFn := createSyncDeletesWalkFunction(tmpRoot)
			err = gofrog.Walk(ddc.SyncDeletesPath(), walkFn, false)
			if err != nil {
				return errorutils.CheckError(err)
			}
		} else if os.IsNotExist(err) {
			log.Info("Sync-deletes path", absSyncDeletesPath, "does not exists.")
		}
	}
	log.Debug("Downloaded", strconv.Itoa(totalDownloaded), "artifacts.")

	// Build Info
	if toCollect {
		var buildName, buildNumber string
		buildName, err = ddc.buildConfiguration.GetBuildName()
		if err != nil {
			return err
		}
		buildNumber, err = ddc.buildConfiguration.GetBuildNumber()
		if err != nil {
			return err
		}
		var buildDependencies []buildinfo.Dependency
		buildDependencies, err = serviceutils.ConvertArtifactsDetailsToBuildInfoDependencies(summary.ArtifactsDetailsReader)
		if err != nil {
			return err
		}
		populateFunc := func(partial *buildinfo.Partial) {
			partial.Dependencies = buildDependencies
			partial.ModuleId = ddc.buildConfiguration.GetModule()
			partial.ModuleType = buildinfo.Generic
		}
		return build.SavePartialBuildInfo(buildName, buildNumber, ddc.buildConfiguration.GetProject(), populateFunc)
	}

	return err
}

func getDirectDownloadParams(f *spec.File, configuration *utils.DownloadConfiguration) (downParams services.DirectDownloadParams, err error) {
	downParams = services.NewDirectDownloadParams()
	downParams.CommonParams, err = f.ToCommonParams()
	if err != nil {
		return
	}
	downParams.MinSplitSize = configuration.MinSplitSize
	downParams.SplitCount = configuration.SplitCount
	downParams.SkipChecksum = configuration.SkipChecksum

	downParams.Recursive, err = f.IsRecursive(true)
	if err != nil {
		return
	}

	downParams.IncludeDirs, err = f.IsIncludeDirs(false)
	if err != nil {
		return
	}

	downParams.Flat, err = f.IsFlat(false)
	if err != nil {
		return
	}

	downParams.Explode, err = f.IsExplode(false)
	if err != nil {
		return
	}

	downParams.ExcludeArtifacts, err = f.IsExcludeArtifacts(false)
	if err != nil {
		return
	}

	downParams.IncludeDeps, err = f.IsIncludeDeps(false)
	if err != nil {
		return
	}

	downParams.Transitive, err = f.IsTransitive(false)
	if err != nil {
		return
	}

	return
}
