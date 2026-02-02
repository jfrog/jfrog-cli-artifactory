package commands

import (
	"errors"

	coreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type ReleaseBundleUpdateCommand struct {
	releaseBundleCmd
	spec *spec.SpecFiles
	// Multi-builds and multi-bundles sources from command-line flags
	ReleaseBundleSources
}

func NewReleaseBundleUpdateCommand() *ReleaseBundleUpdateCommand {
	return &ReleaseBundleUpdateCommand{}
}

func (rbu *ReleaseBundleUpdateCommand) SetServerDetails(serverDetails *config.ServerDetails) *ReleaseBundleUpdateCommand {
	rbu.serverDetails = serverDetails
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleName(releaseBundleName string) *ReleaseBundleUpdateCommand {
	rbu.releaseBundleName = releaseBundleName
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleVersion(releaseBundleVersion string) *ReleaseBundleUpdateCommand {
	rbu.releaseBundleVersion = releaseBundleVersion
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundleProject(rbProjectKey string) *ReleaseBundleUpdateCommand {
	rbu.rbProjectKey = rbProjectKey
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetSpec(spec *spec.SpecFiles) *ReleaseBundleUpdateCommand {
	rbu.spec = spec
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetBuildsSources(sourcesBuilds string) *ReleaseBundleUpdateCommand {
	rbu.sourcesBuilds = sourcesBuilds
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetReleaseBundlesSources(sourcesReleaseBundles string) *ReleaseBundleUpdateCommand {
	rbu.sourcesReleaseBundles = sourcesReleaseBundles
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) SetSync(sync bool) *ReleaseBundleUpdateCommand {
	rbu.sync = sync
	return rbu
}

func (rbu *ReleaseBundleUpdateCommand) CommandName() string {
	return "rb_update"
}

func (rbu *ReleaseBundleUpdateCommand) ServerDetails() (*config.ServerDetails, error) {
	return rbu.serverDetails, nil
}

func (rbu *ReleaseBundleUpdateCommand) Run() error {
	if err := validateArtifactoryVersionSupported(rbu.serverDetails); err != nil {
		return err
	}

	// Validate Artifactory version supports draft bundle operations (update only works on draft bundles)
	if err := ValidateFeatureSupportedVersion(rbu.serverDetails, minArtifactoryVersionForDraftBundleSupport); err != nil {
		return errorutils.CheckErrorf("release bundle update requires Artifactory version %s or higher", minArtifactoryVersionForDraftBundleSupport)
	}

	servicesManager, rbDetails, queryParams, err := rbu.getPrerequisites()
	if err != nil {
		return err
	}

	addSources, err := rbu.getAddSources()
	if err != nil {
		return err
	}

	if len(addSources) == 0 {
		return errorutils.CheckErrorf("at least one source must be provided to update a release bundle")
	}

	_, err = servicesManager.UpdateReleaseBundleFromMultipleSources(rbDetails, queryParams, "", addSources)
	return err
}

func (rbu *ReleaseBundleUpdateCommand) getAddSources() ([]services.RbSource, error) {
	// First check if sources are provided via command-line flags
	if rbu.sourcesBuilds != "" || rbu.sourcesReleaseBundles != "" {
		return rbu.buildSourcesFromCommandFlags(), nil
	}

	// Otherwise, parse from spec file
	if rbu.spec == nil || rbu.spec.Files == nil {
		return nil, errorutils.CheckError(errors.New("no spec file or source flags provided"))
	}

	return rbu.buildSourcesFromSpec()
}

func (rbu *ReleaseBundleUpdateCommand) buildSourcesFromCommandFlags() []services.RbSource {
	var sources []services.RbSource

	if rbu.sourcesBuilds != "" {
		sources = buildRbBuildsSources(rbu.sourcesBuilds, rbu.rbProjectKey, sources)
	}

	if rbu.sourcesReleaseBundles != "" {
		sources = buildRbReleaseBundlesSources(rbu.sourcesReleaseBundles, rbu.rbProjectKey, sources)
	}

	return sources
}

func (rbu *ReleaseBundleUpdateCommand) buildSourcesFromSpec() ([]services.RbSource, error) {
	detectedSources, err := detectSourceTypesFromSpec(rbu.spec.Files, true)
	if err != nil {
		return nil, err
	}

	if err = validateCreationSources(detectedSources, true); err != nil {
		return nil, err
	}

	return rbu.convertSpecToSources(detectedSources)
}

func (rbu *ReleaseBundleUpdateCommand) convertSpecToSources(detectedSources []services.SourceType) ([]services.RbSource, error) {
	var sources []services.RbSource
	handledSources := make(SourceTypeSet)

	for _, sourceType := range detectedSources {
		if handledSources[sourceType] {
			continue
		}
		handledSources[sourceType] = true

		switch sourceType {
		case services.ReleaseBundles:
			rbSources, err := convertSpecToReleaseBundlesSource(rbu.spec.Files)
			if err != nil {
				return nil, err
			}
			if len(rbSources.ReleaseBundles) > 0 {
				sources = append(sources, services.RbSource{
					SourceType:     services.ReleaseBundles,
					ReleaseBundles: rbSources.ReleaseBundles,
				})
			}
		case services.Builds:
			buildSources, err := convertSpecToBuildsSource(rbu.serverDetails, rbu.spec.Files)
			if err != nil {
				return nil, err
			}
			if len(buildSources.Builds) > 0 {
				sources = append(sources, services.RbSource{
					SourceType: services.Builds,
					Builds:     buildSources.Builds,
				})
			}
		case services.Artifacts:
			artifactSources, err := rbu.createArtifactSourceFromSpec()
			if err != nil {
				return nil, err
			}
			if len(artifactSources.Artifacts) > 0 {
				sources = append(sources, services.RbSource{
					SourceType: services.Artifacts,
					Artifacts:  artifactSources.Artifacts,
				})
			}
		default:
			return nil, errorutils.CheckError(errors.New("unexpected source type: " + string(sourceType)))
		}
	}

	return sources, nil
}

func (rbu *ReleaseBundleUpdateCommand) createArtifactSourceFromSpec() (services.CreateFromArtifacts, error) {
	var artifactsSource services.CreateFromArtifacts
	rtServicesManager, err := coreUtils.CreateServiceManager(rbu.serverDetails, 3, 0, false)
	if err != nil {
		return artifactsSource, err
	}

	searchResults, callbackFunc, err := coreUtils.SearchFilesBySpecs(rtServicesManager, getArtifactFilesFromSpec(rbu.spec.Files))
	if err != nil {
		return artifactsSource, err
	}

	defer func() {
		if callbackFunc != nil {
			err = errors.Join(err, callbackFunc())
		}
	}()

	artifactsSource, err = aqlResultToArtifactsSource(searchResults)
	if err != nil {
		return artifactsSource, err
	}
	return artifactsSource, nil
}
