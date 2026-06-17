package healcomponents

import (
	"context"
	"os"

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
		log.Debug("Healed ", lf.Path, " with ", len(resp.Changes), " package change(s)")
		for _, ch := range resp.Changes {
			log.Debug("  ", ch.Package, ": ", ch.BeforeIntegrity, " → ", ch.AfterIntegrity)
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
	log.Info("Xray component resolution healed ", totalChanges, " package change(s) across ", len(toWrite), " lockfile(s)")
	return restore, true, nil
}

func getLockfilePaths(lockfiles []Lockfile) []string {
	paths := make([]string, 0, len(lockfiles))
	for _, lf := range lockfiles {
		paths = append(paths, lf.Path)
	}
	return paths
}
