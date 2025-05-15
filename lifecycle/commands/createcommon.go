package commands

import (
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strconv"
	"strings"
)

const (
	missingCreationSourcesErrMsg      = "unexpected err while validating spec - could not detect any creation sources"
	multipleCreationSourcesErrMsg     = "multiple creation sources were detected in separate spec files. Only a single creation source should be provided. Detected:"
	singleAqlErrMsg                   = "only a single aql query can be provided"
	minMultiSourcesArtifactoryVersion = "7.114.0"
)

type ReleaseBundleCreateSources struct {
	sourcesBuilds         string
	sourcesReleaseBundles string
}

type ReleaseBundleCreateCommand struct {
	releaseBundleCmd
	signingKeyName string
	spec           *spec.SpecFiles
	// Backward compatibility:
	buildsSpecPath         string
	releaseBundlesSpecPath string

	// Multi-source support
	ReleaseBundleCreateSources
}

func NewReleaseBundleCreateCommand() *ReleaseBundleCreateCommand {
	return &ReleaseBundleCreateCommand{}
}

func (rbc *ReleaseBundleCreateCommand) SetServerDetails(serverDetails *config.ServerDetails) *ReleaseBundleCreateCommand {
	rbc.serverDetails = serverDetails
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetReleaseBundleName(releaseBundleName string) *ReleaseBundleCreateCommand {
	rbc.releaseBundleName = releaseBundleName
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetReleaseBundleVersion(releaseBundleVersion string) *ReleaseBundleCreateCommand {
	rbc.releaseBundleVersion = releaseBundleVersion
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetSigningKeyName(signingKeyName string) *ReleaseBundleCreateCommand {
	rbc.signingKeyName = signingKeyName
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetSync(sync bool) *ReleaseBundleCreateCommand {
	rbc.sync = sync
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetReleaseBundleProject(rbProjectKey string) *ReleaseBundleCreateCommand {
	rbc.rbProjectKey = rbProjectKey
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetSpec(spec *spec.SpecFiles) *ReleaseBundleCreateCommand {
	rbc.spec = spec
	return rbc
}

// Deprecated
func (rbc *ReleaseBundleCreateCommand) SetBuildsSpecPath(buildsSpecPath string) *ReleaseBundleCreateCommand {
	rbc.buildsSpecPath = buildsSpecPath
	return rbc
}

// Deprecated
func (rbc *ReleaseBundleCreateCommand) SetReleaseBundlesSpecPath(releaseBundlesSpecPath string) *ReleaseBundleCreateCommand {
	rbc.releaseBundlesSpecPath = releaseBundlesSpecPath
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetSourcesBuilds(sourcesBuilds string) *ReleaseBundleCreateCommand {
	rbc.ReleaseBundleCreateSources.sourcesBuilds = sourcesBuilds
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetSourcesReleaseBundles(sourcesReleaseBundles string) *ReleaseBundleCreateCommand {
	rbc.ReleaseBundleCreateSources.sourcesReleaseBundles = sourcesReleaseBundles
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) CommandName() string {
	return "rb_create"
}

func (rbc *ReleaseBundleCreateCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbc.serverDetails, nil
}

func (rbc *ReleaseBundleCreateCommand) Run() error {
	if err := validateArtifactoryVersionSupported(rbc.serverDetails); err != nil {
		return err
	}

	servicesManager, rbDetails, queryParams, err := rbc.getPrerequisites()
	if err != nil {
		return err
	}

	var isSources bool
	if err := ValidateFeatureSupportedVersion(rbc.serverDetails, minMultiSourcesArtifactoryVersion); err != nil {
		isSources = false
	} else {
		isSources = rbc.sourcesDefined()
	}

	sourceType, err := rbc.identifySourceType()
	if err != nil {
		if isSources {
			log.Debug("multiple sources were defined")
		} else {
			return err
		}
	}

	if !isSources {
		switch sourceType {
		case services.Aql:
			return rbc.createFromAql(servicesManager, rbDetails, queryParams)
		case services.Artifacts:
			return rbc.createFromArtifacts(servicesManager, rbDetails, queryParams)
		case services.Builds:
			return rbc.createFromBuilds(servicesManager, rbDetails, queryParams)
		case services.ReleaseBundles:
			return rbc.createFromReleaseBundles(servicesManager, rbDetails, queryParams)
		default:
			return errorutils.CheckErrorf("unknown source for release bundle creation was provided")
		}
	}
	sources := buildReleaseBundleSourcesParams(rbc)
	return rbc.createFromMultipleSources(servicesManager, rbDetails, queryParams, sources)
}

func buildRbBuildsSources(sourcesStr, projectKey string, sources []services.RbSource) []services.RbSource {
	var buildSources []services.BuildSource
	buildEntries := strings.Split(sourcesStr, ";")
	for _, entry := range buildEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Assuming the format "name=xxx, number=xxx, include-dep=true"
		components := strings.Split(entry, ",")
		if len(components) < 2 {
			continue
		}

		name := strings.TrimSpace(strings.Split(components[0], "=")[1])
		number := strings.TrimSpace(strings.Split(components[1], "=")[1])

		includeDepStr := "false"
		if len(components) >= 3 {
			parts := strings.Split(components[2], "=")
			if len(parts) > 1 {
				includeDepStr = strings.TrimSpace(parts[1])
			}
		}

		includeDep, _ := strconv.ParseBool(includeDepStr)

		buildSources = append(buildSources, services.BuildSource{
			BuildRepository:     utils.GetBuildInfoRepositoryByProject(projectKey),
			BuildName:           name,
			BuildNumber:         number,
			IncludeDependencies: includeDep,
		})
	}
	if len(buildSources) > 0 {
		sources = append(sources, services.RbSource{
			SourceType: "builds",
			Builds:     buildSources,
		})
	}
	return sources
}

func buildRbReleaseBundlesSources(sourcesStr, projectKey string, sources []services.RbSource) []services.RbSource {
	var releaseBundleSources []services.ReleaseBundleSource
	bundleEntries := strings.Split(sourcesStr, ";")
	for _, entry := range bundleEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Assuming the format "name=xxx, version=xxx"
		components := strings.Split(entry, ",")
		if len(components) != 2 {
			continue
		}
		name := strings.TrimSpace(strings.Split(components[0], "=")[1])
		version := strings.TrimSpace(strings.Split(components[1], "=")[1])

		releaseBundleSources = append(releaseBundleSources, services.ReleaseBundleSource{
			ProjectKey:           projectKey,
			ReleaseBundleName:    name,
			ReleaseBundleVersion: version,
		})
	}
	if len(releaseBundleSources) > 0 {
		sources = append(sources, services.RbSource{
			SourceType:     "release_bundles",
			ReleaseBundles: releaseBundleSources,
		})
	}
	return sources
}

func buildReleaseBundleSourcesParams(rbc *ReleaseBundleCreateCommand) []services.RbSource {
	var sources []services.RbSource
	// Process Builds
	if rbc.sourcesBuilds != "" {
		sources = buildRbBuildsSources(rbc.sourcesBuilds, rbc.rbProjectKey, sources)
	}

	// Process Release Bundles
	if rbc.sourcesReleaseBundles != "" {
		sources = buildRbReleaseBundlesSources(rbc.sourcesReleaseBundles, rbc.rbProjectKey, sources)
	}

	return sources

}

func (rbc *ReleaseBundleCreateCommand) identifySourceType() (services.SourceType, error) {
	switch {
	case rbc.buildsSpecPath != "":
		return services.Builds, nil
	case rbc.releaseBundlesSpecPath != "":
		return services.ReleaseBundles, nil
	case rbc.spec != nil:
		return validateAndIdentifyRbCreationSpec(rbc.spec.Files)
	default:
		return "", errorutils.CheckErrorf("a spec file input is mandatory")
	}
}

func (rbc *ReleaseBundleCreateCommand) sourcesDefined() bool {
	return rbc.ReleaseBundleCreateSources.sourcesReleaseBundles != "" || rbc.ReleaseBundleCreateSources.sourcesBuilds != ""
}

func (rbc *ReleaseBundleCreateCommand) createFromMultipleSources(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams,
	sources []services.RbSource) error {
	return servicesManager.CreateReleaseBundlesFromMultipleSources(rbDetails, queryParams, rbc.signingKeyName, sources)
}

func validateAndIdentifyRbCreationSpec(files []spec.File) (services.SourceType, error) {
	if len(files) == 0 {
		return "", errorutils.CheckErrorf("spec must include at least one file group")
	}

	var detectedCreationSources []services.SourceType
	for _, file := range files {
		sourceType, err := validateFile(file)
		if err != nil {
			return "", err
		}
		detectedCreationSources = append(detectedCreationSources, sourceType)
	}

	if err := validateCreationSources(detectedCreationSources); err != nil {
		return "", err
	}
	return detectedCreationSources[0], nil
}

func validateCreationSources(detectedCreationSources []services.SourceType) error {
	if len(detectedCreationSources) == 0 {
		return errorutils.CheckErrorf(missingCreationSourcesErrMsg)
	}

	// Assert single creation source.
	for i := 1; i < len(detectedCreationSources); i++ {
		if detectedCreationSources[i] != detectedCreationSources[0] {
			return generateSingleCreationSourceErr(detectedCreationSources)
		}
	}

	// If aql, assert single file.
	if detectedCreationSources[0] == services.Aql && len(detectedCreationSources) > 1 {
		return errorutils.CheckErrorf(singleAqlErrMsg)
	}
	return nil
}

func generateSingleCreationSourceErr(detectedCreationSources []services.SourceType) error {
	var detectedStr []string
	for _, source := range detectedCreationSources {
		detectedStr = append(detectedStr, string(source))
	}
	return errorutils.CheckErrorf(
		"%s '%s'", multipleCreationSourcesErrMsg, coreutils.ListToText(detectedStr))
}

func validateFile(file spec.File) (services.SourceType, error) {
	// Aql creation source:
	isAql := len(file.Aql.ItemsFind) > 0

	// Build creation source:
	isBuild := len(file.Build) > 0
	isIncludeDeps, _ := file.IsIncludeDeps(false)

	// Bundle creation source:
	isBundle := len(file.Bundle) > 0

	// Build & bundle:
	isProject := len(file.Project) > 0

	// Artifacts creation source:
	isPattern := len(file.Pattern) > 0
	isExclusions := len(file.Exclusions) > 0 && len(file.Exclusions[0]) > 0
	isProps := len(file.Props) > 0
	isExcludeProps := len(file.ExcludeProps) > 0
	isRecursive, err := file.IsRecursive(true)
	if err != nil {
		return "", errorutils.CheckErrorf("invalid value provided to the 'recursive' field. error: %s", err.Error())
	}

	// Unsupported:
	isPathMapping := len(file.PathMapping.Input) > 0 || len(file.PathMapping.Output) > 0
	isTarget := len(file.Target) > 0
	isSortOrder := len(file.SortOrder) > 0
	isSortBy := len(file.SortBy) > 0
	isExcludeArtifacts, _ := file.IsExcludeArtifacts(false)
	isGPGKey := len(file.PublicGpgKey) > 0
	isOffset := file.Offset > 0
	isLimit := file.Limit > 0
	isArchive := len(file.Archive) > 0
	isSymlinks, _ := file.IsSymlinks(false)
	isRegexp := file.Regexp == "true"
	isAnt := file.Ant == "true"
	isExplode, _ := file.IsExplode(false)
	isBypassArchiveInspection, _ := file.IsBypassArchiveInspection(false)
	isTransitive, _ := file.IsTransitive(false)

	if isPathMapping || isTarget || isSortOrder || isSortBy || isExcludeArtifacts || isGPGKey || isOffset || isLimit ||
		isSymlinks || isArchive || isAnt || isRegexp || isExplode || isBypassArchiveInspection || isTransitive {
		return "", errorutils.CheckErrorf("unsupported fields were provided in file spec. " +
			"release bundle creation file spec only supports the following fields: " +
			"'aql', 'build', 'includeDeps', 'bundle', 'project', 'pattern', 'exclusions', 'props', 'excludeProps' and 'recursive'")
	}
	if coreutils.SumTrueValues([]bool{isAql, isBuild, isBundle, isPattern}) != 1 {
		return "", errorutils.CheckErrorf("exactly one creation source is supported (aql, builds, release bundles or pattern (artifacts))")
	}

	switch {
	case isAql:
		return services.Aql,
			validateCreationSource([]bool{isIncludeDeps, isProject, isExclusions, isProps, isExcludeProps, !isRecursive},
				"aql creation source supports no other fields")
	case isBuild:
		return services.Builds,
			validateCreationSource([]bool{isExclusions, isProps, isExcludeProps, !isRecursive},
				"builds creation source only supports the 'includeDeps' and 'project' fields")
	case isBundle:
		return services.ReleaseBundles,
			validateCreationSource([]bool{isIncludeDeps, isExclusions, isProps, isExcludeProps, !isRecursive},
				"release bundles creation source only supports the 'project' field")
	case isPattern:
		return services.Artifacts,
			validateCreationSource([]bool{isIncludeDeps, isProject},
				"release bundles creation source only supports the 'exclusions', 'props', 'excludeProps' and 'recursive' fields")
	default:
		return "", errorutils.CheckErrorf("unexpected err in spec validation")
	}
}

func validateCreationSource(unsupportedFields []bool, errMsg string) error {
	if coreutils.SumTrueValues(unsupportedFields) > 0 {
		return errorutils.CheckErrorf(errMsg)
	}
	return nil
}
