package npm

import (
	"context"
	"fmt"
	"strings"

	gofrogcmd "github.com/jfrog/gofrog/io"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
	cnpm "github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation/npm"
)

func (ca *CommonArgs) runZeroTouchRemediation(ctx context.Context, command, workingDir string, npmArgs []string) (restore func() error, remediated bool, err error) {
	if command == "install" && isSinglePackageInstall(npmArgs) {
		return func() error { return nil }, false, nil
	}
	return ca.runZeroTouchRemediationWithTool(ctx, command, workingDir, cnpm.NewBuildToolWithArgs(npmArgs), cnpm.BootstrapArgsFrom(npmArgs)...)
}

func (ca *CommonArgs) runZeroTouchRemediationForPublish(ctx context.Context, command, workingDir, publishPath string, npmArgs []string) (restore func() error, remediated bool, err error) {
	return ca.runZeroTouchRemediationWithTool(ctx, command, workingDir, cnpm.NewBuildToolForPublish(workingDir, publishPath, npmArgs), cnpm.BootstrapArgsFrom(npmArgs)...)
}

func (ca *CommonArgs) runZeroTouchRemediationWithTool(ctx context.Context, command, workingDir string, tool cnpm.BuildTool, bootstrapArgs ...string) (restore func() error, remediated bool, err error) {
	if !zerotouchremediation.IsComponentResolutionEnabled() {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation is not enabled", nil)
	}
	resolverRepo, resolverErr := ca.resolverRepoForResolution(command)
	if resolverErr != nil {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: could not determine resolver repo: ", resolverErr)
	}
	if resolverRepo == "" {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: resolver repo is empty", nil)
	}
	var projectKey string
	if ca.buildConfiguration != nil {
		projectKey = ca.buildConfiguration.GetProject()
	}
	xrayManager, xrayErr := xray.CreateXrayServiceManager(ca.serverDetails, xray.WithScopedProjectKey(projectKey))
	if xrayErr != nil {
		return zerotouchremediation.SkipRemediation("Zero Touch Remediation skipped: could not create Xray service manager: ", xrayErr)
	}
	return zerotouchremediation.RunIfEnabled(ctx, xrayManager, resolverRepo, tool, command, workingDir, ca.npmBootstrapRunner(), bootstrapArgs...)
}

// resolverRepoForResolution returns the Artifactory virtual repo for dependency policy scope.
func (ca *CommonArgs) resolverRepoForResolution(command string) (string, error) {
	if command != "publish" && ca.repo != "" {
		return ca.repo, nil
	}
	if ca.configFilePath != "" {
		vConfig, err := project.ReadConfigFile(ca.configFilePath, project.YAML)
		if err != nil {
			return "", fmt.Errorf("failed to read config file: %w", err)
		}
		resolverConfig, err := project.GetRepoConfigByPrefix(ca.configFilePath, project.ProjectConfigResolverPrefix, vConfig)
		if err != nil {
			return "", fmt.Errorf("failed to get resolver config: %w", err)
		}
		return resolverConfig.TargetRepo(), nil
	}
	if ca.executablePath != "" {
		registryURL, err := ca.getNpmRegistryURL()
		if err != nil {
			return "", fmt.Errorf("failed to get registry URL: %w", err)
		}
		return extractRepoName(registryURL)
	}
	return ca.repo, nil
}

func (ca *CommonArgs) getNpmRegistryURL() (string, error) {
	configCommand := gofrogcmd.Command{
		Executable: ca.executablePath,
		CmdName:    "config",
		CmdArgs:    []string{"get", "registry"},
	}
	data, err := configCommand.RunWithOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (ca *CommonArgs) npmBootstrapRunner() zerotouchremediation.CommandRunner {
	return func(ctx context.Context, projectRoot string, args ...string) error {
		return runNpmAt(ctx, ca.executablePath, projectRoot, args...)
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
	return cnpm.HasPackageOperands(npmArgs)
}
