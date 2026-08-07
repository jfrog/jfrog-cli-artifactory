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
	// Validate before any filesystem path is built from these tokens —
	// FetchAndInstallPublicKey and the sources/preferences writers interpolate
	// repo/dist directly, so a "../" value could escape /etc/apt as root.
	if err := validateSourcesToken("repo", c.repoName); err != nil {
		return err
	}
	if err := validateSourcesToken("dist", c.dist); err != nil {
		return err
	}

	signedBy := ""
	if c.importKey {
		keyPath, err := FetchAndInstallPublicKey(c.serverDetails, c.repoName, c.dist)
		if err != nil {
			return wrapPermErr(fmt.Errorf("import GPG key: %w", err))
		}
		log.Output(fmt.Sprintf("Installed GPG public key at %s", keyPath))
		signedBy = keyPath
	} else if !c.trusted {
		// No --import-key, but a keyring from a previous import may already exist.
		// Reuse it so re-running setup keeps signature verification (signed-by)
		// rather than silently stripping it. Pass --import-key to refresh the key.
		if existingKey := existingKeyringPath(c.repoName, c.dist); existingKey != "" {
			log.Info(fmt.Sprintf("Reusing previously imported GPG key at %s (pass --import-key to refresh).", existingKey))
			signedBy = existingKey
		}
	}

	sourceLine, err := buildSourcesLine(c.serverDetails, c.repoName, c.dist, c.component, c.trusted, signedBy)
	if err != nil {
		return fmt.Errorf("build sources line: %w", err)
	}

	targetFile := fmt.Sprintf("%s/jfrog-%s-%s.list", sourcesListDir, c.repoName, c.dist)

	// Downgrade guard: the source previously pinned signing (signed-by=) but no
	// keyring was reused above (the .asc is gone) and no --trusted was given, so
	// the rewritten line would silently drop GPG verification. Warn instead.
	if signedBy == "" && !c.trusted && sourceHasSignedBy(targetFile) {
		log.Warn("This apt source was previously configured with GPG verification (signed-by), " +
			"but --import-key was not passed — the updated source will no longer verify signatures. " +
			"Re-run with --import-key to keep verification, or --trusted if disabling it is intentional.")
	}

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
	// Validate before matching so a crafted --dist/--repo cannot slip path
	// separators/".." into the comparison (glob metacharacters stay harmless
	// because the glob pattern below is the fixed "jfrog-*" and repo/dist are
	// only ever compared as literal prefix/suffix, never expanded).
	if c.dist != "" {
		if err := validateSourcesToken("dist", c.dist); err != nil {
			return err
		}
	}
	if c.repoName != "" {
		if err := validateSourcesToken("repo", c.repoName); err != nil {
			return err
		}
	}

	// Files are named jfrog-<repo>-<dist>.<ext>. Narrow by whichever of repo/dist
	// is set so `--remove --repo=A` only deletes repo A's config, not every repo's:
	//   repo+dist → prefix "jfrog-<repo>-" AND suffix "-<dist><ext>"
	//   repo only → prefix "jfrog-<repo>-"                (any dist for that repo)
	//   dist only → suffix "-<dist><ext>"                 (that dist for any repo)
	//   neither   → every jfrog-*<ext>
	removed := 0
	for _, dir := range []struct{ path, ext string }{
		{sourcesListDir, ".list"},
		{preferencesDir, ".pref"},
		{keyringsDir, ".asc"},
	} {
		matches, err := filepath.Glob(filepath.Join(dir.path, "jfrog-*"))
		if err != nil {
			return fmt.Errorf("glob %s: %w", dir.path, err)
		}
		for _, f := range matches {
			base := filepath.Base(f)
			if !strings.HasSuffix(base, dir.ext) {
				continue
			}
			if c.repoName != "" && !strings.HasPrefix(base, "jfrog-"+c.repoName+"-") {
				continue
			}
			if c.dist != "" && !strings.HasSuffix(base, "-"+c.dist+dir.ext) {
				continue
			}
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

// existingKeyringPath returns the keyring path for repo/dist when a previously
// imported ASCII-armored key (jfrog-<repo>-<dist>.asc) already exists on disk,
// else "". Used to preserve signature verification across setup re-runs that
// omit --import-key.
func existingKeyringPath(repoName, dist string) string {
	p := filepath.Join(keyringsDir, fmt.Sprintf("jfrog-%s-%s.asc", repoName, dist))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// sourceHasSignedBy reports whether the sources.list file at path already pins a
// signing key (signed-by=). A missing/unreadable file reports false.
func sourceHasSignedBy(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), "signed-by=")
}

// writeSourcesListIdempotent writes sourceLine to targetFile if the content has changed.
// Returns true if a write occurred, false if the file already contained the exact line.
func (c *AptSetupCommand) writeSourcesListIdempotent(targetFile, sourceLine string) (bool, error) {
	existing, err := os.ReadFile(targetFile)
	if err == nil {
		// Exact whole-line match only — a substring check would treat a narrower
		// config (e.g. "... noble main") as already present when the file holds a
		// broader line ("... noble main contrib"), silently keeping stale config.
		for _, line := range strings.Split(strings.TrimRight(string(existing), "\n"), "\n") {
			if line == sourceLine {
				return false, nil
			}
		}
		log.Info("Updating existing apt source configuration.")
	}
	if err := os.WriteFile(targetFile, []byte(sourceLine+"\n"), 0600); err != nil {
		return true, err
	}
	// os.WriteFile applies the mode only when it creates the file; on an existing
	// file the bits are left as-is. Force 0600 so a pre-existing looser file (older
	// binary, manual edit) is tightened — this file embeds credentials in the URL.
	return true, os.Chmod(targetFile, 0600)
}

// writePinningFile writes an apt preferences file that gives Artifactory
// packages priority 1001 — above apt's downgrade threshold (1000). So whenever
// Artifactory carries the package it ALWAYS wins version selection, even when
// that means installing an older Artifactory version over a newer one from
// another repo (a deliberate downgrade). If only another repo has the package,
// apt still installs it from there — this pins version preference for shared
// packages, it does not block packages Artifactory doesn't carry.
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
	// errors.Is walks the whole %w chain, unlike os.IsPermission which only
	// unwraps *PathError/*LinkError/*SyscallError one level — needed because
	// callers double-wrap (e.g. "import GPG key: %w" over "write public key: %w").
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w — you may need to run with sudo", err)
	}
	return err
}
