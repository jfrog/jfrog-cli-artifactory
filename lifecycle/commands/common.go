package commands

import (
	"fmt"
	"path"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	rtServices "github.com/jfrog/jfrog-client-go/artifactory/services"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	clientUtils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/distribution"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
)

const (
	rbV2manifestName                                      = "release-bundle.json.evd"
	releaseBundlesV2                                      = "release-bundles-v2"
	minimalLifecycleArtifactoryVersion                    = "7.63.2"
	minArtifactoryVersionForMultiSourceAndPackagesSupport = "7.114.0"
)

type releaseBundleCmd struct {
	serverDetails        *config.ServerDetails
	releaseBundleName    string
	releaseBundleVersion string
	sync                 bool
	rbProjectKey         string
}

func (rbc *releaseBundleCmd) getPrerequisites() (servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams, err error) {
	return rbc.initPrerequisites()
}

func (rbp *ReleaseBundlePromoteCommand) getPromotionPrerequisites() (servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams, err error) {
	servicesManager, rbDetails, queryParams, err = rbp.initPrerequisites()
	queryParams.PromotionType = rbp.promotionType
	return servicesManager, rbDetails, queryParams, err
}

func (rbc *releaseBundleCmd) initPrerequisites() (servicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams, err error) {
	servicesManager, err = utils.CreateLifecycleServiceManager(rbc.serverDetails, false)
	if err != nil {
		return
	}
	rbDetails = services.ReleaseBundleDetails{
		ReleaseBundleName:    rbc.releaseBundleName,
		ReleaseBundleVersion: rbc.releaseBundleVersion,
	}
	queryParams = services.CommonOptionalQueryParams{
		ProjectKey: rbc.rbProjectKey,
		Async:      !rbc.sync,
	}

	return
}

func validateArtifactoryVersion(serverDetails *config.ServerDetails, minVersion string) error {
	rtServiceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return err
	}

	versionStr, err := rtServiceManager.GetVersion()
	if err != nil {
		return err
	}

	return clientUtils.ValidateMinimumVersion(clientUtils.Artifactory, versionStr, minVersion)
}

func validateArtifactoryVersionSupported(serverDetails *config.ServerDetails) error {
	return validateArtifactoryVersion(serverDetails, minimalLifecycleArtifactoryVersion)
}

func ValidateFeatureSupportedVersion(serverDetails *config.ServerDetails, minCommandVersion string) error {
	return validateArtifactoryVersion(serverDetails, minCommandVersion)
}

// If distribution rules are empty, distribute to all edges.
func getAggregatedDistRules(distributionRules *spec.DistributionRules) (aggregatedRules []*distribution.DistributionCommonParams) {
	if isDistributionRulesEmpty(distributionRules) {
		aggregatedRules = append(aggregatedRules, &distribution.DistributionCommonParams{SiteName: "*"})
	} else {
		for _, rules := range distributionRules.DistributionRules {
			aggregatedRules = append(aggregatedRules, rules.ToDistributionCommonParams())
		}
	}
	return
}

func isDistributionRulesEmpty(distributionRules *spec.DistributionRules) bool {
	return distributionRules == nil ||
		len(distributionRules.DistributionRules) == 0 ||
		len(distributionRules.DistributionRules) == 1 && distributionRules.DistributionRules[0].IsEmpty()
}

func buildRepoKey(project string) string {
	if project == "" || project == "default" {
		return releaseBundlesV2
	}
	return fmt.Sprintf("%s-%s", project, releaseBundlesV2)
}

func buildManifestPath(projectKey, name, version string) string {
	return fmt.Sprintf("%s/%s/%s/%s", buildRepoKey(projectKey), name, version, rbV2manifestName)
}

// getAqlService creates an AQL service for querying Artifactory
func getAqlService(serverDetails *config.ServerDetails) (*rtServices.AqlService, error) {
	rtServiceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return rtServices.NewAqlService(rtServiceManager.GetConfig().GetServiceDetails(), rtServiceManager.Client()), nil
}

