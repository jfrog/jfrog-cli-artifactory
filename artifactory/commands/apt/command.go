package apt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

// aptValueFlags are apt-get flags whose value is a separate argv token
// (e.g. `-o KEY=VALUE`, `-t release`). The value token must be skipped so it is
// not mistaken for the subcommand.
var aptValueFlags = map[string]bool{
	"-o": true, "--option": true,
	"-c": true, "--config-file": true,
	"-t": true, "--target-release": true,
}

func needsUpdate(args []string) bool {
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			if aptValueFlags[a] {
				skipNext = true
			}
			continue
		}
		switch a {
		case "install", "upgrade", "dist-upgrade", "full-upgrade", "satisfy":
			return true
		}
		return false
	}
	return false
}

// Run executes the native apt tool.
// hasPersistentAptConfig reports whether 'jf setup apt' has written a persistent
// sources.list entry (jfrog-*.list). When present, native apt-get already resolves
// against Artifactory with embedded credentials, so no temp source is required.
func hasPersistentAptConfig() bool {
	matches, err := filepath.Glob(filepath.Join(sourcesListDir, "jfrog-*.list"))
	return err == nil && len(matches) > 0
}

// sweepStaleTempSources best-effort removes leftover on-the-fly sources.list temp
// files (which embed credentials in the repo URL) abandoned by a previous run
// that was killed — OOM, SIGKILL, CI cancellation — before its deferred
// os.Remove ran. Only files older than one hour are removed, so a concurrent
// in-flight `jf apt` is never disturbed. Errors are ignored: this is hygiene, not
// correctness.
func sweepStaleTempSources() {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "jfrog-apt-sources-*.list"))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, f := range matches {
		if info, err := os.Stat(f); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(f)
		}
	}
}

func (c *AptCommand) Run() error {
	if len(c.args) == 0 {
		return fmt.Errorf("no apt arguments provided")
	}

	// Default the component so a missing --component never makes buildSourcesLine
	// reject an empty token and silently disable auth injection on the on-the-fly path.
	if c.component == "" {
		c.component = "main"
	}

	nativeTool := "apt-get"
	nativeArgs := c.args
	if nativeTools[c.args[0]] {
		nativeTool = c.args[0]
		nativeArgs = c.args[1:]
	}

	if nativeTool == "apt-get" && !c.skipLogin {
		usePersistentConfig := func() {
			// 'jf setup apt' already wrote a persistent jfrog-*.list with embedded
			// credentials. Native apt-get resolves against it directly — no temp
			// source needed, and no missing-auth warning is warranted.
			log.Info("Using persistent Artifactory apt configuration from " + sourcesListDir +
				" (written by 'jf setup apt').")
		}
		switch {
		case c.serverDetails != nil && c.repoName != "" && c.dist != "":
			// Best-effort: clear credential-bearing temp files abandoned by a prior
			// run killed before its deferred cleanup ran (see sweepStaleTempSources).
			sweepStaleTempSources()
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
		case (c.repoName != "") != (c.dist != ""):
			// Exactly one of --repo/--dist was given (the both-set case matched above).
			// On-the-fly auth needs both, so the partial flag can't be honored — warn
			// rather than silently ignoring it, then fall back to persistent config.
			log.Warn("On-the-fly auth requires both --repo and --dist — the partial flag was ignored.")
			if hasPersistentAptConfig() {
				usePersistentConfig()
			}
		case hasPersistentAptConfig():
			usePersistentConfig()
		default:
			log.Warn("--repo and --dist not both specified and no persistent 'jf setup apt' " +
				"configuration found — running apt-get without auth injection. Pass --repo and " +
				"--dist for on-the-fly auth, or run 'jf setup apt' first for persistent auth.")
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
