package pnpm

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
