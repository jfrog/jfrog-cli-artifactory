package agentcommon

import "testing"

func TestAgentPluginsRepoOptions(t *testing.T) {
	opts := AgentPluginsRepoOptions()
	if opts.PackageType != "agentplugins" {
		t.Fatalf("expected package type agentplugins, got %s", opts.PackageType)
	}
	if opts.EnvVar != "JFROG_AGENT_PLUGINS_REPO" {
		t.Fatalf("expected env var JFROG_AGENT_PLUGINS_REPO, got %s", opts.EnvVar)
	}
	if opts.Label == "" {
		t.Fatal("expected non-empty label")
	}
}
