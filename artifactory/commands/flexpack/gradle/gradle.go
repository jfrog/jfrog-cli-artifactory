package flexpack

import (
	"fmt"
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
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func CollectGradleBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, tasks []string, buildConfiguration *buildUtils.BuildConfiguration) error {
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for working directory: %w", err)
	}
	workingDir = absWorkingDir

	startTime := time.Now()
	config := flexpack.GradleConfig{
		WorkingDirectory:        workingDir,
		IncludeTestDependencies: true,
	}

	gradleFlex, err := gradle.NewGradleFlexPack(config)
	if err != nil {
		return fmt.Errorf("failed to create Gradle FlexPack: %w", err)
	}

	isPublishCommand := wasPublishCommand(tasks)
	gradleFlex.WasPublishCommand = isPublishCommand

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
		if err := setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, buildConfiguration, buildInfo, startTime); err != nil {
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

		// Match common Gradle publish tasks
		if task == gradleTaskPublish {
			return true
		}

		if strings.HasPrefix(task, gradleTaskPublish) {
			// Find the position of "To" in the task name (e.g., "publishToArtifactory")
			toIdx := strings.Index(task, "To")
			if toIdx != -1 {
				// Ensure there's actual content after "To" (e.g., "publishTo" alone is not valid)
				afterTo := task[toIdx+2:]
				if len(afterTo) > 0 && !strings.HasSuffix(task, "Local") && task != gradleTaskPublishToMavenLocal {
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

func setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber string, buildArgs *buildUtils.BuildConfiguration, buildInfo *entities.BuildInfo, startTime time.Time) error {
	serverDetails, err := getGradleServerDetails()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}
	if serverDetails == nil {
		log.Warn("No server details configured, skipping build properties")
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	projectKey := buildArgs.GetProject()
	recentArtifacts, err := searchRecentArtifacts(servicesManager, buildInfo, startTime, workingDir)
	if err != nil {
		return err
	}

	if len(recentArtifacts) == 0 {
		log.Warn("No recently deployed artifacts found")
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

	for _, artifact := range recentArtifacts {
		writer.Write(artifact)
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

	_, err = servicesManager.SetProps(propsParams)
	if err != nil {
		return fmt.Errorf("failed to set properties on artifacts: %w", err)
	}

	log.Info("Successfully set build properties on deployed Gradle artifacts")
	return nil
}

func getGradleServerDetails() (*config.ServerDetails, error) {
	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("failed to get server details: %w", err)
	}
	return serverDetails, nil
}
