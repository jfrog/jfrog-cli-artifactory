package common

import agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"

const PathAgentName = agentcommon.PathAgentName

type ScopeMode = agentcommon.InstallScope

const (
	ScopeProject = agentcommon.InstallScopeProject
	ScopeGlobal  = agentcommon.InstallScopeGlobal
	ScopePath    = agentcommon.InstallScopePath
)

type AgentTarget = agentcommon.InstallTarget
