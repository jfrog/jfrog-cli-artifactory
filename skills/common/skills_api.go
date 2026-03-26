package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type repoXrayConfig struct {
	XrayIndex *bool `json:"xrayIndex,omitempty"`
}

// WarnIfXrayDisabled fetches the repository configuration and warns if
// Xray indexing is not enabled, indicating security scanning is deactivated.
// The check is skipped entirely when JFROG_CLI_DISABLE_SKILLS_SCAN is set
// to "true" or "1".
func WarnIfXrayDisabled(serverDetails *config.ServerDetails, repoKey string) {
	if v := os.Getenv("JFROG_CLI_DISABLE_SKILLS_SCAN"); strings.EqualFold(v, "true") || v == "1" {
		log.Debug("Xray index check skipped (JFROG_CLI_DISABLE_SKILLS_SCAN is set)")
		return
	}

	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		log.Debug("Could not check repo xray config:", err.Error())
		return
	}
	var cfg repoXrayConfig
	if err := sm.GetRepository(repoKey, &cfg); err != nil {
		log.Debug("Could not fetch repo details:", err.Error())
		return
	}
	if cfg.XrayIndex == nil || !*cfg.XrayIndex {
		log.Warn("Preview version - security scanning is deactivated")
	}
}

func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]services.SkillVersion, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.ListSkillVersions(repoKey, slug)
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkills(repoKey, query, limit)
}

func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	return sm.SkillVersionExists(repoKey, slug, version)
}

// ResolveRepo determines the skills repository to use.
// Priority: flagValue (--repo) > JFROG_SKILLS_REPO env > auto-discover + interactive prompt.
func ResolveRepo(serverDetails *config.ServerDetails, flagValue string, quiet bool) (string, error) {
	if flagValue != "" {
		log.Debug("Using repo from --repo flag:", flagValue)
		return flagValue, nil
	}
	if envRepo := os.Getenv("JFROG_SKILLS_REPO"); envRepo != "" {
		log.Debug("Using repo from JFROG_SKILLS_REPO env:", envRepo)
		return envRepo, nil
	}

	if serverDetails == nil {
		return "", fmt.Errorf("server details are required to discover skills repositories; specify --repo or set JFROG_SKILLS_REPO")
	}

	repos, err := ListSkillsRepositories(serverDetails)
	if err != nil {
		return "", err
	}
	if len(repos) == 0 {
		return "", fmt.Errorf("no skills repositories found")
	}
	if len(repos) == 1 {
		log.Info("Using skills repository: " + repos[0])
		return repos[0], nil
	}

	if quiet || IsNonInteractive() {
		return "", fmt.Errorf("multiple skills repositories found (%s); specify --repo or set JFROG_SKILLS_REPO", strings.Join(repos, ", "))
	}

	options := make([]prompt.Suggest, len(repos))
	for i, r := range repos {
		options[i] = prompt.Suggest{Text: r}
	}
	selected := ioutils.AskFromListWithMismatchConfirmation(
		"Select a skills repository:",
		fmt.Sprintf("'%%s' is not in the list of discovered repos."),
		options,
	)
	return selected, nil
}

func ListSkillsRepositories(serverDetails *config.ServerDetails) ([]string, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	params := services.RepositoriesFilterParams{
		RepoType:    "local",
		PackageType: "skills",
	}
	repos, err := sm.GetAllRepositoriesFiltered(params)
	if err != nil {
		return nil, fmt.Errorf("failed to list skills repositories: %w", err)
	}
	var keys []string
	for _, r := range *repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}

func SearchSkillsByProperty(serverDetails *config.ServerDetails, query string) ([]services.SkillPropertySearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkillsByProperty(query)
}

// GetSkillDescription fetches the skill.description property for a given artifact path.
func GetSkillDescription(serverDetails *config.ServerDetails, repoPath string) (string, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return "", err
	}
	props, err := sm.GetItemProps(repoPath)
	if err != nil {
		return "", err
	}
	if descs, ok := props.Properties["skill.description"]; ok && len(descs) > 0 {
		return descs[0], nil
	}
	return "", nil
}
