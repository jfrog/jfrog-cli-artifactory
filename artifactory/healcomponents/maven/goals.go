package maven

import "strings"

func DeriveResolutionCommand(goals []string) string {
	if ShouldSkipResolution(goals) {
		return ""
	}
	for _, g := range goals {
		if isDependencyResolvingGoal(g) {
			return "resolve"
		}
	}
	return ""
}

func ShouldSkipResolution(goals []string) bool {
	if len(goals) == 0 {
		return true
	}
	if len(goals) == 1 && goals[0] == "clean" {
		return true
	}
	for _, g := range goals {
		lower := strings.ToLower(g)
		switch {
		case lower == "help", lower == "-v", lower == "-version", lower == "version":
			return true
		case lower == "dependency:tree", strings.HasSuffix(lower, ":tree"):
			return true
		}
	}
	return false
}

func isDependencyResolvingGoal(goal string) bool {
	lower := strings.ToLower(goal)
	phases := []string{"validate", "initialize", "generate-sources", "process-sources",
		"generate-resources", "process-resources", "compile", "process-classes",
		"generate-test-sources", "process-test-sources", "generate-test-resources",
		"process-test-resources", "test-compile", "process-test-classes", "test",
		"prepare-package", "package", "pre-integration-test", "integration-test",
		"post-integration-test", "verify", "install", "deploy"}
	for _, phase := range phases {
		if lower == phase || strings.HasSuffix(lower, ":"+phase) {
			return true
		}
	}
	return false
}
