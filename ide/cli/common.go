package cli

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const ideCategory = "IDE Integration"

// GetCommonServerFlags returns server configuration flags used by all IDE commands
func GetCommonServerFlags() []components.Flag {
	return []components.Flag{
		components.NewStringFlag("url", "JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
		components.NewStringFlag("user", "JFrog username.", components.SetMandatoryFalse()),
		components.NewStringFlag("password", "JFrog password.", components.SetMandatoryFalse()),
		components.NewStringFlag("access-token", "JFrog access token.", components.SetMandatoryFalse()),
		components.NewStringFlag("server-id", "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	}
}
