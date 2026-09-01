package npm

import (
	"context"
	"fmt"
	"strings"

	gofrogcmd "github.com/jfrog/gofrog/io"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
)

func (nc *NpmCommand) applyZeroTouchRemediation() error {
	if !zerotouchremediation.IsComponentResolutionEnabled() {
		return nil
	}
	restore, remediated, err := nc.runZeroTouchRemediation(context.Background(), nc.cmdName, nc.workingDirectory, nc.npmArgs)
	if err != nil {
		return err
	}
	nc.restoreResolution = restore
	nc.remediatedLockfile = remediated
	return nil
}

func (nc *NpmCommand) runZeroTouchRemediation(ctx context.Context, command, workingDir string, npmArgs []string) (restore func() error, remediated bool, err error) {
	if command == "install" && isSinglePackageInstall(npmArgs) {
		return func() error { return nil }, false, nil
	}
	return nc.runZeroTouchRemediationWithTool(ctx, command, workingDir, NewBuildToolWithArgs(npmArgs), BootstrapArgsFrom(npmArgs)...)
}

func (nc *NpmCommand) runZeroTouchRemediationWithTool(ctx context.Context, command, workingDir string, tool zerotouchremediation.BuildTool, bootstrapArgs ...string) (restore func() error, remediated bool, err error) {
	resolverRepo, resolverErr := nc.resolverRepoForResolution()
	if resolverErr != nil {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: could not determine resolver repo: ", resolverErr)
	}
	if resolverRepo == "" {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: resolver repo is empty", nil)
	}
	var projectKey string
	if nc.buildConfiguration != nil {
		projectKey = nc.buildConfiguration.GetProject()
	}
	xrayManager, xrayErr := xray.CreateXrayServiceManager(nc.serverDetails, xray.WithScopedProjectKey(projectKey))
	if xrayErr != nil {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: could not create Xray service manager: ", xrayErr)
	}
	return zerotouchremediation.RunIfEnabled(ctx, xrayManager, resolverRepo, tool, command, workingDir, nc.npmBootstrapRunner(), bootstrapArgs...)
}

// resolverRepoForResolution returns the Artifactory virtual repo for dependency policy scope.
func (nc *NpmCommand) resolverRepoForResolution() (string, error) {
	if nc.repo != "" {
		return nc.repo, nil
	}
	if nc.configFilePath != "" {
		vConfig, err := project.ReadConfigFile(nc.configFilePath, project.YAML)
		if err != nil {
			return "", fmt.Errorf("failed to read config file: %w", err)
		}
		resolverConfig, err := project.GetRepoConfigByPrefix(nc.configFilePath, project.ProjectConfigResolverPrefix, vConfig)
		if err != nil {
			return "", fmt.Errorf("failed to get resolver config: %w", err)
		}
		return resolverConfig.TargetRepo(), nil
	}
	if nc.executablePath != "" {
		registryURL, err := nc.getNpmRegistryURL()
		if err != nil {
			return "", fmt.Errorf("failed to get registry URL: %w", err)
		}
		return extractRepoName(registryURL)
	}
	return nc.repo, nil
}

func (nc *NpmCommand) getNpmRegistryURL() (string, error) {
	if parsed := parseNpmCLIArgs(nc.npmArgs); parsed.registryURL != "" {
		return parsed.registryURL, nil
	}
	configCommand := gofrogcmd.Command{
		Executable: nc.executablePath,
		CmdName:    "config",
		CmdArgs:    []string{"get", "registry"},
	}
	data, err := configCommand.RunWithOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (nc *NpmCommand) npmBootstrapRunner() zerotouchremediation.CommandRunner {
	return func(ctx context.Context, projectRoot string, args ...string) error {
		return runNpmAt(ctx, nc.executablePath, projectRoot, args...)
	}
}

func runNpmAt(_ context.Context, executablePath, projectRoot string, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	cmd := gofrogcmd.NewCommand(executablePath, args[0], args[1:])
	cmd.Dir = projectRoot
	_, err := cmd.RunWithOutput()
	return err
}

func isSinglePackageInstall(npmArgs []string) bool {
	return HasPackageOperands(npmArgs)
}
