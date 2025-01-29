package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/distribution/summary"
	speccore "github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

func GetSpec(c *components.Context, isDownload, overrideFieldsIfSet bool) (specFiles *speccore.SpecFiles, err error) {
	specFiles, err = speccore.CreateSpecFromFile(c.GetStringFlagValue("spec"), coreutils.SpecVarsStringToMap(c.GetStringFlagValue("spec-vars")))
	if err != nil {
		return nil, err
	}
	if isDownload {
		trimPatternPrefix(specFiles)
	}
	if overrideFieldsIfSet {
		overrideSpecFields(c, specFiles)
	}
	return
}

func overrideSpecFields(c *components.Context, specFiles *speccore.SpecFiles) {
	for i := 0; i < len(specFiles.Files); i++ {
		OverrideFieldsIfSet(specFiles.Get(i), c)
	}
}

func trimPatternPrefix(specFiles *speccore.SpecFiles) {
	for i := 0; i < len(specFiles.Files); i++ {
		specFiles.Get(i).Pattern = strings.TrimPrefix(specFiles.Get(i).Pattern, "/")
	}
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

// If `fieldName` exist in the cli args, read it to `field` as a string.
func overrideStringIfSet(field *string, c *components.Context, fieldName string) {
	if c.IsFlagSet(fieldName) {
		*field = c.GetStringFlagValue(fieldName)
	}
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

func PrintBuildInfoSummaryReport(succeeded bool, sha256 string, originalErr error) error {
	success, failed := 1, 0
	if !succeeded {
		success, failed = 0, 1
	}
	buildInfoSummary, mErr := CreateBuildInfoSummaryReportString(success, failed, sha256, originalErr)
	if mErr != nil {
		return summaryPrintError(mErr, originalErr)
	}
	log.Output(buildInfoSummary)
	return summaryPrintError(mErr, originalErr)
}

func CreateBuildInfoSummaryReportString(success, failed int, sha256 string, err error) (string, error) {
	buildInfoSummary := summary.NewBuildInfoSummary(success, failed, sha256, err)
	buildInfoSummaryContent, mErr := buildInfoSummary.Marshal()
	if errorutils.CheckError(mErr) != nil {
		return "", mErr
	}
	return clientutils.IndentJson(buildInfoSummaryContent), mErr
}

// Print summary report.
// a given non-nil error will pass through and be returned as is if no other errors are raised.
// In case of a nil error, the current function error will be returned.
func summaryPrintError(summaryError, originalError error) error {
	if originalError != nil {
		if summaryError != nil {
			log.Error(summaryError)
		}
		return originalError
	}
	return summaryError
}
