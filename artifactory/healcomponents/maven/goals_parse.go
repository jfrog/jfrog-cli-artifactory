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
