package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// apmSubcommand is the apm subcommand this package always drives.
const apmSubcommand = "publish"

// ApmPublishCommand runs `apm publish` with JFrog Artifactory authentication and records the
// published package in build-info. Never accepts --repo; a registry must already be declared
// via jf setup agent-apm or apm.yml's own registries: block, which also supplies the repo name
// for build-info enrichment (see ResolveRepoNameFromRegistry).
type ApmPublishCommand struct {
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
}

func NewApmPublishCommand() *ApmPublishCommand {
	return &ApmPublishCommand{}
}

func (c *ApmPublishCommand) SetArgs(args []string) *ApmPublishCommand {
	c.args = args
	return c
}

func (c *ApmPublishCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApmPublishCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *ApmPublishCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *ApmPublishCommand {
	c.buildConfiguration = buildConfiguration
	return c
}

func (c *ApmPublishCommand) CommandName() string {
	return apmcommon.CommandNamePrefix + apmSubcommand
}

func (c *ApmPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// requirePackageFlag returns a clear, jf-level error if --package isn't present in args. It must
// be passed explicitly (not inferred from a bare positional argument), since a positional value
// could be mistaken for a value-taking apm flag's argument (e.g. "--zip foo.zip acme/pkg").
func requirePackageFlag(args []string) error {
	for _, arg := range args {
		if arg == "--package" || strings.HasPrefix(arg, "--package=") {
			return nil
		}
	}
	return fmt.Errorf("jf agent apm publish requires --package <owner>/<name>, e.g. --package acme/my-skill")
}

func (c *ApmPublishCommand) Run() error {
	log.Info("Running apm publish...")

	if err := requirePackageFlag(c.args); err != nil {
		return err
	}
	if err := apmcommon.RunApmSubcommandWithAuth(apmSubcommand, c.args, c.serverDetails); err != nil {
		return fmt.Errorf("run apm publish: %w", err)
	}

	if apmcommon.IsDryRunArg(c.args) {
		// --dry-run still packs the local zip but uploads nothing; skip build-info so the
		// local-zip checksum fallback doesn't record an artifact that was never published.
		log.Info("apm publish: --dry-run - nothing was uploaded, skipping build-info recording.")
	} else if workingDir, err := os.Getwd(); err != nil {
		log.Warn("apm publish completed, but could not determine working directory for build info:", err.Error())
	} else {
		manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
		owner := ownerFromArgs(c.args)
		packageName := packageNameFromArgs(c.args)
		artifactoryRepoKey := apmcommon.ResolveRepoNameFromRegistry(c.serverDetails, manifestPath, c.args)
		zipPath := zipPathFromArgs(c.args)
		if biErr := apmcommon.CollectAndSavePublishBuildInfo(manifestPath, owner, packageName, artifactoryRepoKey, zipPath, c.serverDetails, c.buildConfiguration); biErr != nil {
			log.Warn("apm publish completed, but build info recording failed:", biErr.Error())
		}
	}

	log.Info("apm publish finished successfully.")
	return nil
}

// packageSpecFromArgs extracts the raw "owner/name" value of --package from args.
// Returns "" if --package isn't present.
func packageSpecFromArgs(args []string) string {
	for i, arg := range args {
		if arg == "--package" && i+1 < len(args) {
			return args[i+1]
		}
		if cut, ok := strings.CutPrefix(arg, "--package="); ok {
			return cut
		}
	}
	return ""
}

// ownerFromArgs extracts the owner segment from a "--package owner/name" pair in args.
// Returns "" if --package isn't present or doesn't contain a "/".
func ownerFromArgs(args []string) string {
	owner, _, ok := strings.Cut(packageSpecFromArgs(args), "/")
	if !ok {
		return ""
	}
	return owner
}

// packageNameFromArgs extracts the name segment from a "--package owner/name" pair in args -
// the identifier apm actually uploads under (PUT /v1/packages/{owner}/{name}/versions/{version}),
// which is independent of apm.yml's own name: field. Returns "" if --package isn't present or
// doesn't contain a "/".
func packageNameFromArgs(args []string) string {
	_, name, ok := strings.Cut(packageSpecFromArgs(args), "/")
	if !ok {
		return ""
	}
	return name
}

// zipPathFromArgs extracts the value of --zip, the pre-built archive path apm publishes instead
// of auto-packing one. Returns "" if --zip isn't present.
func zipPathFromArgs(args []string) string {
	for i, arg := range args {
		if arg == "--zip" && i+1 < len(args) {
			return args[i+1]
		}
		if cut, ok := strings.CutPrefix(arg, "--zip="); ok {
			return cut
		}
	}
	return ""
}

// RunPublish is the CLI action handler for `jf agent apm publish`.
func RunPublish(c *components.Context) error {
	if apmcommon.IsHelpRequest(c.Arguments) {
		return apmcommon.RunApmCommand(nil, apmSubcommand, []string{apmcommon.HelpFlag})
	}

	opts, err := apmcommon.ExtractApmSubcommandOptions(c.Arguments)
	if err != nil {
		return err
	}
	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	cmd := NewApmPublishCommand().
		SetArgs(opts.RemainingArgs).
		SetServerDetails(serverDetails).
		SetBuildConfiguration(opts.BuildConfig)

	return commands.ExecWithPackageManager(cmd, apmcommon.PackageManagerID)
}
