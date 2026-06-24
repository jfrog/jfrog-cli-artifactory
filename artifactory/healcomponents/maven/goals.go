package maven

import "strings"

type mavenCLIArgs struct {
	pomFile     string   // relative or absolute path from -f/--file
	projectList []string // from -pl/--projects (comma-separated segments already split)
}

func parseMavenCLIArgs(goals []string) (pomFile string, projectList []string) {
	a := parseMavenCLIArgsStruct(goals)
	return a.pomFile, a.projectList
}

func parseMavenCLIArgsStruct(goals []string) mavenCLIArgs {
	var out mavenCLIArgs
	for i := 0; i < len(goals); i++ {
		g := goals[i]
		switch {
		case g == "-f" || g == "--file":
			if i+1 < len(goals) {
				i++
				out.pomFile = goals[i]
			}
		case strings.HasPrefix(g, "-f"):
			out.pomFile = strings.TrimPrefix(g, "-f")
		case strings.HasPrefix(g, "--file="):
			out.pomFile = strings.TrimPrefix(g, "--file=")
		case g == "-pl" || g == "--projects":
			if i+1 < len(goals) {
				i++
				out.projectList = splitProjectList(goals[i])
			}
		case strings.HasPrefix(g, "-pl"):
			out.projectList = splitProjectList(strings.TrimPrefix(g, "-pl"))
		case strings.HasPrefix(g, "--projects="):
			out.projectList = splitProjectList(strings.TrimPrefix(g, "--projects="))
		}
	}
	return out
}

func splitProjectList(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

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
