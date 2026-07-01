package utils

import (
	"os"
	"path/filepath"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// ExtractDockerBuildContextFromArgs returns the docker build context directory (last non-flag arg).
// Falls back to "." when not found.
func ExtractDockerBuildContextFromArgs(args []string) (string, error) {
	var last string
	for _, arg := range args {
		if arg == "" || arg[0] == '-' {
			continue
		}
		last = arg
	}
	if last == "" {
		last = "."
	}
	abs, err := filepath.Abs(last)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	return abs, nil
}

// ResolveWorkingDirectoryFromDockerArgs prefers the docker build context directory for VCS lookup.
func ResolveWorkingDirectoryFromDockerArgs(cmdParams []string) (string, error) {
	if ctx, err := ExtractDockerBuildContextFromArgs(cmdParams); err == nil && ctx != "" {
		if info, statErr := os.Stat(ctx); statErr == nil && info.IsDir() {
			return ctx, nil
		}
	}
	return os.Getwd()
}
