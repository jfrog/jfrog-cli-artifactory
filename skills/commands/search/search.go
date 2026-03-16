package search

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
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

	// Fan out searches across all repos concurrently, bounded to 10 parallel calls.
	type repoItems struct {
		repo  string
		items []services.SkillSearchResult
	}
	repoResults := make([]repoItems, len(repos))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items, err := common.SearchSkills(sc.serverDetails, r, sc.query, 50)
			if err != nil {
				log.Debug(fmt.Sprintf("Search failed for repo '%s': %s", r, err.Error()))
				return
			}
			repoResults[idx] = repoItems{repo: r, items: items}
		}(i, repo)
	}
	wg.Wait()

	var results []searchResult
	for _, rr := range repoResults {
		for _, item := range rr.items {
			results = append(results, searchResult{
				Name:        item.Name,
				Version:     item.Version,
				Repository:  rr.repo,
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

	// Fan out description fetches concurrently, bounded to 10 parallel calls.
	results := make([]searchResult, len(propResults))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for i, pr := range propResults {
		wg.Add(1)
		go func(idx int, p services.SkillPropertySearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			desc := ""
			repoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", p.Repo, p.Name, p.Version, p.Name, p.Version)
			d, descErr := common.GetSkillDescription(sc.serverDetails, repoPath)
			if descErr != nil {
				log.Debug(fmt.Sprintf("Could not fetch description for %s: %s", repoPath, descErr.Error()))
			} else {
				desc = d
			}
			results[idx] = searchResult{
				Name:        p.Name,
				Version:     p.Version,
				Repository:  p.Repo,
				Description: desc,
			}
		}(i, pr)
	}
	wg.Wait()

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

	query := c.GetArgumentAt(0)

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
