package generic

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	buildinfo "github.com/jfrog/build-info-go/entities"
	gofrog "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-client-go/http/httpclient"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
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

func (ddc *DirectDownloadCommand) Run() error {
	return ddc.directDownload()
}

func (ddc *DirectDownloadCommand) directDownload() (err error) {
	if ddc.progress != nil {
		ddc.progress.SetHeadlineMsg("")
		ddc.progress.InitProgressReaders()
	}

	toCollect, err := ddc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	var buildName, buildNumber, buildProject string
	if toCollect && !ddc.DryRun() {
		buildName, err = ddc.buildConfiguration.GetBuildName()
		if err != nil {
			return err
		}
		buildNumber, err = ddc.buildConfiguration.GetBuildNumber()
		if err != nil {
			return err
		}
		buildProject = ddc.buildConfiguration.GetProject()
	}

	var errorOccurred = false
	downloadSuccess := 0
	downloadFailed := 0
	var filesDownloaded []string
	var downloadedArtifacts []string

	for i := 0; i < len(ddc.Spec().Files); i++ {
		currentSpec := ddc.Spec().Get(i)
		repo, artifactPath, err := ddc.parsePatternForDirectAPI(currentSpec.Pattern)
		if err != nil {
			log.Error(err)
			errorOccurred = true
			continue
		}

		if currentSpec.Exclusions != nil && len(currentSpec.Exclusions) > 0 {
			excluded := false
			for _, exclusion := range currentSpec.Exclusions {
				if matched, _ := filepath.Match(exclusion, artifactPath); matched {
					log.Debug("Artifact excluded by pattern:", artifactPath, "matches", exclusion)
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
		}

		if ddc.containsWildcards(artifactPath) {
			count, failed, downloaded, err := ddc.handleWildcardDownload(repo, artifactPath, currentSpec)
			downloadSuccess += count
			downloadFailed += failed
			filesDownloaded = append(filesDownloaded, downloaded...)
			for _, file := range downloaded {
				downloadedArtifacts = append(downloadedArtifacts, repo+"/"+filepath.Base(file))
			}
			if err != nil {
				log.Error(err)
				errorOccurred = true
			}
		} else {
			success, localPath, err := ddc.downloadSingleFile(repo, artifactPath, currentSpec)
			if err != nil {
				log.Error(err)
				errorOccurred = true
				downloadFailed++
			} else if success {
				downloadSuccess++
				if localPath != "" {
					filesDownloaded = append(filesDownloaded, localPath)
					downloadedArtifacts = append(downloadedArtifacts, repo+"/"+artifactPath)
				}
			} else {
				downloadFailed++
			}
		}
	}

	ddc.result.SetSuccessCount(downloadSuccess)
	ddc.result.SetFailCount(downloadFailed)

	if ddc.SyncDeletesPath() != "" && !ddc.DryRun() && downloadSuccess > 0 {
		absSyncDeletesPath, err := filepath.Abs(ddc.SyncDeletesPath())
		if err != nil {
			return errorutils.CheckError(err)
		}
		if _, err = os.Stat(absSyncDeletesPath); err == nil {
			walkFn := ddc.createDirectDownloadSyncDeletesWalkFunction(filesDownloaded)
			err = gofrog.Walk(absSyncDeletesPath, walkFn, false)
			if err != nil {
				return errorutils.CheckError(err)
			}
		} else if os.IsNotExist(err) {
			log.Info("Sync-deletes path", absSyncDeletesPath, "does not exist.")
		}
	}

	if toCollect && len(downloadedArtifacts) > 0 && !ddc.DryRun() {
		populateFn := func(partial *buildinfo.Partial) {
			var dependencies []buildinfo.Dependency
			for _, artifact := range downloadedArtifacts {
				dependency := buildinfo.Dependency{
					Id: filepath.Base(artifact),
				}
				dependencies = append(dependencies, dependency)
			}
			partial.Dependencies = dependencies
			partial.ModuleId = ddc.buildConfiguration.GetModule()
			partial.ModuleType = buildinfo.Generic
		}

		if err := build.SavePartialBuildInfo(buildName, buildNumber, buildProject, populateFn); err != nil {
			log.Error("Failed to save build info:", err)
			errorOccurred = true
		}
	}

	if errorOccurred {
		return errors.New("direct download finished with errors")
	}
	return nil
}

func (ddc *DirectDownloadCommand) downloadSingleFile(repo, artifactPath string, fileSpec *spec.File) (bool, string, error) {
	artifactoryUrl := strings.TrimSuffix(ddc.serverDetails.ArtifactoryUrl, "/")
	downloadUrl := artifactoryUrl + "/" + repo + "/" + artifactPath

	targetPath := fileSpec.Target
	if targetPath == "" {
		targetPath = "./"
	}

	var localPath string
	isFlat, _ := fileSpec.IsFlat(false)
	if isFlat {
		localPath = filepath.Join(targetPath, filepath.Base(artifactPath))
	} else {
		localPath = filepath.Join(targetPath, artifactPath)
	}

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return false, "", errorutils.CheckError(err)
	}

	if ddc.DryRun() {
		log.Info("[Dry run] Would download:", downloadUrl, "to", localPath)
		return true, localPath, nil
	}

	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return false, "", err
	}

	httpClientDetails := httputils.HttpClientDetails{
		User:        ddc.serverDetails.User,
		Password:    ddc.serverDetails.Password,
		AccessToken: ddc.serverDetails.AccessToken,
		Headers:     make(map[string]string),
	}

	resp, bodyBytes, _, err := client.SendGet(downloadUrl, true, httpClientDetails, "")
	if err != nil {
		return false, "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		log.Debug("Artifact not found:", downloadUrl)
		return false, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", errorutils.CheckErrorf("Failed to download %s: HTTP %d", downloadUrl, resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return false, "", errorutils.CheckError(err)
	}
	defer out.Close()

	_, err = out.Write(bodyBytes)
	if err != nil {
		return false, "", errorutils.CheckError(err)
	}

	if !ddc.Configuration().SkipChecksum {
		if err := ddc.validateChecksum(downloadUrl, localPath); err != nil {
			log.Warn("Checksum validation failed for", localPath, ":", err)
		}
	}

	log.Info("Downloaded:", downloadUrl, "to", localPath)
	return true, localPath, nil
}

