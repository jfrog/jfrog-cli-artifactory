package common

// Property keys used for agent plugin search (Artifactory property search API).
const (
	SearchNamePropertyKey = "agentplugins.name"
)

// SearchDescriptionPropertyKeys lists description property keys tried in order.
var SearchDescriptionPropertyKeys = []string{
	"agentplugins.description",
}
