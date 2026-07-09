package apt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var (
	sourcesListDir = "/etc/apt/sources.list.d"
	preferencesDir = "/etc/apt/preferences.d"
)

// AptSetupCommand writes a persistent Artifactory sources.list entry for apt.
//
// Writes to: /etc/apt/sources.list.d/jfrog-<repo>-<dist>.list
// Format:    deb https://user:token@host/artifactory/repo DIST COMPONENT
//
// Idempotent: re-running with the same repo+dist replaces the existing file.
// Requires root (euid == 0).
type AptSetupCommand struct {
	serverDetails *config.ServerDetails
	repoName      string
	dist          string
	component     string
	trusted       bool
	importKey     bool
	remove        bool
}

func NewAptSetupCommand() *AptSetupCommand {
	return &AptSetupCommand{}
}

func (c *AptSetupCommand) SetServerDetails(serverDetails *config.ServerDetails) *AptSetupCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *AptSetupCommand) SetRepoName(repoName string) *AptSetupCommand {
	c.repoName = repoName
	return c
}

func (c *AptSetupCommand) SetDist(dist string) *AptSetupCommand {
	c.dist = dist
	return c
}

func (c *AptSetupCommand) SetTrusted(trusted bool) *AptSetupCommand {
	c.trusted = trusted
	return c
}

func (c *AptSetupCommand) SetImportKey(importKey bool) *AptSetupCommand {
	c.importKey = importKey
	return c
}

func (c *AptSetupCommand) SetRemove(remove bool) *AptSetupCommand {
	c.remove = remove
	return c
}

func (c *AptSetupCommand) SetComponent(component string) *AptSetupCommand {
	if component == "" {
		component = "main"
	}
	c.component = component
	return c
}

func (c *AptSetupCommand) CommandName() string { return "setup_apt" }

