package npm

import (
	"context"
	"fmt"

	biUtils "github.com/jfrog/build-info-go/build/utils"
	npmUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/npm"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"
	"github.com/jfrog/jfrog-client-go/utils/log"

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
	parsedArgs := parseNpmCLIArgs(npmArgs)
	if command == "install" && len(parsedArgs.packageOperands) > 0 {
		return func() error { return nil }, false, nil
	}
	resolverRepo, resolverErr := nc.resolverRepoForResolution(parsedArgs.registryURL)
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
	tool := BuildTool{opts: discoveryOptions{prefixDir: parsedArgs.prefixDir}}
	runner := func(_ context.Context, projectRoot string, args ...string) error {
		_, _, err := biUtils.RunNpmCmd(nc.executablePath, projectRoot, args, log.Logger)
		return err
	}
	return zerotouchremediation.RunIfEnabled(ctx, xrayManager, resolverRepo, tool, command, workingDir, runner, parsedArgs.bootstrapArgs...)
}

// resolverRepoForResolution returns the Artifactory virtual repo for dependency policy scope.
func (nc *NpmCommand) resolverRepoForResolution(registryURL string) (string, error) {
	if registryURL != "" {
		return extractRepoName(registryURL)
	}
	if nc.repo != "" {
		return nc.repo, nil
	}
	if nc.executablePath != "" {
		registryURL, err := npmUtils.ConfigGet(nc.npmArgs, "registry", nc.executablePath)
		if err != nil {
			return "", fmt.Errorf("failed to get registry URL: %w", err)
		}
		return extractRepoName(registryURL)
	}
	return "", nil
}
