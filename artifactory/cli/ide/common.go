package ide

import (
	"fmt"
	"net/url"
	"strings"

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

// ExtractRepoKeyFromURL extracts the repository key from both JetBrains and VSCode extension URLs.
// For JetBrains: https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins
// For VSCode: https://mycompany.jfrog.io/artifactory/api/aieditorextensions/vscode-extensions/_apis/public/gallery
// Returns the repo key (e.g., "jetbrains-plugins" or "vscode-extensions")
func ExtractRepoKeyFromURL(repoURL string) (string, error) {
	if repoURL == "" {
		return "", fmt.Errorf("URL is empty")
	}

	url := strings.TrimSpace(repoURL)
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimSuffix(url, "/")

	// Check for JetBrains plugins API
	if idx := strings.Index(url, "/api/jetbrainsplugins/"); idx != -1 {
		rest := url[idx+len("/api/jetbrainsplugins/"):]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			return "", fmt.Errorf("repository key not found in JetBrains URL")
		}
		return parts[0], nil
	}

	// Check for VSCode extensions API
	if idx := strings.Index(url, "/api/aieditorextensions/"); idx != -1 {
		rest := url[idx+len("/api/aieditorextensions/"):]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			return "", fmt.Errorf("repository key not found in VSCode URL")
		}
		return parts[0], nil
	}

	return "", fmt.Errorf("URL does not contain a supported API type (/api/jetbrainsplugins/ or /api/aieditorextensions/)")
}

// IsValidUrl checks if a string is a valid URL with scheme and host
func IsValidUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}