func (c *AptSetupCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// Run writes /etc/apt/sources.list.d/jfrog-<repo>-<dist>.list with credentials
// embedded in the repository URL, then runs apt-get update to verify.
// With --remove, deletes all jfrog-*.list and jfrog-*.pref files (filtered by
// --dist if provided) instead of writing.
func (c *AptSetupCommand) Run() error {
	if c.remove {
		return c.runRemove()
	}
	if c.trusted && c.importKey {
		return fmt.Errorf("--trusted and --import-key are mutually exclusive")
	}
	if c.repoName == "" {
		return fmt.Errorf("--repo is required for apt setup")
	}
	if c.dist == "" {
		return fmt.Errorf("--dist is required for apt setup")
	}
	if c.serverDetails == nil {
		return fmt.Errorf("server details not configured; use --server-id or 'jf config add'")
	}

	signedBy := ""
	if c.importKey {
		keyPath, err := FetchAndInstallPublicKey(c.serverDetails, c.repoName, c.dist)
		if err != nil {
			return wrapPermErr(fmt.Errorf("import GPG key: %w", err))
		}
		log.Output(fmt.Sprintf("Installed GPG public key at %s", keyPath))
		signedBy = keyPath
	}

	sourceLine, err := buildSourcesLine(c.serverDetails, c.repoName, c.dist, c.component, c.trusted, signedBy)
	if err != nil {
		return fmt.Errorf("build sources line: %w", err)
	}

	targetFile := fmt.Sprintf("%s/jfrog-%s-%s.list", sourcesListDir, c.repoName, c.dist)

	wrote, err := c.writeSourcesListIdempotent(targetFile, sourceLine)
	if err != nil {
		return wrapPermErr(err)
	}

	artHost := extractHost(c.serverDetails.GetArtifactoryUrl())
	prefFile := fmt.Sprintf("%s/jfrog-%s-%s.pref", preferencesDir, c.repoName, c.dist)
	if err := writePinningFile(prefFile, artHost); err != nil {
		return wrapPermErr(fmt.Errorf("write apt pinning file: %w", err))
	}

	if !wrote {
		log.Output("Apt source already configured — no changes needed.")
		return nil
	}

	log.Output(fmt.Sprintf("Wrote %s", targetFile))

	updateCmd := exec.Command("apt-get", "update")
	var stderrBuf bytes.Buffer
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if err := updateCmd.Run(); err != nil {
		// apt-get exits 100 for many reasons (permissions, connectivity, bad
		// config), so decide the hint from what apt actually reported rather
		// than assuming from the current uid — a non-root user may hold all the
		// needed permissions, in which case the failure is not a sudo problem.
		if isAptPermissionError(stderrBuf.String()) {
			return fmt.Errorf("apt-get update failed — you may need to run with sudo: %w", err)
		}
		return fmt.Errorf("apt-get update failed — check connectivity and credentials: %w", err)
	}

	log.Output(fmt.Sprintf("Successfully configured apt to use JFrog Artifactory repository '%s'.", c.repoName))
	return nil
}

// runRemove deletes all jfrog-managed sources.list and preferences files.
// If --dist is set, only files matching that dist suffix are removed.
func (c *AptSetupCommand) runRemove() error {
	suffix := ".list"
	prefSuffix := ".pref"
	keyringSuffix := ".asc"
	if c.dist != "" {
		suffix = "-" + c.dist + ".list"
		prefSuffix = "-" + c.dist + ".pref"
		keyringSuffix = "-" + c.dist + ".asc"
	}

	removed := 0
	for _, dir := range []struct{ path, suf string }{
		{sourcesListDir, suffix},
		{preferencesDir, prefSuffix},
		{keyringsDir, keyringSuffix},
	} {
		matches, err := filepath.Glob(filepath.Join(dir.path, "jfrog-*"+dir.suf))
		if err != nil {
			return fmt.Errorf("glob %s: %w", dir.path, err)
		}
		for _, f := range matches {
			if err := os.Remove(f); err != nil {
				return wrapPermErr(fmt.Errorf("remove %s: %w", f, err))
			}
			log.Output(fmt.Sprintf("Removed %s", f))
			removed++
		}
	}

	if removed == 0 {
		log.Output("No JFrog apt configuration found to remove.")
	}
	return nil
}

// writeSourcesListIdempotent writes sourceLine to targetFile if the content has changed.
// Returns true if a write occurred, false if the file already contained the exact line.
func (c *AptSetupCommand) writeSourcesListIdempotent(targetFile, sourceLine string) (bool, error) {
	existing, err := os.ReadFile(targetFile)
	if err == nil {
		if strings.Contains(string(existing), sourceLine) {
			return false, nil
		}
		log.Info("Updating existing apt source configuration.")
	}
	return true, os.WriteFile(targetFile, []byte(sourceLine+"\n"), 0600)
}

// writePinningFile writes an apt preferences file that gives Artifactory
// packages priority 1001 — higher than any default (990) or pinned (1000)
// source. apt will prefer Artifactory when both it and another repo have
// the package; if only another repo has it, apt still installs from there.
// This is not a hard block but prevents silent downgrades to non-Artifactory
// versions when both sources carry the same package.
func writePinningFile(path, artHost string) error {
	content := fmt.Sprintf("Package: *\nPin: origin %s\nPin-Priority: 1001\n", artHost)
	return os.WriteFile(path, []byte(content), 0644)
}

// extractHost returns the hostname from a URL, falling back to the raw string.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Hostname()
}

// isAptPermissionError reports whether apt-get output indicates a permission
// problem (typically the /var/lib/apt or /var/lib/dpkg locks) rather than a
// connectivity or credentials failure. apt-get itself exits 100 for all of
// these, so its stderr is the only reliable discriminator.
func isAptPermissionError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "are you root") ||
		strings.Contains(lower, "could not open lock file")
}

// wrapPermErr appends a sudo hint to permission-denied errors.
func wrapPermErr(err error) error {
	if err == nil {
		return nil
	}
	if os.IsPermission(err) || os.IsPermission(errors.Unwrap(err)) {
		return fmt.Errorf("%w — you may need to run with sudo", err)
	}
	return err
}
