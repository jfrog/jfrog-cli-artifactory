package zerotouchremediation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/jfrog/jfrog-client-go/xray/services"
)

// ZtrComponentsEnabledEnvVar enables Zero Touch Remediation when set to "true".
// Feature is disabled by default.
const ZtrComponentsEnabledEnvVar = "JFROG_CLI_ZTR_COMPONENTS_ENABLED"

const ZeroTouchRemediationMinVersion = "3.154.0"

var noopRestore = func() error { return nil }

// writeFile is os.WriteFile; tests replace it to simulate truncate-then-fail.
var writeFile = os.WriteFile

// SkipRemediation logs and returns without error. Zero Touch Remediation is best-effort and must not fail the caller's build.
func SkipRemediation(message string, cause error) (func() error, bool, error) {
	if cause != nil {
		log.Debug(message + cause.Error())
	} else {
		log.Debug(message)
	}
	return noopRestore, false, nil
}

func skipRemediationWarn(message string, cause error) (func() error, bool, error) {
	if cause != nil {
		log.Warn(message + cause.Error())
	} else {
		log.Warn(message)
	}
	return noopRestore, false, nil
}

func IsComponentResolutionEnabled() bool {
	return os.Getenv(ZtrComponentsEnabledEnvVar) == "true"
}

// Lockfile is a CLI-side lock artifact. Path is relative to project root (read/write only — not sent to Xray).
type Lockfile struct {
	Path    string
	Content []byte
}

type lockfileBackup struct {
	path    string
	content []byte // nil means the file did not exist before apply
}

// ComponentResolutionClient resolves a single lockfile via Xray.
type ComponentResolutionClient interface {
	GetVersion() (string, error)
	ZeroTouchRemediation(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error)
}

// RunIfEnabled ensures lockfiles exist, discovers them, calls Xray once per file, writes remediated lockfiles when changes returned.
// Operational failures are skipped (warn + continue) so remediation never fails the caller's build.
// Returns a restore function to revert lockfile writes if the subsequent build-tool command fails, and remediated=true when at least one lockfile was updated.
func RunIfEnabled(ctx context.Context, client ComponentResolutionClient, repo string, tool BuildTool, command, workingDir string, runner CommandRunner, bootstrapArgs ...string) (restore func() error, remediated bool, err error) {
	if !IsComponentResolutionEnabled() || !IsRelevantCommand(tool, command) {
		log.Debug("Zero Touch Remediation disabled or not relevant command: ", command)
		return noopRestore, false, nil
	}
	version, err := client.GetVersion()
	if err != nil {
		return skipRemediationWarn("Zero Touch Remediation skipped: could not get Xray version: ", err)
	}
	log.Debug("Xray version: ", version)
	if versionErr := clientutils.ValidateMinimumVersion(clientutils.Xray, version, ZeroTouchRemediationMinVersion); versionErr != nil {
		return skipRemediationWarn("Zero Touch Remediation is not supported on the current Xray version. ", versionErr)
	}
	log.Debug("Running Zero Touch Remediation at '"+repo+"' RT repository for tool:", tool.ToolName())
	projectRoot, err := tool.ProjectRoot(workingDir)
	if err != nil {
		return skipRemediationWarn("Zero Touch Remediation skipped: could not resolve project root: ", err)
	}
	log.Debug("Ensuring lockfiles in project root: ", projectRoot)
	bootstrapped, err := tool.EnsureLockfiles(ctx, projectRoot, command, runner, bootstrapArgs...)
	if err != nil {
		return skipRemediationWarn("Zero Touch Remediation skipped: ", err)
	}
	lockfiles, err := tool.DiscoverLockfiles(workingDir)
	if err != nil {
		return skipRemediationWarn("Zero Touch Remediation skipped: could not discover lockfiles: ", err)
	}
	log.Debug("Discovered lockfiles: ", getLockfilePaths(lockfiles))
	var toWrite []Lockfile
	var totalChanges int
	for _, lf := range lockfiles {
		resp, disabled, err := client.ZeroTouchRemediation(services.ComponentResolutionRequest{
			BuildTool: tool.ToolName(),
			Repo:      repo,
			Lockfile:  string(lf.Content),
		})
		if err != nil {
			return skipRemediationWarn("Zero Touch Remediation skipped: ", err)
		}
		if disabled {
			log.Debug("Zero Touch Remediation skipped: the service is disabled on the server")
			return restoreAbsentLockfiles(projectRoot, bootstrapped), false, nil
		}
		if len(resp.Changes) == 0 {
			log.Debug("No changes for ", lf.Path)
			continue
		}
		toWrite = append(toWrite, Lockfile{Path: lf.Path, Content: []byte(resp.Lockfile)})
		totalChanges += len(resp.Changes)
		log.Debug("Remediated", lf.Path, "with", len(resp.Changes), "package change(s)")
		for _, ch := range resp.Changes {
			log.Debug("  ", ch.Package, ":", ch.BeforeIntegrity, "→", ch.AfterIntegrity)
		}
	}
	if len(toWrite) == 0 {
		return restoreAbsentLockfiles(projectRoot, bootstrapped), false, nil
	}
	log.Debug("Applying", len(toWrite), "remediated lockfile(s)...")
	restore, err = ApplyLockfiles(projectRoot, toWrite, bootstrapped)
	if err != nil {
		return skipRemediationWarn("Zero Touch Remediation skipped: failed to apply remediated lockfiles: ", err)
	}
	restore = composeRestore(restore, restoreAbsentLockfiles(projectRoot, bootstrappedNotWritten(bootstrapped, toWrite)))
	log.Info("Zero Touch Remediation applied", totalChanges, "package change(s) across", len(toWrite), "lockfile(s)")
	return restore, true, nil
}

