package healcomponents

import (
	"context"
	"slices"
)

// CommandRunner runs a build-tool subprocess (injected for tests).
type CommandRunner func(ctx context.Context, projectRoot string, args ...string) error

// BuildTool describes a package manager for the generic resolution flow.
type BuildTool interface {
	ToolName() string
	RelevantCommands() []string
	// ProjectRoot resolves monorepo/reactor/solution root from workingDir.
	ProjectRoot(workingDir string) (string, error)
	// EnsureLockfiles materializes expected lock artifacts when absent and the command allows it.
	// Returns paths (relative to project root) that were bootstrapped and did not exist before.
	EnsureLockfiles(ctx context.Context, projectRoot, command string, runner CommandRunner, bootstrapArgs ...string) (bootstrapped []string, err error)
	// DiscoverLockfiles returns all lockfiles (paths relative to project root).
	DiscoverLockfiles(workingDir string) ([]Lockfile, error)
}

func IsRelevantCommand(tool BuildTool, command string) bool {
	return slices.Contains(tool.RelevantCommands(), command)
}
