package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func ShouldRunNative(configPath string) bool {
	return buildinfoflexpack.IsFlexPackEnabled() && configPath == ""
}

func CreateDownloadConfiguration(c *components.Context) (downloadConfiguration *artifactoryUtils.DownloadConfiguration, err error) {
	downloadConfiguration = new(artifactoryUtils.DownloadConfiguration)
	downloadConfiguration.MinSplitSize, err = getMinSplit(c, flagkit.DownloadMinSplitKb)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SplitCount, err = getSplitCount(c, flagkit.DownloadSplitCount, flagkit.DownloadMaxSplitCount)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.Threads, err = common.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}
	downloadConfiguration.SkipChecksum = c.GetBoolFlagValue("skip-checksum")
	downloadConfiguration.Symlink = true
	return
}

func getMinSplit(c *components.Context, defaultMinSplit int64) (minSplitSize int64, err error) {
	minSplitSize = defaultMinSplit
	if c.GetStringFlagValue(flagkit.MinSplit) != "" {
		minSplitSize, err = strconv.ParseInt(c.GetStringFlagValue(flagkit.MinSplit), 10, 64)
		if err != nil {
			err = errors.New("The '--min-split' option should have a numeric value. " + common.GetDocumentationMessage())
			return 0, err
		}
	}
	return minSplitSize, nil
}

func getSplitCount(c *components.Context, defaultSplitCount, maxSplitCount int) (splitCount int, err error) {
	splitCount = defaultSplitCount
	err = nil
	if c.GetStringFlagValue("split-count") != "" {
		splitCount, err = strconv.Atoi(c.GetStringFlagValue("split-count"))
		if err != nil {
			err = errors.New("The '--split-count' option should have a numeric value. " + common.GetDocumentationMessage())
		}
		if splitCount > maxSplitCount {
			err = errors.New("The '--split-count' option value is limited to a maximum of " + strconv.Itoa(maxSplitCount) + ".")
		}
		if splitCount < 0 {
			err = errors.New("the '--split-count' option cannot have a negative value")
		}
	}
	return
}

func CreateUploadConfiguration(c *components.Context) (uploadConfiguration *artifactoryUtils.UploadConfiguration, err error) {
	uploadConfiguration = new(artifactoryUtils.UploadConfiguration)
	uploadConfiguration.MinSplitSizeMB, err = getMinSplit(c, flagkit.UploadMinSplitMb)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.ChunkSizeMB, err = getUploadChunkSize(c, flagkit.UploadChunkSizeMb)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.SplitCount, err = getSplitCount(c, flagkit.UploadSplitCount, flagkit.UploadMaxSplitCount)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.Threads, err = common.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}
	uploadConfiguration.Deb, err = getDebFlag(c)
	if err != nil {
		return
	}
	return
}

func getUploadChunkSize(c *components.Context, defaultChunkSize int64) (chunkSize int64, err error) {
	chunkSize = defaultChunkSize
	if c.GetStringFlagValue(flagkit.ChunkSize) != "" {
		chunkSize, err = strconv.ParseInt(c.GetStringFlagValue(flagkit.ChunkSize), 10, 64)
		if err != nil {
			err = fmt.Errorf("the '--%s' option should have a numeric value. %s", flagkit.ChunkSize, common.GetDocumentationMessage())
			return 0, err
		}
	}

	return chunkSize, nil
}

func getDebFlag(c *components.Context) (deb string, err error) {
	deb = c.GetStringFlagValue("deb")
	slashesCount := strings.Count(deb, "/") - strings.Count(deb, "\\/")
	if deb != "" && slashesCount != 2 {
		return "", errors.New("the --deb option should be in the form of distribution/component/architecture")
	}
	return deb, nil
}
