package civcs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/utils/cienv"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type gitConfigReader interface {
	ReadConfig() error
	GetUrl() string
	GetRevision() string
	GetBranch() string
}

var (
	findDotGitUpstreamFn = findDotGitUpstream
	newGitManagerFn      = func(dotGitPath string) gitConfigReader {
		return clientutils.NewGitManager(dotGitPath)
	}
)

func getLocalGitVcsInfo(sourcePattern string) (cienv.CIVcsInfo, error) {
	startDir := deriveSearchDirFromPattern(sourcePattern)
	dotGitPath, err := findDotGitUpstreamFn(startDir)
	if err != nil || dotGitPath == "" {
		return cienv.CIVcsInfo{}, err
	}

	gitManager := newGitManagerFn(dotGitPath)
	if err = gitManager.ReadConfig(); err != nil {
		return cienv.CIVcsInfo{}, err
	}

	return cienv.CIVcsInfo{
		Url:      gitManager.GetUrl(),
		Revision: gitManager.GetRevision(),
		Branch:   gitManager.GetBranch(),
	}, nil
}

func getLocalGitPropsFromSourcePattern(sourcePattern string) string {
	info, err := getLocalGitVcsInfo(sourcePattern)
	if err != nil {
		log.Debug("Skipping local git VCS props, failed getting VCS info:", err.Error())
		return ""
	}
	return BuildCIVcsPropsString(info)
}

func deriveSearchDirFromPattern(sourcePattern string) string {
	wildcardIdx := strings.IndexAny(sourcePattern, "*?[")
	if wildcardIdx == -1 {
		dir := filepath.Dir(sourcePattern)
		if dir == "." {
			return "."
		}
		return dir
	}

	prefix := strings.TrimRight(sourcePattern[:wildcardIdx], "/\\")
	if prefix == "" {
		return "."
	}
	return prefix
}

func findDotGitUpstream(startDir string) (string, error) {
	if startDir == "" || startDir == "." {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		dotGitPath := filepath.Join(absDir, ".git")
		if _, statErr := os.Stat(dotGitPath); statErr == nil {
			return dotGitPath, nil
		}

		parent := filepath.Dir(absDir)
		if parent == absDir {
			return "", nil
		}
		absDir = parent
	}
}
