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

// ValidateInstallFlags validates `--path | (--harness [, --project-dir | --global])` for install/update.
// absoluteInstallBaseDir is non-empty when --path was supplied; otherwise specs are resolved from --harness.
func ValidateInstallFlags(c *components.Context) (absoluteInstallBaseDir string, specs []AgentSpec, projectDirAbs string, isGlobal bool, err error) {
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

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	if err != nil {
		return
	}
	if rawHarness == "" {
		err = fmt.Errorf("--harness is required unless --path is set. Supported harnesses: %s", agentcommon.AgentNames(registry))
		return
	}

	harnessNames, err := ParseHarnessList(rawHarness)
	if err != nil {
		return
	}
	specs = make([]AgentSpec, 0, len(harnessNames))
	for _, name := range harnessNames {
		agentSpec, resolveErr := agentcommon.ResolveAgent(registry, name, RegistryHelp)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		specs = append(specs, agentSpec)
	}

	projectDirAbs, err = agentcommon.ResolveInstallProjectDir(projectDir, isGlobal)
	return
}

// ResolveAgentTargets resolves per-agent install destinations.
// When path is non-empty, a single ScopePath target is returned.
func ResolveAgentTargets(slug, path string, agents []AgentSpec, projectDirAbs string, isGlobal bool) ([]AgentTarget, error) {
	if path != "" {
		target, err := agentcommon.BuildPathInstallTarget(slug, path)
		if err != nil {
			return nil, err
		}
		return []AgentTarget{target}, nil
	}

	scope := ScopeProject
	if isGlobal {
		scope = ScopeGlobal
	}
	if scope == ScopeProject && projectDirAbs == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}

	targets := make([]AgentTarget, 0, len(agents))
	for _, agent := range agents {
		base, err := agentcommon.ResolveAgentInstallDir(agent, projectDirAbs, isGlobal)
		if err != nil {
			return nil, err
		}
		targets = append(targets, AgentTarget{
			Agent:          agent,
			Scope:          scope,
			DestinationDir: filepath.Join(base, slug),
		})
	}
	return targets, nil
}