func bootstrappedNotWritten(bootstrapped []string, written []Lockfile) []string {
	inWritten := make(map[string]bool, len(written))
	for _, lf := range written {
		inWritten[lf.Path] = true
	}
	var leftover []string
	for _, path := range bootstrapped {
		if !inWritten[path] {
			leftover = append(leftover, path)
		}
	}
	return leftover
}

func restoreAbsentLockfiles(projectRoot string, relPaths []string) func() error {
	if len(relPaths) == 0 {
		return noopRestore
	}
	return func() error {
		var restoreErr error
		for _, rel := range relPaths {
			fullPath, pathErr := containedLockfilePath(projectRoot, rel)
			if pathErr != nil {
				restoreErr = errors.Join(restoreErr, pathErr)
				continue
			}
			if rbErr := restoreLockfileBackup(lockfileBackup{path: fullPath}); rbErr != nil {
				restoreErr = errors.Join(restoreErr, rbErr)
			}
		}
		return restoreErr
	}
}

func composeRestore(fns ...func() error) func() error {
	return func() error {
		var restoreErr error
		for _, fn := range fns {
			if fn == nil {
				continue
			}
			restoreErr = errors.Join(restoreErr, fn())
		}
		return restoreErr
	}
}

func getLockfilePaths(lockfiles []Lockfile) []string {
	paths := make([]string, 0, len(lockfiles))
	for _, lf := range lockfiles {
		paths = append(paths, lf.Path)
	}
	return paths
}

// ApplyLockfiles backs up existing lockfiles under projectRoot, writes updates, returns restore func.
// Paths listed in treatAsAbsent are restored by deletion even if they exist on disk (bootstrapped locks).
func ApplyLockfiles(projectRoot string, lockfiles []Lockfile, treatAsAbsent []string) (restore func() error, err error) {
	if len(lockfiles) == 0 {
		return func() error { return nil }, nil
	}

	absent := make(map[string]bool, len(treatAsAbsent))
	for _, path := range treatAsAbsent {
		absent[path] = true
	}

	var backups []lockfileBackup
	restoreWritten := func() error {
		var restoreErr error
		for _, backup := range backups {
			if rbErr := restoreLockfileBackup(backup); rbErr != nil {
				restoreErr = errors.Join(restoreErr, rbErr)
			}
		}
		return restoreErr
	}
	fail := func(cause error) (func() error, error) {
		if rbErr := restoreWritten(); rbErr != nil {
			return nil, errors.Join(errorutils.CheckError(cause), rbErr)
		}
		return nil, errorutils.CheckError(cause)
	}

	for _, lf := range lockfiles {
		fullPath, pathErr := containedLockfilePath(projectRoot, lf.Path)
		if pathErr != nil {
			return fail(pathErr)
		}
		if linkErr := rejectSymlinksUnderRoot(projectRoot, fullPath); linkErr != nil {
			return fail(linkErr)
		}
		backup := lockfileBackup{path: fullPath}
		if !absent[lf.Path] {
			data, readErr := os.ReadFile(fullPath)
			if readErr == nil {
				backup.content = data
			} else if !os.IsNotExist(readErr) {
				return fail(readErr)
			}
		}

		backups = append(backups, backup)
		if err = os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fail(err)
		}
		if err = writeFile(fullPath, lf.Content, 0644); err != nil {
			return fail(err)
		}
	}

	return restoreWritten, nil
}

func restoreLockfileBackup(backup lockfileBackup) error {
	if backup.content == nil {
		if err := os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
			return errorutils.CheckError(err)
		}
		return nil
	}
	return errorutils.CheckError(os.WriteFile(backup.path, backup.content, 0644))
}

func containedLockfilePath(projectRoot, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", errorutils.CheckErrorf("lockfile path %q is not relative to the project root", rel)
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	full := filepath.Clean(filepath.Join(root, rel))
	relToRoot, err := filepath.Rel(root, full)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", errorutils.CheckErrorf("lockfile path %q escapes project root", rel)
	}
	return full, nil
}

// rejectSymlinksUnderRoot Lstats each path component from projectRoot to fullPath,
// not including projectRoot itself. Walking above the root would reject macOS
// TempDirs whose prefix is /var → /private/var.
func rejectSymlinksUnderRoot(projectRoot, fullPath string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return errorutils.CheckError(err)
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return errorutils.CheckError(err)
	}
	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if linkErr := rejectSymlink(current); linkErr != nil {
			return linkErr
		}
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errorutils.CheckError(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errorutils.CheckErrorf("refusing to write lockfile through symlink %s", path)
	}
	return nil
}
