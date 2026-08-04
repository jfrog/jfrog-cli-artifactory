package pnpm

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/version"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// recordPnpmCommandMetadata saves the pnpm version and the full executed pnpm
// command as build-info properties, mirroring the docker.build.command pattern
// used by the docker build command (docker_build.go's getBiProperties). Takes
// the already-resolved pnpm version and build handle so it doesn't re-spawn
// `pnpm --version` or re-run build prerequisite prep, both already done by the
// caller's command validation and build-info setup.
func recordPnpmCommandMetadata(pnpmBuild *build.Build, pnpmVersion *version.Version, commandArgs []string) {
	env := buildCommandMetadataEnv(pnpmVersion, commandArgs)
	if err := pnpmBuild.SavePartialBuildInfo(&entities.Partial{Env: env}); err != nil {
		log.Warn("Failed to save pnpm command metadata:", err.Error())
	}
}

// buildCommandMetadataEnv builds the pnpm.command/pnpm.version build-info Env entries.
func buildCommandMetadataEnv(pnpmVersion *version.Version, commandArgs []string) entities.Env {
	env := entities.Env{"pnpm.command": "pnpm " + strings.Join(commandArgs, " ")}
	if pnpmVersion != nil {
		env["pnpm.version"] = pnpmVersion.GetVersion()
	}
	return env
}

const (
	minSupportedPnpmVersion = "10.0.0"
	pnpm11Version           = "11.0.0"
	// minRequiredNodeVersion applies to pnpm 10.x. pnpm 11 dropped support for Node
	// versions below 22.13 (pure ESM), so pnpm >= 11 requires minRequiredNodeVersionForPnpm11 instead.
	minRequiredNodeVersion          = "18.12.0"
	minRequiredNodeVersionForPnpm11 = "22.13.0"
)

// NewCommand creates a pnpm command by subcommand name with common fields set.
func NewCommand(cmdName string, args []string, buildConfig *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails) (commands.Command, error) {
	pnpmVer, err := validatePnpmPrerequisites()
	if err != nil {
		return nil, err
	}
	switch cmdName {
	case "install", "i":
		return NewPnpmInstallCommand().SetArgs(args).SetBuildConfiguration(buildConfig).SetServerDetails(serverDetails).SetPnpmVersion(pnpmVer), nil
	case "publish":
		return NewPnpmPublishCommand().SetArgs(args).SetBuildConfiguration(buildConfig).SetServerDetails(serverDetails).SetPnpmVersion(pnpmVer), nil
	default:
		return nil, fmt.Errorf("unsupported pnpm command: %s", cmdName)
	}
}

// validatePnpmPrerequisites checks that pnpm and Node.js meet the version requirements,
// returning the resolved pnpm version so callers don't need to re-query it.
func validatePnpmPrerequisites() (*version.Version, error) {
	pnpmVer, err := getPnpmVersion()
	if err != nil {
		return nil, err
	}
	if pnpmVer.Compare(minSupportedPnpmVersion) > 0 {
		return nil, errorutils.CheckErrorf(
			"JFrog CLI pnpm commands require pnpm version %s or higher. Current version: %s", minSupportedPnpmVersion, pnpmVer.GetVersion())
	}
	log.Debug("pnpm version:", pnpmVer.GetVersion())

	nodeVer, err := getNodeJSVersion()
	if err != nil {
		return nil, err
	}
	requiredNodeVersion := minRequiredNodeVersion
	if pnpmVer.Compare(pnpm11Version) <= 0 {
		requiredNodeVersion = minRequiredNodeVersionForPnpm11
	}
	if nodeVer.Compare(requiredNodeVersion) > 0 {
		return nil, errorutils.CheckErrorf(
			"pnpm %s requires Node.js version %s or higher. Current version: %s", pnpmVer.GetVersion(), requiredNodeVersion, nodeVer.GetVersion())
	}
	log.Debug("Node.js version:", nodeVer.GetVersion())
	return pnpmVer, nil
}

// getPnpmVersion returns the installed pnpm version.
func getPnpmVersion() (*version.Version, error) {
	output, err := exec.Command("pnpm", "--version").Output()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to determine pnpm version. Ensure pnpm is installed: %w", err)
	}
	return version.NewVersion(strings.TrimSpace(string(output))), nil
}

// getNodeJSVersion returns the installed Node.js version.
func getNodeJSVersion() (*version.Version, error) {
	output, err := exec.Command("node", "--version").Output()
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to determine Node.js version. Ensure Node.js is installed: %w", err)
	}
	// node --version returns "vX.Y.Z", strip the leading "v"
	return version.NewVersion(strings.TrimPrefix(strings.TrimSpace(string(output)), "v")), nil
}

type moduleInfo struct {
	id           string
	dependencies []entities.Dependency
	rawDeps      []depInfo
}

type depInfo struct {
	name        string
	version     string
	resolvedURL string
	scopes      []string
	requestedBy [][]string
}

type tarballParts struct {
	repo     string
	dirPath  string
	fileName string
}

type parsedDep struct {
	dep   depInfo
	parts tarballParts
}

type aqlBatch struct {
	repo string
	deps []parsedDep
}

func parseTarballURL(tarballURL string) (tarballParts, error) {
	u, err := url.Parse(tarballURL)
	if err != nil {
		return tarballParts{}, fmt.Errorf("invalid tarball URL %q: %w", tarballURL, err)
	}

	path := strings.TrimPrefix(u.Path, "/")

	const apiNpmPrefix = "api/npm/"
	if idx := strings.Index(path, apiNpmPrefix); idx != -1 {
		path = path[idx+len(apiNpmPrefix):]
	}

	slashIdx := strings.Index(path, "/")
	if slashIdx == -1 {
		return tarballParts{}, fmt.Errorf("cannot extract repo from path %q", path)
	}
	repo := path[:slashIdx]
	rest := path[slashIdx+1:]

	dashIdx := strings.Index(rest, "/-/")
	if dashIdx == -1 {
		return tarballParts{}, fmt.Errorf("cannot find /-/ separator in %q", rest)
	}

	dirPath := rest[:dashIdx] + "/-"
	fileName := rest[dashIdx+3:]

	return tarballParts{
		repo:     repo,
		dirPath:  dirPath,
		fileName: fileName,
	}, nil
}

func buildTarballPartsFromName(name, version string) tarballParts {
	var dirPath, fileName string
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			dirPath = name + "/-"
			fileName = parts[1] + "-" + version + ".tgz"
		}
	} else {
		dirPath = name + "/-"
		fileName = name + "-" + version + ".tgz"
	}
	return tarballParts{dirPath: dirPath, fileName: fileName}
}
