package apt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// AptCommand wraps apt-get/apt-cache/dpkg-query with JFrog Artifactory authentication.
//
// Authentication modes (design doc D3):
//   - Default: write temp sources.list with creds embedded in URL, inject via
//     apt-get -o Dir::Etc::sourcelist=<tmp>, defer cleanup.
//   - --skip-login: use system sources.list as-is.
//
// Dispatching: first arg selects the native tool.
//   - "apt-cache" or "dpkg-query" → that tool, remaining args, no auth injection
//   - anything else → apt-get, all args, with auth injection when --repo+--dist set
type AptCommand struct {
	args          []string
	skipLogin     bool
	trusted       bool
	serverDetails *config.ServerDetails
	repoName      string
	dist          string
	component     string
}

func NewAptCommand() *AptCommand {
	return &AptCommand{}
}

func (c *AptCommand) SetArgs(args []string) *AptCommand {
	c.args = args
	return c
}

func (c *AptCommand) SetSkipLogin(skip bool) *AptCommand {
	c.skipLogin = skip
	return c
}

func (c *AptCommand) SetTrusted(trusted bool) *AptCommand {
	c.trusted = trusted
	return c
}

func (c *AptCommand) SetServerDetails(serverDetails *config.ServerDetails) *AptCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *AptCommand) SetDist(dist string) *AptCommand {
	c.dist = dist
	return c
}

func (c *AptCommand) SetComponent(component string) *AptCommand {
	if component == "" {
		component = "main"
	}
	c.component = component
	return c
}

func (c *AptCommand) SetRepoName(repoName string) *AptCommand {
	c.repoName = repoName
	return c
}

func (c *AptCommand) CommandName() string { return "rt_apt" }

func (c *AptCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// nativeTools lists tools selectable as args[0] instead of the default apt-get.
var nativeTools = map[string]bool{
	"apt-cache":  true,
	"dpkg-query": true,
}

func needsUpdate(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			switch a {
			case "install", "upgrade", "dist-upgrade", "full-upgrade", "satisfy":
				return true
			}
			return false
		}
	}
	return false
}

// Run executes the native apt tool.
func (c *AptCommand) Run() error {
	if len(c.args) == 0 {
		return fmt.Errorf("no apt arguments provided")
	}

	nativeTool := "apt-get"
	nativeArgs := c.args
	if nativeTools[c.args[0]] {
		nativeTool = c.args[0]
		nativeArgs = c.args[1:]
	}

	if nativeTool == "apt-get" && !c.skipLogin {
		if c.serverDetails != nil && c.repoName != "" && c.dist != "" {
			tmpPath, err := WriteTempSourcesList(c.serverDetails, c.repoName, c.dist, c.component, c.trusted)
			if err != nil {
				log.Warn("Failed to create temporary sources.list — proceeding without auth injection: " + err.Error())
			} else {
				defer func() { _ = os.Remove(tmpPath) }()
				// Dir::Etc::sourcelist replaces the main sources.list; Dir::Etc::sourceparts=-
				// disables sources.list.d/ so ONLY the temp Artifactory entry is live for this
				// command — packages cannot resolve to any other configured repository.
				sourceOpts := []string{
					"-o", "Dir::Etc::sourcelist=" + tmpPath,
					"-o", "Dir::Etc::sourceparts=-",
				}
				log.Debug("Using temporary sources.list at: " + tmpPath)

				// Populate the package index before install/upgrade so apt can locate
				// packages that were never indexed by a prior apt-get update.
				// Skipped for subcommands that don't resolve packages (remove, purge, etc.)
				if needsUpdate(c.args) {
					log.Output("Updating package lists from Artifactory...")
					updateCmd := exec.Command("apt-get", append(sourceOpts, "update")...)
					updateCmd.Stdout = os.Stdout
					updateCmd.Stderr = os.Stderr
					if err := updateCmd.Run(); err != nil {
						return fmt.Errorf("apt-get update failed: %w", err)
					}
				}

				nativeArgs = append(sourceOpts, nativeArgs...)
			}
		} else {
			log.Warn("--repo and --dist not both specified — running apt-get without auth injection. " +
				"Pass --repo and --dist for on-the-fly auth, or run 'jf setup apt' for persistent auth.")
		}
	}

	cmd := exec.Command(nativeTool, nativeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", nativeTool, err)
	}
	return nil
}
