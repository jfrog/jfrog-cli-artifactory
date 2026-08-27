package npm

import (
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type discoveryOptions struct {
	prefixDir   string
	publishPath string
}

type npmCLIArgs struct {
	prefixDir     string
	bootstrapArgs []string
}

func parseNpmCLIArgs(args []string) npmCLIArgs {
	var out npmCLIArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--prefix" || arg == "--cwd" || arg == "-C":
			if i+1 < len(args) {
				i++
				out.prefixDir = args[i]
			}
		case strings.HasPrefix(arg, "--prefix="):
			out.prefixDir = strings.TrimPrefix(arg, "--prefix=")
		case strings.HasPrefix(arg, "--cwd="):
			out.prefixDir = strings.TrimPrefix(arg, "--cwd=")
		case strings.HasPrefix(arg, "-C"):
			if arg == "-C" {
				continue
			}
			out.prefixDir = strings.TrimPrefix(arg, "-C")
		case arg == "--workspaces" || arg == "-w":
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case strings.HasPrefix(arg, "--workspace="):
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case arg == "--workspace" && i+1 < len(args):
			out.bootstrapArgs = append(out.bootstrapArgs, arg, args[i+1])
			i++
		}
	}
	return out
}

// BootstrapArgsFrom extracts workspace flags to pass to npm install --package-lock-only.
func BootstrapArgsFrom(npmArgs []string) []string {
	return parseNpmCLIArgs(npmArgs).bootstrapArgs
}

func effectiveStartDir(workingDir string, opts discoveryOptions) (string, error) {
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	if opts.publishPath != "" {
		return resolveDiscoveryPath(abs, opts.publishPath)
	}
	if opts.prefixDir != "" {
		return resolveDiscoveryPath(abs, opts.prefixDir)
	}
	return abs, nil
}

// resolveDiscoveryPath joins base and p unless p is already absolute.
// On Windows, Unix-style paths (e.g. /repo/pkg) are not filepath.IsAbs but must not be joined with base.
func resolveDiscoveryPath(base, p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	if strings.HasPrefix(filepath.ToSlash(p), "/") {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", errorutils.CheckError(err)
		}
		return filepath.Clean(abs), nil
	}
	return filepath.Clean(filepath.Join(base, p)), nil
}
