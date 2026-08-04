package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/gofrog/datastructures"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// dockerBuildValueFlags are docker build flags that consume the next argument.
var dockerBuildValueFlags = datastructures.MakeSetFromElements(
	"-t", "--tag",
	"-f", "--file",
	"--build-arg",
	"--label",
	"--target",
	"--platform",
	"-o", "--output",
	"--secret",
	"--ssh",
	"--cache-from",
	"--cache-to",
	"--iidfile",
	"--metadata-file",
	"--network",
	"--add-host",
	"--attest",
	"--call",
	"--progress",
	"-m", "--memory",
	"--cpuset-cpus",
	"--cgroup-parent",
)

// ExtractDockerBuildContextFromArgs returns the docker build context directory.
// Falls back to "." when not found.
func ExtractDockerBuildContextFromArgs(args []string) (string, error) {
	args = skipDockerBuildCommandPrefix(args)

	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		if arg[0] == '-' {
			flagName, _, hasEmbeddedValue := splitDockerFlag(arg)
			if hasEmbeddedValue {
				continue
			}
			if consumesNextDockerBuildArg(flagName) {
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	context := "."
	if len(positionals) > 0 {
		context = positionals[len(positionals)-1]
	}
	abs, err := filepath.Abs(context)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	return abs, nil
}

func skipDockerBuildCommandPrefix(args []string) []string {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "docker", "podman", "buildx":
			i++
		case "build":
			i++
		default:
			return args[i:]
		}
	}
	return nil
}

func splitDockerFlag(arg string) (name, value string, hasEmbeddedValue bool) {
	if idx := strings.IndexByte(arg, '='); idx > 0 {
		return arg[:idx], arg[idx+1:], true
	}
	return arg, "", false
}

func consumesNextDockerBuildArg(flagName string) bool {
	return dockerBuildValueFlags.Exists(flagName)
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
