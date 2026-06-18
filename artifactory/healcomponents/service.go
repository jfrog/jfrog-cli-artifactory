package healcomponents

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/jfrog/jfrog-client-go/xray/services"
)

// HealComponentsDisabledEnvVar disables Xray heal components when set to "true".
// Feature is enabled by default; users run normal jf <build-tool> commands with no extra setup.
const HealComponentsDisabledEnvVar = "JFROG_CLI_HEAL_COMPONENTS_DISABLED"

var noopRestore = func() error { return nil }

func IsComponentResolutionDisabled() bool {
	return os.Getenv(HealComponentsDisabledEnvVar) == "true"
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
	HealComponents(req services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, error)
}

// RunIfEnabled ensures lockfiles exist, discovers them, calls Xray once per file, writes healed lockfiles when changes returned.
// Returns a restore function to revert lockfile writes if the subsequent build-tool command fails, and healed=true when at least one lockfile was updated.
func RunIfEnabled(ctx context.Context, client ComponentResolutionClient, repo string, tool BuildTool, command, workingDir string, runner CommandRunner, bootstrapArgs ...string) (restore func() error, healed bool, err error) {
	if IsComponentResolutionDisabled() || !IsRelevantCommand(tool, command) {
		log.Debug("Xray heal components disabled or not relevant command: ", command)
		return noopRestore, false, nil
	}
	log.Debug("Running Xray heal components at '"+repo+"' RT repository for tool:", tool.ToolName())
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
		resp, err := client.HealComponents(services.ComponentResolutionRequest{
			BuildTool: tool.ToolName(),
			Repo:      repo,
			Lockfile:  string(lf.Content),
		})
		if err != nil {
			return noopRestore, false, errorutils.CheckError(err)
		}
		if len(resp.Changes) == 0 {
			log.Debug("No changes for ", lf.Path)
			continue
		}
		toWrite = append(toWrite, Lockfile{Path: lf.Path, Content: []byte(resp.Lockfile)})
		totalChanges += len(resp.Changes)
		log.Debug("Healed", lf.Path, "with", len(resp.Changes), "package change(s)")
		for _, ch := range resp.Changes {
			log.Debug("  ", ch.Package, ":", ch.BeforeIntegrity, "→", ch.AfterIntegrity)
		}
	}
	if len(toWrite) == 0 {
		return noopRestore, false, nil
	}
	log.Debug("Applying", len(toWrite), "healed lockfile(s)...")
	restore, err = ApplyLockfiles(projectRoot, toWrite, bootstrapped)
	if err != nil {
		return noopRestore, false, err
	}
	log.Info("Xray component resolution healed", totalChanges, "package change(s) across", len(toWrite), "lockfile(s)")
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
	for _, lf := range lockfiles {
		fullPath := filepath.Join(projectRoot, lf.Path)
		backup := lockfileBackup{path: fullPath}
		if !absent[lf.Path] {
			data, readErr := os.ReadFile(fullPath)
			if readErr == nil {
				backup.content = data
			} else if !os.IsNotExist(readErr) {
				return nil, errorutils.CheckError(readErr)
			}
		}
		backups = append(backups, backup)

		if err = os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, errorutils.CheckError(err)
		}
		if err = os.WriteFile(fullPath, lf.Content, 0644); err != nil {
			return nil, errorutils.CheckError(err)
		}
	}

	return func() error {
		for _, backup := range backups {
			if backup.content == nil {
				if err = os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
					return errorutils.CheckError(err)
				}
				continue
			}
			if err = os.WriteFile(backup.path, backup.content, 0644); err != nil {
				return errorutils.CheckError(err)
			}
		}
		return nil
	}, nil
}
