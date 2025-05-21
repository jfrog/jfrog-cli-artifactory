package sonarqube

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ReportInfo struct {
	Tool string
	Path string
	Time time.Time
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func DetectBuildToolAndBestReport(basePath string) (string, string) {
	var reports []ReportInfo

	// Potential tools and paths
	candidates := []ReportInfo{
		{"maven", filepath.Join(basePath, "target/sonar/report-task.txt"), time.Time{}},
		{"gradle", filepath.Join(basePath, "build/sonar/report-task.txt"), time.Time{}},
		{"cli", filepath.Join(basePath, ".scannerwork/report-task.txt"), time.Time{}},
		{"msbuild", filepath.Join(basePath, ".sonarqube/out/.sonar/report-task.txt"), time.Time{}},
	}

	for i, c := range candidates {
		if fileExists(c.Path) {
			candidates[i].Time = fileModTime(c.Path)
			reports = append(reports, candidates[i])
			fmt.Printf("Found report for %s at %s (modified: %s)\n", c.Tool, c.Path, candidates[i].Time)
		}
	}

	if len(reports) == 0 {
		fmt.Println("No report-task.txt found. Falling back to CLI default.")
		return "cli", filepath.Join(basePath, ".scannerwork/report-task.txt")
	}

	// Find the most recent report
	best := reports[0]
	for _, r := range reports {
		if r.Time.After(best.Time) {
			best = r
		}
	}

	fmt.Printf("Selected latest report from %s\n", best.Tool)
	return best.Tool, best.Path
}

//
//func main() {
//	tool, path := detectBuildToolAndBestReport(".")
//	fmt.Printf("Using: %s -> %s\n", tool, path)
//}

func detectCIPlatform() string {
	switch {
	case os.Getenv("GITHUB_ACTIONS") == "true":
		return "github"
	case os.Getenv("GITLAB_CI") == "true":
		return "gitlab"
	case os.Getenv("JENKINS_URL") != "":
		return "jenkins"
	case os.Getenv("BITBUCKET_BUILD_NUMBER") != "":
		return "bitbucket"
	case os.Getenv("CIRCLECI") == "true":
		return "circleci"
	case os.Getenv("TF_BUILD") == "true":
		return "azure"
	case os.Getenv("TRAVIS") == "true":
		return "travis"
	case os.Getenv("TEAMCITY_VERSION") != "":
		return "teamcity"
	default:
		return "unknown"
	}
}
