package civcs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/spf13/viper"
)

type gitConfigReader interface {
	ReadConfig() error
	GetUrl() string
	GetRevision() string
	GetBranch() string
}

type UploadPatternOptions struct {
	IsRegexp bool
	IsAnt    bool
}

var (
	findDotGitFromDirFn = utils.GetDotGitFromDir
	newGitManagerFn     = func(dotGitPath string) gitConfigReader {
		return clientutils.NewGitManager(dotGitPath)
	}
)

func DeriveSearchDirFromUploadPattern(pattern string, opts UploadPatternOptions) string {
	if opts.IsRegexp {
		return "."
	}
	wildcardIdx := strings.IndexAny(pattern, "*?[")
	if wildcardIdx == -1 {
		if fileutils.IsPathExists(pattern, false) {
			if info, err := os.Stat(pattern); err == nil && info.IsDir() {
				return pattern
			}
		}
		dir := filepath.Dir(pattern)
		if dir == "." {
			return "."
		}
		return dir
	}
	prefix := strings.TrimRight(pattern[:wildcardIdx], "/\\")
	if prefix == "" {
		return "."
	}
	return prefix
}

func hasAllLocalGitProps(props string) bool {
	return hasProp(props, VcsUrlKey) && hasProp(props, VcsRevisionKey) && hasProp(props, VcsBranchKey)
}

func hasAllConfigGitProps(vConfig *viper.Viper) bool {
	return vConfig.IsSet(VcsUrlKey) && vConfig.IsSet(VcsRevisionKey) && vConfig.IsSet(VcsBranchKey)
}

func getLocalGitVcsInfo(searchDir string) (cienv.CIVcsInfo, error) {
	if IsCIVcsPropsDisabled() {
		return cienv.CIVcsInfo{}, nil
	}
	dotGitPath, err := findDotGitFromDirFn(searchDir)
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
