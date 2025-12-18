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

	// Get project key from build configuration (same as non-FlexPack flow)
	projectKey := ""
	if buildConfiguration != nil {
		projectKey = buildConfiguration.GetProject()
	}

	if err := saveGradleFlexPackBuildInfo(buildInfo, projectKey); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	if isPublishCommand {
		if err := setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, projectKey, buildInfo, serverDetails); err != nil {
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

func saveGradleFlexPackBuildInfo(buildInfo *entities.BuildInfo, projectKey string) error {
	service := build.NewBuildInfoService()
	// Pass the project key to organize build info under the correct project (same as non-FlexPack flow)
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return buildInstance.SaveBuildInfo(buildInfo)
}

func setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, projectKey string, buildInfo *entities.BuildInfo, serverDetails *config.ServerDetails) error {
	if serverDetails == nil {
		log.Warn("No server details configured, skipping build properties")
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	artifacts := collectArtifactsFromBuildInfo(buildInfo, workingDir)
	if len(artifacts) == 0 {
		log.Warn("No artifacts found to set build properties on")
		return nil
	}
	// This creates: build.name=<name>;build.number=<number>;build.timestamp=<timestamp>
	buildProps, err := buildUtils.CreateBuildProperties(buildName, buildNumber, projectKey)
	if err != nil {
		// Fallback to manual creation if CreateBuildProperties fails
		log.Debug(fmt.Sprintf("CreateBuildProperties failed, using fallback: %s", err.Error()))
		timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
		buildProps = fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	}

	// Add project key to properties if specified (same as non-FlexPack flow sets deploy.build.project)
	if projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}

	writer, err := content.NewContentWriter(content.DefaultKey, true, false)
	if err != nil {
		return fmt.Errorf("failed to create content writer: %w", err)
	}

	// Write all artifacts to the writer
	for _, art := range artifacts {
		writer.Write(art)
	}

	// Close flushes all writes and surfaces any accumulated errors
	if closeErr := writer.Close(); closeErr != nil {
		// Clean up temp file on error
		if writerFilePath := writer.GetFilePath(); writerFilePath != "" {
			if removeErr := os.Remove(writerFilePath); removeErr != nil {
				log.Debug(fmt.Sprintf("Failed to remove temp file after write error: %s", removeErr))
			}
		}
		return fmt.Errorf("failed to close content writer: %w", closeErr)
	}

	writerFilePath := writer.GetFilePath()

	// Ensure temp file cleanup after we're done (success or failure)
	defer func() {
		if writerFilePath != "" {
			if removeErr := os.Remove(writerFilePath); removeErr != nil {
				log.Debug(fmt.Sprintf("Failed to remove temp file: %s", removeErr))
			}
		}
	}()

	reader := content.NewContentReader(writerFilePath, content.DefaultKey)
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
	// Always return empty slice instead of nil for consistent behavior
	if buildInfo == nil {
		return []servicesutils.ResultItem{}
	}
	result := make([]servicesutils.ResultItem, 0)

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
