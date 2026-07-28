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

// ApmPublishCommand runs `apm publish` with JFrog Artifactory authentication and records the
// published package in build-info.
//
// Unlike passthrough commands, publish never accepts --repo: no other package-manager
// integration in this CLI supports declaring a new repository at run time either - they all
// require the one-time `jf setup <tool>` step first. A registry must already be declared
// (via jf setup agent-apm or apm.yml's own registries: block) before publish can authenticate
// against it; the repo name used for build-info enrichment is derived from that same
// declaration (see ResolveRepoNameFromRegistry).
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
	return "rt_agent_apm_publish"
}

func (c *ApmPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

// withPackageFlag promotes a bare positional package spec (e.g. "jfrog/proj3") into the
// --package flag apm publish requires. If --package is already present, args are left untouched.
func withPackageFlag(args []string) []string {
	for _, arg := range args {
		if arg == "--package" || strings.HasPrefix(arg, "--package=") {
			return args
		}
	}
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			rest := make([]string, 0, len(args)-1)
			rest = append(rest, args[:i]...)
			rest = append(rest, args[i+1:]...)
			return append([]string{"--package", arg}, rest...)
		}
	}
	return args
}

func (c *ApmPublishCommand) Run() error {
	log.Info("Running apm publish...")

	args := withPackageFlag(c.args)
	if err := apmcommon.RunApmSubcommandWithAuth("publish", args, c.serverDetails); err != nil {
		return fmt.Errorf("run apm publish: %w", err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Warn("apm publish completed, but could not determine working directory for build info:", err.Error())
	} else {
		manifestPath := filepath.Join(workingDir, apmcommon.ApmManifestName)
		owner := ownerFromArgs(args)
		repoName := apmcommon.ResolveRepoNameFromRegistry(c.serverDetails, manifestPath)
		if biErr := apmcommon.CollectAndSavePublishBuildInfo(manifestPath, owner, repoName, c.serverDetails, c.buildConfiguration); biErr != nil {
			log.Warn("apm publish completed, but build info recording failed:", biErr.Error())
		}
	}

	log.Info("apm publish finished successfully.")
	return nil
}

// ownerFromArgs extracts the owner segment from a "--package owner/name" pair in args.
// Returns "" if --package isn't present or doesn't contain a "/".
func ownerFromArgs(args []string) string {
	for i, arg := range args {
		var pkg string
		if arg == "--package" && i+1 < len(args) {
			pkg = args[i+1]
		} else if cut, ok := strings.CutPrefix(arg, "--package="); ok {
			pkg = cut
		}
		if pkg == "" {
			continue
		}
		if owner, _, ok := strings.Cut(pkg, "/"); ok {
			return owner
		}
	}
	return ""
}

// RunPublish is the CLI action handler for `jf agent apm publish`.
func RunPublish(c *components.Context) error {
	if apmcommon.IsHelpRequest(c.Arguments) {
		return apmcommon.RunApmCommand(nil, "publish", []string{"--help"})
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

	return commands.ExecWithPackageManager(cmd, "agent-apm")
}