// getBuildDetailsFromIdentifier resolves build name and number from a build identifier string
func getBuildDetailsFromIdentifier(serverDetails *config.ServerDetails, buildIdentifier, project string) (string, string, error) {
	aqlService, err := getAqlService(serverDetails)
	if err != nil {
		return "", "", err
	}

	buildName, buildNumber, err := rtServicesUtils.GetBuildNameAndNumberFromBuildIdentifier(buildIdentifier, project, aqlService)
	if err != nil {
		return "", "", err
	}
	if buildName == "" || buildNumber == "" {
		return "", "", errorutils.CheckErrorf("could not identify a build info by the '%s' identifier in artifactory", buildIdentifier)
	}
	return buildName, buildNumber, nil
}

// getArtifactFilesFromSpec filters spec files to return only those with a Pattern (artifacts)
func getArtifactFilesFromSpec(files []spec.File) []spec.File {
	artifactFiles := make([]spec.File, 0, len(files))
	for _, file := range files {
		if file.Pattern != "" {
			artifactFiles = append(artifactFiles, file)
		}
	}
	return artifactFiles
}

// aqlResultToArtifactsSource converts AQL search results to CreateFromArtifacts source
func aqlResultToArtifactsSource(readers []*content.ContentReader) (artifactsSource services.CreateFromArtifacts, err error) {
	// Allocate buffer once outside the loops to avoid unnecessary heap allocations on every iteration
	searchResult := new(rtServicesUtils.ResultItem)
	for _, reader := range readers {
		for reader.NextRecord(searchResult) == nil {
			artifactsSource.Artifacts = append(artifactsSource.Artifacts, services.ArtifactSource{
				Path:   path.Join(searchResult.Repo, searchResult.Path, searchResult.Name),
				Sha256: searchResult.Sha256,
			})
		}
		if err = reader.GetError(); err != nil {
			return
		}
		reader.Reset()
	}
	return
}

// convertSpecToReleaseBundlesSource converts spec files with Bundle field to ReleaseBundlesSource
func convertSpecToReleaseBundlesSource(files []spec.File) (services.CreateFromReleaseBundlesSource, error) {
	var releaseBundlesSource services.CreateFromReleaseBundlesSource
	for _, file := range files {
		if file.Bundle == "" {
			continue
		}
		bundleName, bundleVersion, err := rtServicesUtils.ParseNameAndVersion(file.Bundle, false)
		if err != nil {
			return releaseBundlesSource, err
		}
		if bundleName == "" || bundleVersion == "" {
			return releaseBundlesSource, errorutils.CheckErrorf(
				"invalid release bundle source was provided. Both name and version are mandatory. Provided name: '%s', version: '%s'", bundleName, bundleVersion)
		}
		releaseBundlesSource.ReleaseBundles = append(releaseBundlesSource.ReleaseBundles, services.ReleaseBundleSource{
			ReleaseBundleName:    bundleName,
			ReleaseBundleVersion: bundleVersion,
			ProjectKey:           file.Project,
		})
	}
	return releaseBundlesSource, nil
}

// convertSpecToBuildsSource converts spec files with Build field to CreateFromBuildsSource
func convertSpecToBuildsSource(serverDetails *config.ServerDetails, files []spec.File) (services.CreateFromBuildsSource, error) {
	var buildsSource services.CreateFromBuildsSource
	for _, file := range files {
		if file.Build == "" {
			continue
		}
		buildName, buildNumber, err := getBuildDetailsFromIdentifier(serverDetails, file.Build, file.Project)
		if err != nil {
			return services.CreateFromBuildsSource{}, err
		}
		isIncludeDeps, err := file.IsIncludeDeps(false)
		if err != nil {
			return services.CreateFromBuildsSource{}, err
		}
		buildSource := services.BuildSource{
			BuildName:           buildName,
			BuildNumber:         buildNumber,
			BuildRepository:     rtServicesUtils.GetBuildInfoRepositoryByProject(file.Project),
			IncludeDependencies: isIncludeDeps,
		}
		buildsSource.Builds = append(buildsSource.Builds, buildSource)
	}
	return buildsSource, nil
}
