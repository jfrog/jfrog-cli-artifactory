package pnpm

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/version"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	minSupportedPnpmVersion = "10.0.0"
	minUnsupportedPnpmVersion = "11.0.0"
	minRequiredNodeVersion  = "18.12.0"
)

// pnpmCommand is the common interface for pnpm subcommands (install, publish).
type pnpmCommand interface {
	commands.Command
	SetArgs([]string) pnpmCommand
	SetBuildConfiguration(*buildUtils.BuildConfiguration) pnpmCommand
	SetServerDetails(*config.ServerDetails) pnpmCommand
}

// pnpmInstallAdapter wraps PnpmInstallCommand to satisfy pnpmCommand.
type pnpmInstallAdapter struct{ *PnpmInstallCommand }

func (a *pnpmInstallAdapter) SetArgs(args []string) pnpmCommand {
	a.PnpmInstallCommand.SetArgs(args)
	return a
}
func (a *pnpmInstallAdapter) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) pnpmCommand {
	a.PnpmInstallCommand.SetBuildConfiguration(bc)
	return a
}
func (a *pnpmInstallAdapter) SetServerDetails(sd *config.ServerDetails) pnpmCommand {
	a.PnpmInstallCommand.SetServerDetails(sd)
	return a
}

// pnpmPublishAdapter wraps PnpmPublishCommand to satisfy pnpmCommand.
type pnpmPublishAdapter struct{ *PnpmPublishCommand }

func (a *pnpmPublishAdapter) SetArgs(args []string) pnpmCommand {
	a.PnpmPublishCommand.SetArgs(args)
	return a
}
func (a *pnpmPublishAdapter) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) pnpmCommand {
	a.PnpmPublishCommand.SetBuildConfiguration(bc)
	return a
}
func (a *pnpmPublishAdapter) SetServerDetails(sd *config.ServerDetails) pnpmCommand {
	a.PnpmPublishCommand.SetServerDetails(sd)
	return a
}

// NewCommand creates a pnpm command by subcommand name with common fields set.
func NewCommand(cmdName string, args []string, buildConfig *buildUtils.BuildConfiguration, serverDetails *config.ServerDetails) (commands.Command, error) {
	if err := validatePnpmPrerequisites(); err != nil {
		return nil, err
	}
	var cmd pnpmCommand
	switch cmdName {
	case "install", "i":
		cmd = &pnpmInstallAdapter{NewPnpmInstallCommand()}
	case "publish", "p":
		cmd = &pnpmPublishAdapter{NewPnpmPublishCommand()}
	default:
		return nil, fmt.Errorf("unsupported pnpm command: %s", cmdName)
	}
	cmd.SetArgs(args).SetBuildConfiguration(buildConfig).SetServerDetails(serverDetails)
	return cmd, nil
}

// validatePnpmPrerequisites checks that pnpm and Node.js meet the version requirements.
// Currently only pnpm 10.x is supported.
func validatePnpmPrerequisites() error {
	pnpmVer, err := getPnpmVersion()
	if err != nil {
		return err
	}
	if pnpmVer.Compare(minSupportedPnpmVersion) > 0 {
		return errorutils.CheckErrorf(
			"JFrog CLI pnpm commands require pnpm version %s or higher. Current version: %s", minSupportedPnpmVersion, pnpmVer.GetVersion())
	}
	if pnpmVer.Compare(minUnsupportedPnpmVersion) <= 0 {
		return errorutils.CheckErrorf(
			"JFrog CLI pnpm commands currently support pnpm 10.x only. Current version: %s", pnpmVer.GetVersion())
	}
	log.Debug("pnpm version:", pnpmVer.GetVersion())

	nodeVer, err := getNodeJSVersion()
	if err != nil {
		return err
	}
	if nodeVer.Compare(minRequiredNodeVersion) > 0 {
		return errorutils.CheckErrorf(
			"pnpm 10 requires Node.js version %s or higher. Current version: %s", minRequiredNodeVersion, nodeVer.GetVersion())
	}
	log.Debug("Node.js version:", nodeVer.GetVersion())
	return nil
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
