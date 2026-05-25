package common

import "testing"

func TestRepoOptions(t *testing.T) {
	opts := RepoOptions()
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
