package commands

import (
	"errors"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"path"
)

func (rbc *ReleaseBundleCreateCommand) createFromArtifacts(lcServicesManager *lifecycle.LifecycleServicesManager,
	rbDetails services.ReleaseBundleDetails, queryParams services.CommonOptionalQueryParams) (err error) {

	err, artifactsSource := rbc.createArtifactSourceFromSpec()
	if err != nil {
		return err
	}

	return lcServicesManager.CreateReleaseBundleFromArtifacts(rbDetails, queryParams, rbc.signingKeyName, artifactsSource)
}
func (rbc *ReleaseBundleCreateCommand) getArtifactPatternsFromSpec() []string {
	var patterns []string
	for _, file := range rbc.spec.Files {
		if file.Pattern != "" {
			patterns = append(patterns, file.Pattern)
		}
	}
	return patterns
}

func (rbc *ReleaseBundleCreateCommand) getArtifactFilesFromSpec() []spec.File {
	var artifactFiles []spec.File
	for _, file := range rbc.spec.Files {
		if file.Pattern != "" {
			artifactFiles = append(artifactFiles, file)
		}
	}
	return artifactFiles
}

func (rbc *ReleaseBundleCreateCommand) createArtifactSourceFromSpec() (error, services.CreateFromArtifacts) {
	var artifactsSource services.CreateFromArtifacts

	rtServicesManager, err := utils.CreateServiceManager(rbc.serverDetails, 3, 0, false)
	if err != nil {
		return err, artifactsSource
	}

	searchResults, callbackFunc, err := utils.SearchFilesBySpecific(rtServicesManager, rbc.getArtifactFilesFromSpec())
	defer func() {
		err = errors.Join(err, callbackFunc())
	}()
	if err != nil {
		return err, artifactsSource
	}

	artifactsSource, err = aqlResultToArtifactsSource(searchResults)
	if err != nil {
		return err, artifactsSource
	}
	return nil, artifactsSource
}

func aqlResultToArtifactsSource(readers []*content.ContentReader) (artifactsSource services.CreateFromArtifacts, err error) {
	for _, reader := range readers {
		for searchResult := new(rtServicesUtils.ResultItem); reader.NextRecord(searchResult) == nil; searchResult = new(rtServicesUtils.ResultItem) {
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
