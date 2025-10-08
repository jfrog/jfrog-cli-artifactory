package generic

import (
	"fmt"
	goio "io"
	"os"
	"path/filepath"

	"github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
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

func (ddc *DirectDownloadCommand) ShouldPrompt() bool {
	return !ddc.DryRun() && ddc.SyncDeletesPath() != "" && !ddc.Quiet()
}

func (ddc *DirectDownloadCommand) Run() error {
	if err := ddc.directDownload(); err != nil {
		return err
	}
	return nil
}

func (ddc *DirectDownloadCommand) directDownload() error {
	// Use the threads configuration from the download command
	threads := 3 // Default threads
	if ddc.configuration != nil && ddc.configuration.Threads > 0 {
		threads = ddc.configuration.Threads
	}

	servicesManager, err := utils.CreateServiceManager(ddc.serverDetails, threads, 0, ddc.DryRun())
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

		// Set split download parameters from configuration
		if ddc.configuration != nil {
			downloadParams.MinSplitSizeMB = ddc.configuration.MinSplitSize / 1024 // Convert KB to MB
			downloadParams.SplitCount = ddc.configuration.SplitCount
		}

		downloadParamsArray = append(downloadParamsArray, *downloadParams)
	}

	var summary *serviceutils.OperationSummary
	// We need the detailed summary when either showing detailed output or performing sync-deletes
	// (sync-deletes needs the file list to know what to keep)
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

	if summary.TransferDetailsReader != nil {
		ddc.result.SetReader(summary.TransferDetailsReader)
	}

	if !ddc.DryRun() && ddc.SyncDeletesPath() != "" && summary.TransferDetailsReader != nil {
		if err = ddc.handleSyncDeletes(summary.TransferDetailsReader); err != nil {
			return err
		}
	}

	return nil
}

// handleSyncDeletes removes local files that weren't part of the current download operation.
// It creates a temporary directory structure mirroring downloaded files, then walks the target
// directory to identify and delete files that don't exist in the download set.
func (ddc *DirectDownloadCommand) handleSyncDeletes(reader *content.ContentReader) error {
	reader.Reset()

	absSyncDeletesPath, err := filepath.Abs(ddc.SyncDeletesPath())
	if err != nil {
		return errorutils.CheckError(err)
	}

	if _, err = os.Stat(absSyncDeletesPath); err != nil {
		if os.IsNotExist(err) {
			log.Info("Sync-deletes path", absSyncDeletesPath, "does not exist.")
			return nil
		}
		return errorutils.CheckError(err)
	}

	tmpRoot, err := fileutils.CreateTempDir()
	if err != nil {
		return errorutils.CheckError(err)
	}
	defer func() {
		if removeErr := fileutils.RemoveTempDir(tmpRoot); removeErr != nil {
			log.Error("Failed to remove temp directory:", removeErr)
		}
	}()

	fileCount := 0

	for transferDetails := new(clientutils.FileTransferDetails); ; transferDetails = new(clientutils.FileTransferDetails) {
		err := reader.NextRecord(transferDetails)
		if err == goio.EOF {
			break
		}
		if err != nil {
			log.Error("Error reading transfer details:", err)
			log.Error("Error type:", fmt.Sprintf("%T", err))
			return err
		}
		fileCount++
		if transferDetails.TargetPath != "" {
			tempPath := createLegalPathDDL(tmpRoot, transferDetails.TargetPath)

			parentDir := filepath.Dir(tempPath)
			if err = os.MkdirAll(parentDir, 0755); err != nil {
				return errorutils.CheckError(err)
			}

			file, err := os.Create(tempPath)
			if err != nil {
				return errorutils.CheckError(err)
			}
			file.Close()
		}
	}

	walkFn := createSyncDeletesWalkFunctionDDL(tmpRoot, absSyncDeletesPath, ddc.SyncDeletesPath())
	return io.Walk(ddc.SyncDeletesPath(), walkFn, false)
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

func createLegalPathDDL(root, path string) string {
	volumeName := filepath.VolumeName(path)
	if volumeName != "" && filepath.IsAbs(path) {
		alternativeVolumeName := "VolumeName" + string(volumeName[0])
		path = filepath.Clean(path)
		path = alternativeVolumeName + path[len(volumeName):]
	}
	path = filepath.Join(root, path)
	return path
}

func createSyncDeletesWalkFunctionDDL(tempRoot string, syncDeletesRoot string, syncDeletesPath string) io.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		absPath, err := filepath.Abs(path)
		if errorutils.CheckError(err) != nil {
			return err
		}

		if absPath == syncDeletesRoot && info.IsDir() {
			return nil
		}

		pathToCheck := filepath.Join(tempRoot, path)

		if fileutils.IsPathExists(pathToCheck, false) {
			return nil
		}
		if info.IsDir() {
			err = fileutils.RemoveTempDir(absPath)
			if err == nil {
				return io.ErrSkipDir
			}
		} else {
			err = os.Remove(absPath)
		}

		return errorutils.CheckError(err)
	}
}
