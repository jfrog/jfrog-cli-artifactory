package common

import (
	"fmt"
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
func ValidateInstallFlags(c *components.Context) (agentcommon.InstallFlagsResult, error) {
	pathInstallBase := strings.TrimSpace(c.GetStringFlagValue("path"))
	rawHarness := strings.TrimSpace(c.GetStringFlagValue("harness"))
	isGlobal := c.GetBoolFlagValue("global")
	projectDir := strings.TrimSpace(c.GetStringFlagValue("project-dir"))

	absoluteInstallBaseDir, err := agentcommon.ResolvePathInstallBase(agentcommon.InstallFlagInput{
		PathInstallBase: pathInstallBase,
		RawHarness:      rawHarness,
		ProjectDir:      projectDir,
		IsGlobal:        isGlobal,
	})
	if err != nil {
		return agentcommon.InstallFlagsResult{}, err
	}
	if absoluteInstallBaseDir != "" {
		return agentcommon.InstallFlagsResult{AbsoluteInstallBaseDir: absoluteInstallBaseDir}, nil
	}

	registry, err := agentcommon.LoadAgentRegistry(Agents, agentcommon.SkillsAgentsKey)
	if err != nil {
		return agentcommon.InstallFlagsResult{}, err
	}
	if rawHarness == "" {
		return agentcommon.InstallFlagsResult{}, fmt.Errorf("--harness is required unless --path is set. Supported harnesses: %s", agentcommon.AgentNames(registry))
	}

	harnessNames, err := agentcommon.ParseHarnessList(rawHarness)
	if err != nil {
		return agentcommon.InstallFlagsResult{}, err
	}
	specs := make([]AgentSpec, 0, len(harnessNames))
	for _, name := range harnessNames {
		agentSpec, resolveErr := agentcommon.ResolveAgent(registry, name, RegistryHelp)
		if resolveErr != nil {
			return agentcommon.InstallFlagsResult{}, resolveErr
		}
		specs = append(specs, agentSpec)
	}

	projectDirAbs, err := agentcommon.ResolveInstallProjectDir(projectDir, isGlobal)
	if err != nil {
		return agentcommon.InstallFlagsResult{}, err
	}
	return agentcommon.InstallFlagsResult{
		Specs:         specs,
		ProjectDirAbs: projectDirAbs,
		IsGlobal:      isGlobal,
	}, nil
}
