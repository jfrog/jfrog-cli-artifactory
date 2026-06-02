package search

import (
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	plugincommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/stretchr/testify/require"
)

func TestSearchCommand_RunEmptyResults(t *testing.T) {
	sc := &SearchCommand{query: "missing", format: "table"}
	// No server details: SearchRowsByProperty fails before print; this test only covers print path.
	err := agentcommon.PrintSearchResults(nil, agentcommon.PrintSearchResultsOptions{
		Query:           sc.query,
		Format:          sc.format,
		TableTitle:      plugincommon.SearchTableTitle,
		EmptyTableLabel: plugincommon.SearchEmptyTableLabel,
		NotFoundMessage: plugincommon.SearchNotFoundMessage,
	})
	require.NoError(t, err)
}
