package ide

import (
	"fmt"

	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// ValidateSingleNonEmptyArg checks that there is exactly one argument and it is not empty.
func ValidateSingleNonEmptyArg(c *components.Context, usage string) (string, error) {
	if c.GetNumberOfArgs() != 1 {
		return "", pluginsCommon.WrongNumberOfArgumentsHandler(c)
	}
	arg := c.GetArgumentAt(0)
	if arg == "" {
		return "", fmt.Errorf("argument cannot be empty\n\nUsage: %s", usage)
	}
	return arg, nil
}

// HasServerConfigFlags checks if any server configuration flags are provided
func HasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		// Only consider password if other required fields are also provided
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
}
