package commands

import (
	"errors"
	"fmt"
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
	missingCreationSourcesErrMsg    = "unexpected err while validating spec - could not detect any creation sources"
	multipleCreationSourcesErrMsg   = "multiple creation sources were detected in separate spec files. Only a single creation source should be provided. Detected:"
	singleAqlErrMsg                 = "only a single aql query can be provided"
	unsupportedCreationSourceMethod = "creation source 'package' is not supported in current version"
)

type SourceTypeSet map[services.SourceType]bool

type ReleaseBundleSources struct {
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

	// Multi-bundles and multi-builds sources from command-line
	ReleaseBundleSources
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

func (rbc *ReleaseBundleCreateCommand) SetBuildsSources(sourcesBuilds string) *ReleaseBundleCreateCommand {
	rbc.ReleaseBundleSources.sourcesBuilds = sourcesBuilds
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) SetReleaseBundlesSources(sourcesReleaseBundles string) *ReleaseBundleCreateCommand {
	rbc.ReleaseBundleSources.sourcesReleaseBundles = sourcesReleaseBundles
	return rbc
}

func (rbc *ReleaseBundleCreateCommand) CommandName() string {
	return "rb_create"
}

func (rbc *ReleaseBundleCreateCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbc.serverDetails, nil
}

func isSingleSourceType(sourceTypes []services.SourceType) bool {
	if len(sourceTypes) == 1 {
		return true
	}

	first := sourceTypes[0]
	for _, t := range sourceTypes[1:] {
		if t != first {
			return false
		}
	}
	return true
}

func (rbc *ReleaseBundleCreateCommand) Run() error {
	if err := validateArtifactoryVersionSupported(rbc.serverDetails); err != nil {
		return err
	}

	servicesManager, rbDetails, queryParams, err := rbc.getPrerequisites()
	if err != nil {
		return err
	}

	var isReleaseBundleCreationWithMultiSourcesSupported bool
	if err = ValidateFeatureSupportedVersion(rbc.serverDetails, minArtifactoryVersionForMultiSourceAndPackagesSupport); err != nil {
		isReleaseBundleCreationWithMultiSourcesSupported = false
	} else {
		isReleaseBundleCreationWithMultiSourcesSupported = true
	}

	sourceTypes, err := rbc.identifySourceTypeBySpecOrByLegacyCommands(isReleaseBundleCreationWithMultiSourcesSupported)
	if err != nil {
		return err
	}

	if sourceTypes != nil && isSingleSourceType(sourceTypes) {
		switch sourceTypes[0] {
		case services.Aql:
			return rbc.createFromAql(servicesManager, rbDetails, queryParams)
		case services.Artifacts:
			return rbc.createFromArtifacts(servicesManager, rbDetails, queryParams)
		case services.Builds:
			return rbc.createFromBuilds(servicesManager, rbDetails, queryParams)
		case services.ReleaseBundles:
			return rbc.createFromReleaseBundles(servicesManager, rbDetails, queryParams)
		case services.Packages:
			return rbc.createFromPackages(servicesManager, rbDetails, queryParams)
		default:
			return errorutils.CheckError(errors.New("unknown source for release bundle creation was provided"))
		}
	}

	if isReleaseBundleCreationWithMultiSourcesSupported {
		sources, err := rbc.getMultipleSourcesIfDefined()
		if err != nil {
			return err
		}
		_, err = rbc.createFromMultipleSources(servicesManager, rbDetails, queryParams, sources)
		return err
	}

	return errorutils.CheckErrorf("release bundle creation failed, unable to identify source for creation")
}

func parseKeyValueString(input string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(input, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			log.Warn("Inappropriate format, it should be k=v")
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		result[key] = value
	}
	return result
}

func buildRbBuildsSources(sourcesStr, projectKey string, sources []services.RbSource) []services.RbSource {
	var buildSources []services.BuildSource
	buildEntries := strings.Split(sourcesStr, ";")
	for _, entry := range buildEntries {
		buildInfoMap := parseKeyValueString(entry)
		includeDepStr := buildInfoMap["include-deps"]
		includeDep, _ := strconv.ParseBool(includeDepStr)
		buildSources = append(buildSources, services.BuildSource{
			BuildRepository:     utils.GetBuildInfoRepositoryByProject(projectKey),
			BuildName:           buildInfoMap["name"],
			BuildNumber:         buildInfoMap["id"],
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
		buildInfoMap := parseKeyValueString(entry)
		releaseBundleSources = append(releaseBundleSources, services.ReleaseBundleSource{
			ProjectKey:           projectKey,
			ReleaseBundleName:    buildInfoMap["name"],
			ReleaseBundleVersion: buildInfoMap["version"],
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

func buildReleaseBundleSourcesParams(rbc *ReleaseBundleCreateCommand) (sources []services.RbSource) {

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

func buildReleaseBundleSourcesParamsFromSpec(rbc *ReleaseBundleCreateCommand, detectedSources []services.SourceType) ([]services.RbSource, error) {
	var sources []services.RbSource
	var err error

	// Track handled source types to avoid duplication
	handledSources := make(SourceTypeSet)

	for _, sourceType := range detectedSources {
		if handledSources[sourceType] {
			continue
		}

		handledSources[sourceType] = true

		switch sourceType {
		case services.ReleaseBundles:
			if sources, err = appendReleaseBundlesSource(rbc, sources); err != nil {
				return nil, err
			}
		case services.Builds:
			if sources, err = appendBuildsSource(rbc, sources); err != nil {
				return nil, err
			}
		case services.Artifacts:
			if sources, err = appendArtifactsSource(rbc, sources); err != nil {
				return nil, err
			}
		case services.Packages:
			sources = appendPackagesSource(rbc, sources)
		case services.Aql:
			sources = appendAqlSource(rbc, sources)
		default:
			return nil, errorutils.CheckError(errors.New("unexpected source type: " + string(sourceType)))
		}
	}

	return sources, nil
}

func appendReleaseBundlesSource(rbc *ReleaseBundleCreateCommand, sources []services.RbSource) ([]services.RbSource, error) {
	err, releaseBundleSources := rbc.createReleaseBundleSourceFromSpec()
	if err != nil {
		return nil, err
	}
	if len(releaseBundleSources.ReleaseBundles) > 0 {
		sources = append(sources, services.RbSource{
			SourceType:     services.ReleaseBundles,
			ReleaseBundles: releaseBundleSources.ReleaseBundles,
		})
	}
	return sources, nil
}

func appendBuildsSource(rbc *ReleaseBundleCreateCommand, sources []services.RbSource) ([]services.RbSource, error) {
	err, buildsSource := rbc.createBuildSourceFromSpec()
	if err != nil {
		return nil, err
	}
	if len(buildsSource.Builds) > 0 {
		sources = append(sources, services.RbSource{
			SourceType: services.Builds,
			Builds:     buildsSource.Builds,
		})
	}
	return sources, nil
}

func appendArtifactsSource(rbc *ReleaseBundleCreateCommand, sources []services.RbSource) ([]services.RbSource, error) {
	artifactsSource, err := rbc.createArtifactSourceFromSpec()
	if err != nil {
		return nil, err
	}
	if len(artifactsSource.Artifacts) > 0 {
		sources = append(sources, services.RbSource{
			SourceType: services.Artifacts,
			Artifacts:  artifactsSource.Artifacts,
		})
	}
	return sources, nil
}

func appendPackagesSource(rbc *ReleaseBundleCreateCommand, sources []services.RbSource) []services.RbSource {
	packagesSource := rbc.createPackageSourceFromSpec()
	if len(packagesSource.Packages) > 0 {
		sources = append(sources, services.RbSource{
			SourceType: services.Packages,
			Packages:   packagesSource.Packages,
		})
	}
	return sources
}

func appendAqlSource(rbc *ReleaseBundleCreateCommand, sources []services.RbSource) []services.RbSource {
	aqlQuery := rbc.createAqlQueryFromSpec()
	sources = append(sources, services.RbSource{
		SourceType: services.Aql,
		Aql:        aqlQuery,
	})
	return sources
}

func (rbc *ReleaseBundleCreateCommand) identifySourceTypeBySpecOrByLegacyCommands(multipleSourcesAndPackagesSupported bool) (sourceTypes []services.SourceType, err error) {
	if rbc.buildsSpecPath != "" {
		sourceTypes = append(sourceTypes, services.Builds)
	}

	if rbc.releaseBundlesSpecPath != "" {
		sourceTypes = append(sourceTypes, services.ReleaseBundles)
	}

	if rbc.spec != nil {
		return validateAndIdentifyRbCreationSpec(rbc.spec.Files, multipleSourcesAndPackagesSupported)
	}

	return sourceTypes, err
}

func (rbc *ReleaseBundleCreateCommand) atLeastASingleMultiSourceRBDefinedFromCommand() bool {
	return rbc.ReleaseBundleSources.sourcesReleaseBundles != "" || rbc.ReleaseBundleSources.sourcesBuilds != ""
}

func (rbc *ReleaseBundleCreateCommand) multiSourcesDefinedFromSpec() ([]services.SourceType, error) {
	if rbc.spec != nil && rbc.spec.Files != nil {
		detectedCreationSources, err := detectSourceTypesFromSpec(rbc.spec.Files, true)
		if err != nil {
			return nil, err
		}
		if err = validateCreationSources(detectedCreationSources, true); err != nil {
			return nil, err
		}

		return detectedCreationSources, nil
	}
	return nil, errorutils.CheckError(errors.New("no spec file input"))
}

func (rbc *ReleaseBundleCreateCommand) createFromMultipleSources(servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams,
	sources []services.RbSource) (response []byte, err error) {
	return servicesManager.CreateReleaseBundlesFromMultipleSources(rbDetails, queryParams, rbc.signingKeyName, sources)
}

func (rbc *ReleaseBundleCreateCommand) createAqlQueryFromSpec() (aql string) {
	for _, file := range rbc.spec.Files {
		if len(file.Aql.ItemsFind) > 0 {
			aql = fmt.Sprintf(`items.find(%s)`, file.Aql.ItemsFind)
			break
		}
	}
	return aql
}

func (rbc *ReleaseBundleCreateCommand) getMultipleSourcesIfDefined() ([]services.RbSource, error) {
	if rbc.atLeastASingleMultiSourceRBDefinedFromCommand() {
		return buildReleaseBundleSourcesParams(rbc), nil
	} else {
		detectedCreationSources, err := rbc.multiSourcesDefinedFromSpec()
		if err != nil {
			return nil, err
		}

		return buildReleaseBundleSourcesParamsFromSpec(rbc, detectedCreationSources)
	}
}

func detectSourceTypesFromSpec(files []spec.File, multiSrcAndPackageSupported bool) ([]services.SourceType, error) {
	var detectedCreationSources []services.SourceType
	for _, file := range files {
		sourceType, err := validateFile(file, multiSrcAndPackageSupported)
		if err != nil {
			return nil, err
		}
		detectedCreationSources = append(detectedCreationSources, sourceType)
	}
	return detectedCreationSources, nil
}

func validateAndIdentifyRbCreationSpec(files []spec.File, multipleSourcesAndPackageSupported bool) ([]services.SourceType, error) {
	if len(files) == 0 {
		return nil, errorutils.CheckErrorf("spec must include at least one file group")
	}

	detectedCreationSources, err := detectSourceTypesFromSpec(files, multipleSourcesAndPackageSupported)
	if err != nil {
		return nil, err
	}

	if err = validateCreationSources(detectedCreationSources, multipleSourcesAndPackageSupported); err != nil {
		return nil, err
	}
	return detectedCreationSources, nil
}

func validateCreationSources(detectedCreationSources []services.SourceType, multipleSourcesAndPackagesSupported bool) error {
	if len(detectedCreationSources) == 0 {
		return errorutils.CheckErrorf(missingCreationSourcesErrMsg)
	}

	// Assert single creation source in case multiple sources isn't supported
	var aqlSrcCnt int
	for i := 0; i < len(detectedCreationSources); i++ {
		if detectedCreationSources[i] == services.Aql {
			aqlSrcCnt++
		}

		if !multipleSourcesAndPackagesSupported {
			if detectedCreationSources[i] == services.Packages {
				return errorutils.CheckErrorf(unsupportedCreationSourceMethod)
			}
			if detectedCreationSources[i] != detectedCreationSources[0] {
				return generateSingleCreationSourceErr(detectedCreationSources)
			}
		}

	}

	// If aql, assert single file.
	if aqlSrcCnt > 1 {
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

func validateFile(file spec.File, packageAndMultiSourceSupported bool) (services.SourceType, error) {
	// Aql creation source:
	isAql := len(file.Aql.ItemsFind) > 0

	// Build creation source:
	isBuild := len(file.Build) > 0
	isIncludeDeps, _ := file.IsIncludeDeps(false)

	// Bundle creation source:
	isBundle := len(file.Bundle) > 0

	// Build & bundle
	isProject := len(file.Project) > 0

	// Packages
	isPackage := len(file.Package) > 0

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
	if !packageAndMultiSourceSupported {
		if coreutils.SumTrueValues([]bool{isAql, isBuild, isBundle, isPattern}) != 1 {
			return "", errorutils.CheckErrorf("exactly one creation source should be defined per file (aql, builds, release bundles or pattern (artifacts))")
		}
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
	case isPackage:
		return services.Packages,
			validateCreationSource([]bool{isIncludeDeps, isExclusions, isProps, isExcludeProps, !isRecursive, isProject},
				"release bundles creation source only supports the 'version', 'type', 'repoKey' fields")
	}

	return "", errorutils.CheckError(errors.New("unexpected err in spec validation"))
}

func validateCreationSource(unsupportedFields []bool, errMsg string) error {
	if coreutils.SumTrueValues(unsupportedFields) > 0 {
		return errorutils.CheckError(errors.New(errMsg))
	}
	return nil
}
