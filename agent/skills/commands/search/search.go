package search

import (
	"fmt"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-artifactory/agent/skills/common"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type SearchCommand struct {
	serverDetails *config.ServerDetails
	query         string
	repoKey       string
	format        string
	propSearch    bool
}

func (sc *SearchCommand) SetServerDetails(details *config.ServerDetails) *SearchCommand {
	sc.serverDetails = details
	return sc
}

func (sc *SearchCommand) SetQuery(query string) *SearchCommand {
	sc.query = query
	return sc
}

func (sc *SearchCommand) SetRepoKey(repoKey string) *SearchCommand {
	sc.repoKey = repoKey
	return sc
}

func (sc *SearchCommand) SetFormat(format string) *SearchCommand {
	sc.format = format
	return sc
}

func (sc *SearchCommand) SetPropSearch(prop bool) *SearchCommand {
	sc.propSearch = prop
	return sc
}

func (sc *SearchCommand) Run() error {
	if sc.propSearch {
		return sc.runPropSearch()
	}
	return sc.runSkillsAPISearch()
}

func (sc *SearchCommand) runSkillsAPISearch() error {
	var repos []string
	if sc.repoKey != "" {
		repos = []string{sc.repoKey}
	} else {
		discovered, err := agentcommon.ListRepositoriesByPackageType(sc.serverDetails, common.PackageType)
		if err != nil {
			return err
		}
		if len(discovered) == 0 {
			return fmt.Errorf("no skills repositories found")
		}
		repos = discovered
		log.Debug(fmt.Sprintf("Discovered %d skills repositories: %v", len(repos), repos))
	}

	var results []agentcommon.SearchResultRow
	var failedRepos []string
	var firstErr error
	for _, repo := range repos {
		items, err := common.SearchSkills(sc.serverDetails, repo, sc.query, 50)
		if err != nil {
			log.Warn(fmt.Sprintf("Skills search failed for repo '%s': %s", repo, err.Error()))
			failedRepos = append(failedRepos, repo)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, item := range items {
			results = append(results, agentcommon.SearchResultRow{
				Name:        item.Name,
				Version:     item.Version,
				Repository:  repo,
				Description: item.Description,
			})
		}
	}

	if len(results) == 0 && len(failedRepos) == len(repos) {
		return fmt.Errorf("skills search for %q failed in %s: %w",
			sc.query, strings.Join(failedRepos, ", "), firstErr)
	}

	return sc.printResults(results)
}

func (sc *SearchCommand) runPropSearch() error {
	rows, err := agentcommon.SearchRowsByProperty(sc.serverDetails, agentcommon.PropertySearchOptions{
		NamePropertyKey: common.SearchNamePropertyKey,
		Query:           sc.query,
		RepoKey:         sc.repoKey,
	}, common.SearchDescriptionPropertyKeys)
	if err != nil {
		return fmt.Errorf("property search failed: %w", err)
	}
	return sc.printResults(rows)
}

func (sc *SearchCommand) printResults(results []agentcommon.SearchResultRow) error {
	return agentcommon.PrintSearchResults(results, agentcommon.PrintSearchResultsOptions{
		Query:           sc.query,
		Format:          sc.format,
		TableTitle:      "Skills",
		EmptyTableLabel: "No skills found",
		NotFoundMessage: "No skills found matching '%s'.",
	})
}

// RunSearch is the CLI action for `jf agent skills search`.
func RunSearch(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent skills search <query> [--repo <repo>] [--format json] [--prop]")
	}

	query := strings.TrimSpace(c.GetArgumentAt(0))
	if query == "" {
		return fmt.Errorf("search query cannot be empty. Usage: jf agent skills search <query>")
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	cmd := &SearchCommand{}
	cmd.SetServerDetails(serverDetails).
		SetQuery(query).
		SetRepoKey(c.GetStringFlagValue("repo")).
		SetFormat(format).
		SetPropSearch(c.GetBoolFlagValue("prop"))

	return cmd.Run()
}

func GetSearchFlags() []components.Flag {
	return flagkit.GetCommandFlags(flagkit.SkillsSearch)
}
