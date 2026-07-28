package search

import (
	"fmt"
	"os"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type SearchCommand struct {
	serverDetails *config.ServerDetails
	query         string
	repoKey       string
	format        string
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

func (sc *SearchCommand) Run() error {
	rows, err := agentcommon.SearchLatestRowsByProperty(sc.serverDetails, agentcommon.PropertySearchOptions{
		NamePropertyKey: plugincommon.SearchNamePropertyKey,
		Query:           wildcardSearchQuery(sc.query),
		RepoKey:         sc.repoKey,
	}, plugincommon.SearchDescriptionPropertyKeys)
	if err != nil {
		return fmt.Errorf("plugin search failed: %w", err)
	}
	return agentcommon.PrintSearchResults(rows, agentcommon.PrintSearchResultsOptions{
		Query:           sc.query,
		Format:          sc.format,
		TableTitle:      plugincommon.SearchTableTitle,
		EmptyTableLabel: plugincommon.SearchEmptyTableLabel,
		NotFoundMessage: plugincommon.SearchNotFoundMessage,
	})
}

// wildcardSearchQuery wraps query in "*...*" so search matches substrings, not just an exact
// plugin name — the underlying Artifactory property-search API only does exact matches
// unless the caller supplies wildcards itself. A query that already contains "*" is passed
// through unchanged so a caller who deliberately wrote their own pattern isn't double-wrapped.
func wildcardSearchQuery(query string) string {
	if strings.Contains(query, "*") {
		return query
	}
	return "*" + query + "*"
}

// RunSearch is the CLI action for `jf agent plugins search`.
func RunSearch(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins search <query> [--repo <repo>] [--format json]")
	}

	query := strings.TrimSpace(c.GetArgumentAt(0))
	if query == "" {
		return fmt.Errorf("search query cannot be empty. Usage: jf agent plugins search <query>")
	}

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	repoKey := strings.TrimSpace(c.GetStringFlagValue("repo"))
	if repoKey == "" {
		repoKey = os.Getenv(plugincommon.RepoEnvVar)
	}

	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	cmd := &SearchCommand{}
	cmd.SetServerDetails(serverDetails).
		SetQuery(query).
		SetRepoKey(repoKey).
		SetFormat(format)

	return cmd.Run()
}
