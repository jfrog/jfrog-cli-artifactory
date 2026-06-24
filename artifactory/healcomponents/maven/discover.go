package maven

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const pomFileName = "pom.xml"

type discoveryOptions struct {
	pomFile     string
	projectList []string
}

func discoverProjectRoot(workingDir string) (string, error) {
	return discoverProjectRootWithOptions(workingDir, discoveryOptions{})
}

func discoverProjectRootWithOptions(workingDir string, opts discoveryOptions) (string, error) {
	startDir, err := effectiveStartDir(workingDir, opts.pomFile)
	if err != nil {
		return "", err
	}
	dir := startDir
	var firstPomDir string
	var topAggregator string
	for {
		pomPath := filepath.Join(dir, pomFileName)
		if _, statErr := os.Stat(pomPath); statErr == nil {
			if firstPomDir == "" {
				firstPomDir = dir
			}
			data, readErr := os.ReadFile(pomPath)
			if readErr == nil {
				pom, parseErr := parsePom(data)
				if parseErr == nil && pom.isAggregator() {
					topAggregator = dir
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if topAggregator != "" {
		return topAggregator, nil
	}
	if firstPomDir != "" {
		return firstPomDir, nil
	}
	return "", errorutils.CheckErrorf("no %s found from %s", pomFileName, startDir)
}

func effectiveStartDir(workingDir, pomFile string) (string, error) {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if pomFile == "" {
		return abs, nil
	}
	pomPath := pomFile
	if !filepath.IsAbs(pomPath) {
		pomPath = filepath.Join(abs, pomPath)
	}
	pomPath = filepath.Clean(pomPath)
	if _, statErr := os.Stat(pomPath); statErr != nil {
		return "", errorutils.CheckErrorf("%s not found: %s", pomFileName, pomPath)
	}
	return filepath.Dir(pomPath), nil
}

func discoverPomPaths(projectRoot string) ([]string, error) {
	return discoverPomPathsWithOptions(projectRoot, discoveryOptions{})
}

func discoverPomPathsWithOptions(projectRoot string, opts discoveryOptions) ([]string, error) {
	rootPom := filepath.Join(projectRoot, pomFileName)
	if _, err := os.Stat(rootPom); err != nil {
		return nil, errorutils.CheckErrorf("expected %s under %s", pomFileName, projectRoot)
	}
	all, err := collectReactorPoms(projectRoot, pomFileName)
	if err != nil {
		return nil, err
	}
	if len(opts.projectList) == 0 {
		return all, nil
	}
	return filterByProjectList(all, opts.projectList)
}

func collectReactorPoms(projectRoot, relPom string) ([]string, error) {
	full := filepath.Join(projectRoot, relPom)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	pom, err := parsePom(data)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	paths := []string{relPom}
	if !pom.isAggregator() {
		return paths, nil
	}
	baseDir := filepath.Dir(relPom)
	for _, mod := range pom.modulePaths() {
		mod = filepath.FromSlash(strings.TrimSpace(mod))
		if mod == "" {
			continue
		}
		childRel := filepath.Join(baseDir, mod, pomFileName)
		childRel = filepath.Clean(childRel)
		sub, err := collectReactorPoms(projectRoot, childRel)
		if err != nil {
			return nil, err
		}
		paths = append(paths, sub...)
	}
	return paths, nil
}

func filterByProjectList(relPoms []string, projectList []string) ([]string, error) {
	want := make(map[string]bool, len(projectList))
	for _, p := range projectList {
		want[filepath.Clean(filepath.FromSlash(p))] = true
	}
	var out []string
	for _, rel := range relPoms {
		modDir := filepath.Dir(rel)
		if modDir == "." {
			modDir = ""
		}
		if want[modDir] {
			out = append(out, rel)
		}
	}
	if len(out) == 0 {
		return nil, errorutils.CheckErrorf("no reactor modules matched -pl: %s", strings.Join(projectList, ","))
	}
	return out, nil
}
