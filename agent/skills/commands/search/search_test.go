package search

import (
	"testing"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/stretchr/testify/require"
)

func TestPrintResults_DelegatesToCommon(t *testing.T) {
	sc := &SearchCommand{query: "q", format: "table"}
	err := sc.printResults(nil)
	require.NoError(t, err)
}

func TestSearchResultRowShape(t *testing.T) {
	_ = agentcommon.SearchResultRow{
		Name: "skill-a", Version: "1.0.0", Repository: "skills-local", Description: "desc",
	}
}
