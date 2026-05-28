package common

import (
	"fmt"
	"path/filepath"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const PathAgentName = agentcommon.PathAgentName

type ScopeMode = agentcommon.InstallScope

const (
	ScopeProject = agentcommon.InstallScopeProject
	ScopeGlobal  = agentcommon.InstallScopeGlobal
	ScopePath    = agentcommon.InstallScopePath
)

type AgentTarget = agentcommon.InstallTarget

// ValidateInstallFlags validates `--path | (--harness [, --project-dir | --global])` for plugins install.
// absoluteInstallBaseDir is non-empty when --path was supplied; otherwise spec is resolved from --harness.
// Unlike skills install, --harness accepts exactly one agent name.
func ValidateInstallFlags(c *components.Context) (absoluteInstallBaseDir string, spec AgentSpec, projectDirAbs string, isGlobal bool, err error) {
	pathInstallBase := strings.TrimSpace(c.GetStringFlagValue("path"))
	rawHarness := strings.TrimSpace(c.GetStringFlagValue("harness"))
	isGlobal = c.GetBoolFlagValue("global")
	projectDir := strings.TrimSpace(c.GetStringFlagValue("project-dir"))

	absoluteInstallBaseDir, err = agentcommon.ResolvePathInstallBase(agentcommon.InstallFlagInput{
		PathInstallBase: pathInstallBase,
		RawHarness:      rawHarness,
		ProjectDir:      projectDir,
		IsGlobal:        isGlobal,
	})
	if err != nil || absoluteInstallBaseDir != "" {
		return
	}

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.PluginsAgentsKey)
	if err != nil {
		return
	}
	if rawHarness == "" {
		err = fmt.Errorf("--harness is required unless --path is set. Supported harnesses: %s", agentcommon.AgentNames(registry))
		return
	}

	harnessName, err := ParseSingleHarness(rawHarness)
	if err != nil {
		return
	}
	spec, err = agentcommon.ResolveAgent(registry, harnessName, RegistryHelp)
	if err != nil {
		return
	}

	projectDirAbs, err = agentcommon.ResolveInstallProjectDir(projectDir, isGlobal)
	return
}

// ResolveAgentTarget returns the single install destination for a plugin.
// When path is non-empty, a ScopePath target is returned; otherwise the
// agent's project/global directory is used.
func ResolveAgentTarget(slug, path string, spec AgentSpec, projectDirAbs string, isGlobal bool) (AgentTarget, error) {
	if path != "" {
		return agentcommon.BuildPathInstallTarget(slug, path)
	}

	scope := ScopeProject
	if isGlobal {
		scope = ScopeGlobal
	}
	if scope == ScopeProject && projectDirAbs == "" {
		return AgentTarget{}, fmt.Errorf("project directory is required for project-scoped install")
	}

	base, err := agentcommon.ResolveAgentInstallDir(spec, projectDirAbs, isGlobal)
	if err != nil {
		return AgentTarget{}, err
	}
	return AgentTarget{
		Agent:          spec,
		Scope:          scope,
		DestinationDir: filepath.Join(base, slug),
	}, nil
}
