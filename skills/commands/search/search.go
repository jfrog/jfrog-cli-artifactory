package search

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type searchResult struct {
	Name        string `json:"name" col-name:"Name"`
	Version     string `json:"version" col-name:"Version"`
	Repository  string `json:"repository" col-name:"Repository"`
	Description string `json:"description" col-name:"Description"`
}

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
		discovered, err := common.ListSkillsRepositories(sc.serverDetails)
		if err != nil {
			return err
		}
		if len(discovered) == 0 {
			return fmt.Errorf("no skills repositories found")
		}
		repos = discovered
		log.Debug(fmt.Sprintf("Discovered %d skills repositories: %v", len(repos), repos))
	}

	var results []searchResult
	for _, repo := range repos {
		items, err := common.SearchSkills(sc.serverDetails, repo, sc.query, 50)
		if err != nil {
			log.Debug(fmt.Sprintf("Search failed for repo '%s': %s", repo, err.Error()))
			continue
		}
		for _, item := range items {
			results = append(results, searchResult{
				Name:        item.Name,
				Version:     item.Version,
				Repository:  repo,
				Description: item.Description,
			})
		}
	}

	return sc.printResults(results)
}

func (sc *SearchCommand) runPropSearch() error {
	propResults, err := common.SearchSkillsByProperty(sc.serverDetails, sc.query)
	if err != nil {
		return fmt.Errorf("property search failed: %w", err)
	}

	var results []searchResult
	for _, pr := range propResults {
		desc := ""
		repoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", pr.Repo, pr.Name, pr.Version, pr.Name, pr.Version)
		d, err := common.GetSkillDescription(sc.serverDetails, repoPath)
		if err != nil {
			log.Debug(fmt.Sprintf("Could not fetch description for %s: %s", repoPath, err.Error()))
		} else {
			desc = d
		}
		results = append(results, searchResult{
			Name:        pr.Name,
			Version:     pr.Version,
			Repository:  pr.Repo,
			Description: desc,
		})
	}

	return sc.printResults(results)
}

func (sc *SearchCommand) printResults(results []searchResult) error {
	if len(results) == 0 {
		log.Info(fmt.Sprintf("No skills found matching '%s'.", sc.query))
		return nil
	}

	if strings.EqualFold(sc.format, "json") {
		return printJSON(results)
	}
	return printTable(results)
}

func printJSON(results []searchResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printTable(results []searchResult) error {
	return coreutils.PrintTable(results, "Skills", "No skills found", false)
}

func RunSearch(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills search <query> [--repo <repo>] [--format json] [--prop]")
	}

	query := strings.TrimSpace(c.GetArgumentAt(0))
	if query == "" {
		return fmt.Errorf("search query cannot be empty. Usage: jf skills search <query>")
	}

	serverDetails, err := common.GetServerDetails(c)
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