func (ddc *DirectDownloadCommand) handleWildcardDownload(repo, pattern string, fileSpec *spec.File) (int, int, []string, error) {
	artifactoryUrl := strings.TrimSuffix(ddc.serverDetails.ArtifactoryUrl, "/")

	dir := filepath.Dir(pattern)
	filePattern := filepath.Base(pattern)

	listUrl := artifactoryUrl + "/api/storage/" + repo + "/" + dir

	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return 0, 0, nil, err
	}

	httpClientDetails := httputils.HttpClientDetails{
		User:        ddc.serverDetails.User,
		Password:    ddc.serverDetails.Password,
		AccessToken: ddc.serverDetails.AccessToken,
		Headers:     make(map[string]string),
	}
	resp, bodyBytes, _, err := client.SendGet(listUrl, true, httpClientDetails, "")
	if err != nil {
		return 0, 0, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return 0, 0, nil, errorutils.CheckErrorf("Failed to list directory %s: HTTP %d", listUrl, resp.StatusCode)
	}

	var storageInfo struct {
		Children []struct {
			Uri    string `json:"uri"`
			Folder bool   `json:"folder"`
		} `json:"children"`
	}

	if err := json.Unmarshal(bodyBytes, &storageInfo); err != nil {
		return 0, 0, nil, err
	}

	downloadCount := 0
	failCount := 0
	var downloadedFiles []string

	for _, child := range storageInfo.Children {
		if child.Folder {
			continue
		}

		fileName := strings.TrimPrefix(child.Uri, "/")
		matched, err := filepath.Match(filePattern, fileName)
		if err != nil {
			return downloadCount, failCount, downloadedFiles, err
		}

		if matched {
			excluded := false
			if fileSpec.Exclusions != nil {
				fullPath := filepath.Join(dir, fileName)
				for _, exclusion := range fileSpec.Exclusions {
					if excludeMatched, _ := filepath.Match(exclusion, fullPath); excludeMatched {
						excluded = true
						break
					}
				}
			}

			if !excluded {
				filePath := filepath.Join(dir, fileName)
				success, localPath, err := ddc.downloadSingleFile(repo, filePath, fileSpec)
				if err != nil {
					log.Error("Failed to download", filePath, ":", err)
					failCount++
				} else if success {
					downloadCount++
					if localPath != "" {
						downloadedFiles = append(downloadedFiles, localPath)
					}
				} else {
					failCount++
				}
			}
		}
	}

	return downloadCount, failCount, downloadedFiles, nil
}

func (ddc *DirectDownloadCommand) parsePatternForDirectAPI(pattern string) (repo, artifactPath string, err error) {
	parts := strings.SplitN(pattern, "/", 2)
	if len(parts) < 2 {
		return "", "", errorutils.CheckErrorf("Invalid pattern format. Expected: repo/path/to/artifact, got: %s", pattern)
	}
	return parts[0], parts[1], nil
}

func (ddc *DirectDownloadCommand) containsWildcards(path string) bool {
	return strings.ContainsAny(path, "*?[]")
}

func (ddc *DirectDownloadCommand) validateChecksum(artifactUrl, localPath string) error {
	storageUrl := strings.Replace(artifactUrl, ddc.serverDetails.ArtifactoryUrl, ddc.serverDetails.ArtifactoryUrl+"/api/storage/", 1)

	client, err := httpclient.ClientBuilder().Build()
	if err != nil {
		return err
	}

	httpClientDetails := httputils.HttpClientDetails{
		User:        ddc.serverDetails.User,
		Password:    ddc.serverDetails.Password,
		AccessToken: ddc.serverDetails.AccessToken,
		Headers:     make(map[string]string),
	}
	resp, bodyBytes, _, err := client.SendGet(storageUrl, true, httpClientDetails, "")
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errorutils.CheckErrorf("Failed to get checksum info: HTTP %d", resp.StatusCode)
	}

	var storageInfo struct {
		Checksums struct {
			Sha1   string `json:"sha1"`
			Md5    string `json:"md5"`
			Sha256 string `json:"sha256"`
		} `json:"checksums"`
	}

	if err := json.Unmarshal(bodyBytes, &storageInfo); err != nil {
		return err
	}

	log.Debug("Retrieved checksums - SHA1:", storageInfo.Checksums.Sha1,
		"MD5:", storageInfo.Checksums.Md5, "SHA256:", storageInfo.Checksums.Sha256)

	return nil
}

func (ddc *DirectDownloadCommand) createDirectDownloadSyncDeletesWalkFunction(downloadedFiles []string) gofrog.WalkFunc {
	downloadedMap := make(map[string]bool)
	for _, file := range downloadedFiles {
		absPath, _ := filepath.Abs(file)
		downloadedMap[absPath] = true
	}

	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		if downloadedMap[absPath] {
			return nil
		}

		if info.IsDir() {
			log.Info("Deleting directory:", path)
			return fileutils.RemoveTempDir(path)
		} else {
			log.Info("Deleting file:", path)
			return os.Remove(path)
		}
	}
}
