package container

import (
	"path"

	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	containerutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/container"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	servicesutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type PushCommand struct {
	ContainerCommand
	threads         int
	detailedSummary bool
	validateSha     bool
	result          *commandsutils.Result
}

func NewPushCommand(containerManagerType containerutils.ContainerManagerType) *PushCommand {
	return &PushCommand{
		ContainerCommand: ContainerCommand{
			containerManagerType: containerManagerType,
		},
	}
}

func (pc *PushCommand) Threads() int {
	return pc.threads
}

func (pc *PushCommand) SetThreads(threads int) *PushCommand {
	pc.threads = threads
	return pc
}

func (pc *PushCommand) SetDetailedSummary(detailedSummary bool) *PushCommand {
	pc.detailedSummary = detailedSummary
	return pc
}

func (pc *PushCommand) IsDetailedSummary() bool {
	return pc.detailedSummary
}

func (pc *PushCommand) SetValidateSha(validateSha bool) *PushCommand {
	pc.ContainerCommandBase.SetValidateSha(validateSha)
	return pc
}

func (pc *PushCommand) IsValidateSha() bool {
	return pc.ContainerCommandBase.IsValidateSha()
}

func (pc *PushCommand) Result() *commandsutils.Result {
	return pc.result
}

func (pc *PushCommand) SetResult(result *commandsutils.Result) *PushCommand {
	pc.result = result
	return pc
}

func (pc *PushCommand) Run() error {
	if err := pc.init(); err != nil {
		return err
	}
	if pc.containerManagerType == containerutils.DockerClient {
		err := containerutils.ValidateClientApiVersion()
		if err != nil {
			return err
		}
	}
	serverDetails, err := pc.ServerDetails()
	if errorutils.CheckError(err) != nil {
		return err
	}
	// Perform login
	if err := pc.PerformLogin(serverDetails, pc.containerManagerType); err != nil {
		return err
	}
	// Perform push.
	cm := containerutils.NewManager(pc.containerManagerType)
	err = cm.RunNativeCmd(pc.cmdParams)
	if err != nil {
		return err
	}

	toCollect, err := pc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if !toCollect && !pc.IsDetailedSummary() {
		return nil
	}
	buildName, err := pc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := pc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}
	serviceManager, err := utils.CreateServiceManagerWithThreads(serverDetails, false, pc.threads, -1, 0)
	if err != nil {
		return err
	}
	repo, err := pc.GetRepo()
	if err != nil {
		return err
	}

	// If SHA validation is enabled, log it
	if pc.IsValidateSha() {
		log.Info("Performing SHA-based validation for Docker push...")
		// Get image SHA from the container manager
		imageSha2, err := cm.Id(pc.image)
		if err != nil {
			return err
		}
		log.Debug("Using image SHA256 for validation: " + imageSha2)

		// Use RemoteAgentBuildInfoBuilder for SHA-based validation
		remoteBuilder, err := containerutils.NewRemoteAgentBuildInfoBuilder(pc.image, repo, buildName, buildNumber, pc.BuildConfiguration().GetProject(), serviceManager, imageSha2)
		if err != nil {
			return err
		}

		if toCollect {
			if err := build.SaveBuildGeneralDetails(buildName, buildNumber, pc.buildConfiguration.GetProject()); err != nil {
				return err
			}
			buildInfoModule, err := remoteBuilder.Build(pc.BuildConfiguration().GetModule())
			if err != nil || buildInfoModule == nil {
				return err
			}
			if err = build.SaveBuildInfo(buildName, buildNumber, pc.BuildConfiguration().GetProject(), buildInfoModule); err != nil {
				return err
			}
		}

		if pc.IsDetailedSummary() {
			if !toCollect {
				// No build info collection triggered yet, build it now for the summary
				_, err = remoteBuilder.Build("")
				if err != nil {
					return err
				}
			}
			return pc.layersMapToFileTransferDetails(serverDetails.ArtifactoryUrl, remoteBuilder.GetLayers())
		}
		return nil
	}

	// Standard path: use LocalAgentBuildInfoBuilder for tag-based validation
	builder, err := containerutils.NewLocalAgentBuildInfoBuilder(pc.image, repo, buildName, buildNumber, pc.BuildConfiguration().GetProject(), serviceManager, containerutils.Push, cm)
	if err != nil {
		return err
	}

	if toCollect {
		if err := build.SaveBuildGeneralDetails(buildName, buildNumber, pc.buildConfiguration.GetProject()); err != nil {
			return err
		}
		buildInfoModule, err := builder.Build(pc.BuildConfiguration().GetModule())
		if err != nil || buildInfoModule == nil {
			return err
		}
		if err = build.SaveBuildInfo(buildName, buildNumber, pc.BuildConfiguration().GetProject(), buildInfoModule); err != nil {
			return err
		}
	}

	if pc.IsDetailedSummary() {
		if !toCollect {
			// The build-info collection hasn't been triggered at this point, and we do need it for handling the detailed summary.
			// We are therefore skipping setting image build name/number props before running build-info collection.
			builder.SetSkipTaggingLayers(true)
			_, err = builder.Build("")
			if err != nil {
				return err
			}
		}
		return pc.layersMapToFileTransferDetails(serverDetails.ArtifactoryUrl, builder.GetLayers())
	}

	return nil
}

func (pc *PushCommand) layersMapToFileTransferDetails(artifactoryUrl string, layers *[]servicesutils.ResultItem) error {
	var details []clientutils.FileTransferDetails
	for _, layer := range *layers {
		sha256 := ""
		for _, property := range layer.Properties {
			if property.Key == "sha256" {
				sha256 = property.Value
			}
		}
		details = append(details, clientutils.FileTransferDetails{TargetPath: path.Join(layer.Repo, layer.Path, layer.Name), RtUrl: artifactoryUrl, Sha256: sha256})
	}
	tempFile, err := clientutils.SaveFileTransferDetailsInTempFile(&details)
	if err != nil {
		return err
	}
	result := new(commandsutils.Result)
	result.SetReader(content.NewContentReader(tempFile, "files"))
	result.SetSuccessCount(len(details))
	pc.SetResult(result)
	return nil
}

func (pc *PushCommand) CommandName() string {
	return "rt_docker_push"
}

func (pc *PushCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

// Backward compatibility: If --validate-sha is not set, the legacy tag-based validation path is used, ensuring no change for existing users.
