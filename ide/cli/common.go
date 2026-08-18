package cli

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const ideCategory = "IDE Integration"

// GetCommonServerFlags returns server configuration flags used by all IDE commands
func GetCommonServerFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag("url", "JFrog Artifactory base URL (example: https://acme.jfrog.io/artifactory). When set, you must also pass --access-token, or --user and --password. To use a server saved with 'jf config add', drop --url and pass --server-id alone.", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username. Must be combined with --password.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password. Must be combined with --user.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}
}
