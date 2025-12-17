package flexpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	gradle "github.com/jfrog/build-info-go/flexpack/gradle"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func CollectGradleBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, tasks []string, buildConfiguration *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails) error {
	if workingDir == "" {
		return fmt.Errorf("working directory is required")
	}
	if buildName == "" || buildNumber == "" {
		return fmt.Errorf("build name and build number are required")
	}

	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for working directory: %w", err)
	}
	workingDir = absWorkingDir
	config := flexpack.GradleConfig{
		WorkingDirectory:        workingDir,
		IncludeTestDependencies: true,
	}

	gradleFlex, err := gradle.NewGradleFlexPack(config)
	if err != nil {
		return fmt.Errorf("failed to create Gradle FlexPack: %w", err)
	}

	isPublishCommand := wasPublishCommand(tasks)
	gradleFlex.SetWasPublishCommand(isPublishCommand)

	buildInfo, err := gradleFlex.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
	}

	if err := saveGradleFlexPackBuildInfo(buildInfo); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	if isPublishCommand {
		if err := setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, buildConfiguration, buildInfo, serverDetails); err != nil {
			log.Warn("Failed to set build properties on deployed artifacts: " + err.Error())
		}
	}
	return nil
}

func wasPublishCommand(tasks []string) bool {
	for _, task := range tasks {
		// Handle tasks with project paths (e.g., ":subproject:publish")
		if idx := strings.LastIndex(task, ":"); idx != -1 {
			task = task[idx+1:]
		}
		if task == gradleTaskPublish {
			return true
		}

		if strings.HasPrefix(task, gradleTaskPublish) {
			toIdx := strings.Index(task, "To")
			if toIdx != -1 {
				afterTo := task[toIdx+2:]
				// Exclude local publishing tasks like "publishToMavenLocal" or "publishAllPublicationsToLocal"
				if len(afterTo) > 0 && !strings.HasSuffix(task, "Local") {
					return true
				}
			}
		}
	}
	return false
}

func saveGradleFlexPackBuildInfo(buildInfo *entities.BuildInfo) error {
	service := build.NewBuildInfoService()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return buildInstance.SaveBuildInfo(buildInfo)
}

func setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber string, buildArgs *buildUtils.BuildConfiguration, buildInfo *entities.BuildInfo, serverDetails *config.ServerDetails) error {
	if serverDetails == nil {
		log.Warn("No server details configured, skipping build properties")
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	projectKey := ""
	if buildArgs != nil {
		projectKey = buildArgs.GetProject()
	}
	artifacts := collectArtifactsFromBuildInfo(buildInfo, workingDir)
	if len(artifacts) == 0 {
		log.Warn("No artifacts found to set build properties on")
		return nil
	}

	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}

	writer, err := content.NewContentWriter(content.DefaultKey, true, false)
	if err != nil {
		return fmt.Errorf("failed to create content writer: %w", err)
	}
	for _, art := range artifacts {
		writer.Write(art)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close content writer: %w", err)
	}

	reader := content.NewContentReader(writer.GetFilePath(), content.DefaultKey)
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close reader: %s", closeErr))
		}
	}()

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  buildProps,
	}

	if _, err = servicesManager.SetProps(propsParams); err != nil {
		return fmt.Errorf("failed to set properties on artifacts: %w", err)
	}

	log.Info("Successfully set build properties on deployed Gradle artifacts")
	return nil
}

func collectArtifactsFromBuildInfo(buildInfo *entities.BuildInfo, workingDir string) []servicesutils.ResultItem {
	if buildInfo == nil {
		return nil
	}
	var result []servicesutils.ResultItem

	// Cache per (module dir, version) to avoid repeated lookups and wrong reuse.
	moduleRepoCache := make(map[string]string)

	resolveRepoForModule := func(module entities.Module) string {
		// Try to infer version from module ID: group:artifact:version
		version := ""
		if parts := strings.Split(module.Id, ":"); len(parts) >= 3 {
			version = parts[len(parts)-1]
		}

		modulePath := ""
		if props, ok := module.Properties.(map[string]string); ok {
			modulePath = props["module_path"]
		}
		moduleWorkingDir := workingDir
		if modulePath != "" {
			moduleWorkingDir = filepath.Join(workingDir, modulePath)
		}

		cacheKey := fmt.Sprintf("%s|%s", moduleWorkingDir, version)
		if cached, ok := moduleRepoCache[cacheKey]; ok {
			return cached
		}

		repo, err := getGradleDeployRepository(moduleWorkingDir, workingDir, version)
		if err != nil && moduleWorkingDir != workingDir {
			log.Debug(fmt.Sprintf("Repo not found in module dir %s, trying root: %v", moduleWorkingDir, err))
			repo, err = getGradleDeployRepository(workingDir, workingDir, version)
		}
		if err != nil {
			log.Warn("Failed to resolve Gradle deploy repository for module " + module.Id + ": " + err.Error())
		}
		if repo == "" {
			log.Warn("Gradle deploy repository not found for module " + module.Id + ", skipping artifacts without repo")
		}
		moduleRepoCache[cacheKey] = repo
		return repo
	}

	for _, module := range buildInfo.Modules {
		// Resolve repo once per module (OriginalDeploymentRepo is not populated for Gradle FlexPack).
		repo := resolveRepoForModule(module)
		if repo == "" {
			continue
		}

		for _, art := range module.Artifacts {
			if art.Name == "" {
				continue
			}

			itemPath := art.Path
			if strings.HasSuffix(itemPath, "/"+art.Name) {
				itemPath = strings.TrimSuffix(itemPath, "/"+art.Name)
			}
			result = append(result, servicesutils.ResultItem{
				Repo: repo,
				Path: itemPath,
				Name: art.Name,
			})
		}
	}
	return result
}

// ValidateWorkingDirectory checks if the working directory is valid.
func ValidateWorkingDirectory(workingDir string) error {
	if workingDir == "" {
		return fmt.Errorf("working directory cannot be empty")
	}
	info, err := os.Stat(workingDir)
	if err != nil {
		return fmt.Errorf("invalid working directory: %s - %w", workingDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory: %s", workingDir)
	}
	return nil
}
