package container

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container/strategies"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type BuildCommand struct {
	ContainerCommandBase
	cmdParams []string
	strategy  strategies.BuildStrategy
}

func NewBuildCommand() *BuildCommand {
	return &BuildCommand{}
}

func (bc *BuildCommand) SetCmdParams(cmdParams []string) *BuildCommand {
	bc.cmdParams = cmdParams
	// Create strategy based on command parameters
	factory := &strategies.BuildStrategyFactory{}
	bc.strategy = factory.CreateStrategy(cmdParams)
	return bc
}

func (bc *BuildCommand) SetBuildConfiguration(buildConfig *build.BuildConfiguration) *BuildCommand {
	bc.buildConfiguration = buildConfig
	return bc
}

func (bc *BuildCommand) Run() error {
	// Validate configuration if needed
	if err := bc.validateConfig(); err != nil {
		return err
	}

	// Execute using the selected strategy
	return bc.strategy.Execute(bc.cmdParams, bc.buildConfiguration)
}

func (bc *BuildCommand) validateConfig() error {
	// Validate build parameters if build-info collection is requested
	if bc.buildConfiguration != nil {
		return bc.buildConfiguration.ValidateBuildParams()
	}
	return nil
}

func (bc *BuildCommand) CommandName() string {
	return "docker_build"
}

func (bc *BuildCommand) ServerDetails() (*config.ServerDetails, error) {
	return bc.serverDetails, nil
}
