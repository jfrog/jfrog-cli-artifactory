package utils

import (
	"errors"
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	speccore "github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strconv"
	"strings"
)

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

func GetFileSystemSpec(c *components.Context) (fsSpec *speccore.SpecFiles, err error) {
	fsSpec, err = speccore.CreateSpecFromFile(c.GetStringFlagValue("spec"), coreutils.SpecVarsStringToMap(c.GetStringFlagValue("spec-vars")))
	if err != nil {
		return
	}
	// Override spec with CLI options
	for i := 0; i < len(fsSpec.Files); i++ {
		fsSpec.Get(i).Target = strings.TrimPrefix(fsSpec.Get(i).Target, "/")
		OverrideFieldsIfSet(fsSpec.Get(i), c)
	}
	return
}

func OverrideFieldsIfSet(spec *speccore.File, c *components.Context) {
	overrideArrayIfSet(&spec.Exclusions, c, "exclusions")
	overrideArrayIfSet(&spec.SortBy, c, "sort-by")
	overrideIntIfSet(&spec.Offset, c, "offset")
	overrideIntIfSet(&spec.Limit, c, "limit")
	overrideStringIfSet(&spec.SortOrder, c, "sort-order")
	overrideStringIfSet(&spec.Props, c, "props")
	overrideStringIfSet(&spec.TargetProps, c, "target-props")
	overrideStringIfSet(&spec.ExcludeProps, c, "exclude-props")
	overrideStringIfSet(&spec.Build, c, "build")
	overrideStringIfSet(&spec.Project, c, "project")
	overrideStringIfSet(&spec.ExcludeArtifacts, c, "exclude-artifacts")
	overrideStringIfSet(&spec.IncludeDeps, c, "include-deps")
	overrideStringIfSet(&spec.Bundle, c, "bundle")
	overrideStringIfSet(&spec.Recursive, c, "recursive")
	overrideStringIfSet(&spec.Flat, c, "flat")
	overrideStringIfSet(&spec.Explode, c, "explode")
	overrideStringIfSet(&spec.BypassArchiveInspection, c, "bypass-archive-inspection")
	overrideStringIfSet(&spec.Regexp, c, "regexp")
	overrideStringIfSet(&spec.IncludeDirs, c, "include-dirs")
	overrideStringIfSet(&spec.ValidateSymlinks, c, "validate-symlinks")
	overrideStringIfSet(&spec.Symlinks, c, "symlinks")
	overrideStringIfSet(&spec.Transitive, c, "transitive")
	overrideStringIfSet(&spec.PublicGpgKey, c, "gpg-key")
}

// If `fieldName` exist in the cli args, read it to `field` as an array split by `;`.
func overrideArrayIfSet(field *[]string, c *components.Context, fieldName string) {
	if c.IsFlagSet(fieldName) {
		*field = append([]string{}, strings.Split(c.GetStringFlagValue(fieldName), ";")...)
	}
}

// If `fieldName` exist in the cli args, read it to `field` as a int.
func overrideIntIfSet(field *int, c *components.Context, fieldName string) {
	if c.IsFlagSet(fieldName) {
		*field, _ = c.GetIntFlagValue(fieldName)
	}
}

// If `fieldName` exist in the cli args, read it to `field` as a string.
func overrideStringIfSet(field *string, c *components.Context, fieldName string) {
	if c.IsFlagSet(fieldName) {
		*field = c.GetStringFlagValue(fieldName)
	}
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

func LogCommandRemovalNotice(cmdName, oldSubCommand string) error {
	log.Warn(
		`The command syntax you are using has been decommissioned and is no longer available.
	Instead of:
	$ ` + coreutils.GetCliExecutableName() + ` ` + oldSubCommand + ` ` + cmdName + ` ...
	Use:
	$ ` + coreutils.GetCliExecutableName() + ` ` + cmdName + ` ...`)
	return nil
}
