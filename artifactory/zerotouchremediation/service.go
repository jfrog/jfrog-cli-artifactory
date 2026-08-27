package zerotouchremediation

import (
	"context"
	"errors"
	"os"
	"path/filepath"

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
// Returns a restore function to revert lockfile writes if the subsequent build-tool command fails, and remediated=true when at least one lockfile was updated.
func RunIfEnabled(ctx context.Context, client ComponentResolutionClient, repo string, tool BuildTool, command, workingDir string, runner CommandRunner, bootstrapArgs ...string) (restore func() error, remediated bool, err error) {
	if !IsComponentResolutionEnabled() || !IsRelevantCommand(tool, command) {
		log.Debug("Zero Touch Remediation disabled or not relevant command: ", command)
		return noopRestore, false, nil
	}
	version, err := client.GetVersion()
	if err != nil {
		return noopRestore, false, err
	}
	log.Debug("Xray version: ", version)
	if versionErr := clientutils.ValidateMinimumVersion(clientutils.Xray, version, ZeroTouchRemediationMinVersion); versionErr != nil {
		return skipRemediationWarn("Zero Touch Remediation is not supported on the current Xray version. ", versionErr)
	}
	log.Debug("Running Zero Touch Remediation at '"+repo+"' RT repository for tool:", tool.ToolName())
	projectRoot, err := tool.ProjectRoot(workingDir)
	if err != nil {
		return noopRestore, false, err
	}
	log.Debug("Ensuring lockfiles in project root: ", projectRoot)
	bootstrapped, err := tool.EnsureLockfiles(ctx, projectRoot, command, runner, bootstrapArgs...)
	if err != nil {
		return noopRestore, false, err
	}
	lockfiles, err := tool.DiscoverLockfiles(workingDir)
	if err != nil {
		return noopRestore, false, err
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
			return noopRestore, false, errorutils.CheckError(err)
		}
		if disabled {
			log.Debug("Zero Touch Remediation skipped: the service is disabled on the server")
			return noopRestore, false, nil
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
		return noopRestore, false, nil
	}
	log.Debug("Applying", len(toWrite), "remediated lockfile(s)...")
	restore, err = ApplyLockfiles(projectRoot, toWrite, bootstrapped)
	if err != nil {
		return noopRestore, false, err
	}
	log.Info("Zero Touch Remediation applied", totalChanges, "package change(s) across", len(toWrite), "lockfile(s)")
	return restore, true, nil
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
		fullPath := filepath.Join(projectRoot, lf.Path)
		backup := lockfileBackup{path: fullPath}
		if !absent[lf.Path] {
			data, readErr := os.ReadFile(fullPath)
			if readErr == nil {
				backup.content = data
			} else if !os.IsNotExist(readErr) {
				return fail(readErr)
			}
		}

		if err = os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return fail(err)
		}
		if err = os.WriteFile(fullPath, lf.Content, 0644); err != nil {
			return fail(err)
		}
		backups = append(backups, backup)
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
