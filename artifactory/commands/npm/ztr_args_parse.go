package npm

import (
	"strings"
)

type npmCLIArgs struct {
	prefixDir       string
	registryURL     string
	bootstrapArgs   []string
	packageOperands []string
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
		case strings.HasPrefix(arg, "-C") && arg != "-C":
			out.prefixDir = strings.TrimPrefix(arg, "-C")
		case arg == "--workspaces":
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case arg == "--workspace" || arg == "-w":
			if i+1 < len(args) {
				i++
				out.bootstrapArgs = append(out.bootstrapArgs, arg, args[i])
			}
		case strings.HasPrefix(arg, "--workspace="):
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case strings.HasPrefix(arg, "-w=") && len(arg) > 3:
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case arg == "--registry":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				out.registryURL = args[i]
				out.bootstrapArgs = append(out.bootstrapArgs, arg, args[i])
			}
		case strings.HasPrefix(arg, "--registry="):
			out.registryURL = strings.TrimPrefix(arg, "--registry=")
			out.bootstrapArgs = append(out.bootstrapArgs, arg)
		case arg == "--tag" || arg == "--omit" || arg == "--include" || arg == "--save-prefix":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case strings.HasPrefix(arg, "-"):
			continue
		default:
			out.packageOperands = append(out.packageOperands, arg)
		}
	}
	return out
}

func stripNpmInstallOnlyArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		drop, consumesValue := npmInstallOnlyArg(args[i])
		if !drop {
			filtered = append(filtered, args[i])
			continue
		}
		if i+1 < len(args) {
			next := args[i+1]
			if (consumesValue && !strings.HasPrefix(next, "-")) ||
				(npmInstallOnlyBooleanArg(args[i]) && (next == "true" || next == "false")) {
				i++
			}
		}
	}
	return filtered
}

func npmInstallOnlyArg(arg string) (drop, consumesValue bool) {
	switch arg {
	case "--save", "-S",
		"--save-prod", "-P",
		"--save-dev", "-D",
		"--save-optional", "-O",
		"--save-peer", "--save-bundle",
		"--no-save",
		"--save-exact", "-E",
		"--package-lock-only",
		"--package-lock", "--no-package-lock":
		return true, false
	case "--save-prefix":
		return true, true
	}
	name, _, hasValue := strings.Cut(arg, "=")
	if !hasValue {
		return false, false
	}
	switch name {
	case "--save",
		"--save-prod",
		"--save-dev",
		"--save-optional",
		"--save-peer",
		"--save-bundle",
		"--save-exact",
		"--save-prefix",
		"--package-lock-only",
		"--package-lock",
		"--no-package-lock":
		return true, false
	}
	return false, false
}

func npmInstallOnlyBooleanArg(arg string) bool {
	switch arg {
	case "--package-lock-only", "--package-lock", "--no-package-lock":
		return true
	default:
		return false
	}
}
