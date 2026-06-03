package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// SearchResultRow is one row in agent skills/plugins search output.
type SearchResultRow struct {
	Name        string `json:"name" col-name:"Name"`
	Version     string `json:"version" col-name:"Version"`
	Repository  string `json:"repository" col-name:"Repository"`
	Description string `json:"description" col-name:"Description"`
}

// PrintSearchResultsOptions configures table/json output for search commands.
type PrintSearchResultsOptions struct {
	Query           string
	Format          string
	TableTitle      string
	EmptyTableLabel string
	NotFoundMessage string
}

// PrintSearchResults prints search rows as a table or JSON.
func PrintSearchResults(rows []SearchResultRow, opts PrintSearchResultsOptions) error {
	if len(rows) == 0 {
		log.Info(fmt.Sprintf(opts.NotFoundMessage, opts.Query))
		return nil
	}
	if strings.EqualFold(opts.Format, "json") {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	return coreutils.PrintTable(rows, opts.TableTitle, opts.EmptyTableLabel, false)
}
