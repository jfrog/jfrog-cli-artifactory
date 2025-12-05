package ocicontainer

import (
	"fmt"
	"strings"

	buildinfo "github.com/jfrog/build-info-go/entities"
	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// DockerBuildInfoBuilder is a simplified builder for docker build command
type DockerBuildInfoBuilder struct {
	buildName                       string
	buildNumber                     string
	project                         string
	module                          string
	serviceManager                  artifactory.ArtifactoryServicesManager
	imageTag                        string
	baseImages                      []BaseImage
	isImagePushed                   bool
	cmdArgs                         []string
	repositoryDetails               dockerRepositoryDetails
	searchableLayerForApplyingProps []utils.ResultItem
}

type dockerRepositoryDetails struct {
	Key                   string `json:"key"`
	RepoType              string `json:"rclass"`
	DefaultDeploymentRepo string `json:"defaultDeploymentRepo"`
}

type BaseImage struct {
	Image        string
	OS           string
	Architecture string
}

type manifestType string

const (
	ManifestList manifestType = "list.manifest.json"
	Manifest     manifestType = "manifest.json"
)

// NewDockerBuildInfoBuilder creates a new builder for docker build command
func NewDockerBuildInfoBuilder(buildName, buildNumber, project string, module string, serviceManager artifactory.ArtifactoryServicesManager,
	imageTag string, baseImages []BaseImage, isImagePushed bool, cmdArgs []string) *DockerBuildInfoBuilder {

	biImage := NewImage(imageTag)

	var err error
	if module == "" {
		module, err = biImage.GetImageShortNameWithTag()
		if err != nil {
			log.Warn("Failed to extract module name from image tag '%s': %s. Using entire image tag as module name.", imageTag, err.Error())
			module = imageTag
		}
	}

	return &DockerBuildInfoBuilder{
		buildName:      buildName,
		buildNumber:    buildNumber,
		project:        project,
		module:         module,
		serviceManager: serviceManager,
		imageTag:       imageTag,
		baseImages:     baseImages,
		isImagePushed:  isImagePushed,
		cmdArgs:        cmdArgs,
	}
}

// Build orchestrates the collection of dependencies and artifacts for the docker build
func (dbib *DockerBuildInfoBuilder) Build() error {
	if err := build.SaveBuildGeneralDetails(dbib.buildName, dbib.buildNumber, dbib.project); err != nil {
		return err
	}

	dependencies, err := dbib.getDependencies()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get dependencies for '%s'. Error: %v", dbib.buildName, err))
	}

	artifacts, leadSha, err := dbib.getArtifacts()
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to get artifacts for '%s'. Error: %v", dbib.buildName, err))
	}

	err = dbib.applyBuildProps(dbib.searchableLayerForApplyingProps)
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to apply build prop. Error: %v", err))
	}

	biProperties := dbib.getBiProperties(leadSha)

	buildInfo := &buildinfo.BuildInfo{Modules: []buildinfo.Module{{
		Id:           dbib.module,
		Type:         buildinfo.Docker,
		Properties:   biProperties,
		Dependencies: dependencies,
		Artifacts:    artifacts,
	}}}

	if err = build.SaveBuildInfo(dbib.buildName, dbib.buildNumber, dbib.project, buildInfo); err != nil {
		return errorutils.CheckErrorf("failed to save build info for '%s/%s': %s", dbib.buildName, dbib.buildNumber, err.Error())
	}

	return nil
}

// applyBuildProps applies build properties to the artifacts
func (dbib *DockerBuildInfoBuilder) applyBuildProps(items []utils.ResultItem) (err error) {
	props, err := build.CreateBuildProperties(dbib.buildName, dbib.buildNumber, dbib.project)
	if err != nil {
		return
	}
	pushedRepo := dbib.getPushedRepo()
	filteredLayers := filterLayersFromVirtualRepo(items, pushedRepo)
	if len(filteredLayers) == 0 {
		log.Debug(fmt.Sprintf("Filtered layers length is 0 after filtering with pushedRepo: %s, All layers: %v", pushedRepo, items))
		log.Warn(fmt.Sprintf("No eligible layers found to apply build properties for pushedRepo: %s. "+
			"Skipping...", pushedRepo))
		return nil
	}
	pathToFile, err := writeLayersToFile(filteredLayers)
	if err != nil {
		return
	}
	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer ioutils.Close(reader, &err)
	_, err = dbib.serviceManager.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true, IsRecursive: true})
	return err
}

// getBiProperties returns build info properties for the docker build
func (dbib *DockerBuildInfoBuilder) getBiProperties(leadSha string) map[string]string {
	properties := map[string]string{
		"docker.image.tag": dbib.imageTag,
	}
	if dbib.isImagePushed {
		properties["docker.image.id"] = leadSha
	}
	if dbib.cmdArgs != nil {
		properties["docker.build.command"] = strings.Join(dbib.cmdArgs, " ")
	}
	return properties
}
