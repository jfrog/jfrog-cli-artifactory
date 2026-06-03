package common

// Property keys and CLI output labels for agent plugin search.
const (
	SearchNamePropertyKey = "agentplugins.name"

	SearchTableTitle      = "Plugins"
	SearchEmptyTableLabel = "No plugins found"
	SearchNotFoundMessage = "No plugins found matching '%s'."
)

// SearchDescriptionPropertyKeys lists description property keys tried in order.
var SearchDescriptionPropertyKeys = []string{
	"agentplugins.description",
}
