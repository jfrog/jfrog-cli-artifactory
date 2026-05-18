package agentcommon

import (
	"fmt"
	"os"
	"strings"

	prompt "github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ResolveRepoOptions parameterizes repo resolution across different agent package types.
type ResolveRepoOptions struct {
	PackageType string // Artifactory package type (e.g. "agentplugins")
	EnvVar      string // env var consulted before discovery (e.g. "JFROG_AGENT_PLUGINS_REPO")
	Label       string // human-readable label used in prompts and logs (e.g. "agent plugins")
}

// ResolveRepo determines the repository to use for the given agent package type.
// Priority: flagValue (--repo) > options.EnvVar > auto-discover + interactive prompt.
func ResolveRepo(serverDetails *config.ServerDetails, flagValue string, quiet bool, opts ResolveRepoOptions) (string, error) {
	if flagValue != "" {
		log.Debug("Using repo from --repo flag:", flagValue)
		return flagValue, nil
	}
	if opts.EnvVar != "" {
		if envRepo := os.Getenv(opts.EnvVar); envRepo != "" {
			log.Debug(fmt.Sprintf("Using repo from %s env: %s", opts.EnvVar, envRepo))
			return envRepo, nil
		}
	}

	if serverDetails == nil {
		return "", fmt.Errorf("server details are required to discover %s repositories; specify --repo or set %s", opts.Label, opts.EnvVar)
	}

	repos, err := ListRepositoriesByPackageType(serverDetails, opts.PackageType)
	if err != nil {
		return "", err
	}
	if len(repos) == 0 {
		return "", fmt.Errorf("no %s repositories found", opts.Label)
	}
	if len(repos) == 1 {
		log.Info(fmt.Sprintf("Using %s repository: %s", opts.Label, repos[0]))
		return repos[0], nil
	}

	if quiet || IsNonInteractive() {
		return "", fmt.Errorf("multiple %s repositories found (%s); specify --repo or set %s", opts.Label, strings.Join(repos, ", "), opts.EnvVar)
	}

	options := make([]prompt.Suggest, len(repos))
	for i, r := range repos {
		options[i] = prompt.Suggest{Text: r}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		fmt.Sprintf("Select a %s repository:", opts.Label),
		"'%s' is not in the list of discovered repos.",
		options,
	)
	return selected, nil
}

// ListRepositoriesByPackageType returns the keys of all local repositories of the given package type.
func ListRepositoriesByPackageType(serverDetails *config.ServerDetails, packageType string) ([]string, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	params := services.RepositoriesFilterParams{
		RepoType:    "local",
		PackageType: packageType,
	}
	repos, err := sm.GetAllRepositoriesFiltered(params)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s repositories: %w", packageType, err)
	}
	keys := make([]string, 0, len(*repos))
	for _, r := range *repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}
