package npm

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
// Native npmrc can set both registry and @scope:registry; Xray accepts one repo, so this
// uses the unique Artifactory npm repo across those URLs and errors if more than one exists.
func (nc *NpmCommand) resolverRepoForResolution(registryURL string) (string, error) {
	if registryURL == "" && nc.repo != "" {
		return nc.repo, nil
	}
	listedConfig := false
	if nc.executablePath != "" {
		if data, err := npmUtils.GetConfigList(nc.npmArgs, nc.executablePath); err == nil {
			listedConfig = true
			repo, repoErr := resolverRepoFromNpmConfig(registryURL, data)
			if repoErr != nil {
				return "", repoErr
			}
			if repo != "" {
				return repo, nil
			}
		}
	}
	if registryURL != "" {
		return extractRepoName(registryURL)
	}
	if nc.repo != "" {
		return nc.repo, nil
	}
	if listedConfig || nc.executablePath == "" {
		return "", nil
	}
	registryURL, err := npmUtils.ConfigGet(nc.npmArgs, "registry", nc.executablePath)
	if err != nil {
		return "", fmt.Errorf("failed to get registry URL: %w", err)
	}
	return extractRepoName(registryURL)
}

func resolverRepoFromNpmConfig(cliRegistryURL string, configList []byte) (string, error) {
	repos := uniqueArtifactoryNpmRepos(npmConfigRegistryURLs(cliRegistryURL, configList))
	switch len(repos) {
	case 0:
		return "", nil
	case 1:
		return repos[0], nil
	default:
		return "", fmt.Errorf("multiple Artifactory npm registries in npm config: %s", strings.Join(repos, ", "))
	}
}

func npmConfigRegistryURLs(cliRegistryURL string, configList []byte) []string {
	cliOverridesDefault := cliRegistryURL != ""
	var urls []string
	if cliOverridesDefault {
		urls = append(urls, cliRegistryURL)
	}
	for _, rawLine := range strings.Split(string(configList), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if value == "" || value == "undefined" {
			continue
		}
		switch {
		case key == "registry" && !cliOverridesDefault:
			urls = append(urls, value)
		case strings.HasPrefix(key, "@") && strings.HasSuffix(key, ":registry"):
			urls = append(urls, value)
		}
	}
	return urls
}

func uniqueArtifactoryNpmRepos(urls []string) []string {
	seen := make(map[string]struct{})
	var repos []string
	for _, raw := range urls {
		if !isArtifactoryNpmRegistryURL(raw) {
			continue
		}
		repo, err := extractRepoName(raw)
		if err != nil || repo == "" {
			continue
		}
		if _, ok := seen[repo]; ok {
			continue
		}
		seen[repo] = struct{}{}
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos
}

func isArtifactoryNpmRegistryURL(registryURL string) bool {
	return strings.Contains(registryURL, "/api/npm/") || strings.Contains(registryURL, "/artifactory/")
}
