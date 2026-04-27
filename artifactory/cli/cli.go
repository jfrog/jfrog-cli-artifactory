package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/buildinfo"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/formats"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/curl"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/oc"
	containerutils "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/replication"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildadddependencies"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildaddgit"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildappend"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildclean"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildcollectenv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/builddiscard"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/builddockercreate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildpromote"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildpublish"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/buildscan"
	copydocs "github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/copy"
	curldocs "github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/curl"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/delete"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/deleteprops"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/directdownload"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/dockerpromote"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/dockerpull"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/dockerpush"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/download"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/gitlfsclean"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/move"
	nugettree "github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/nugetdepstree"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/ocstartbuild"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/ping"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/podmanpull"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/podmanpush"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/replicationcreate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/replicationdelete"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/replicationtemplate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/repocreate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/repodelete"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/repotemplate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/repoupdate"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/search"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/setprops"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/docs/upload"
	artifactoryUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/commandWrappers"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	coregeneric "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/generic"
	commandUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/cliutils/summary"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	coreformat "github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/progressbar"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	buildinfocmd "github.com/jfrog/jfrog-client-go/artifactory/buildinfo"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/pkg/errors"
)

const (
	filesCategory    = "Files Management"
	buildCategory    = "Build Info"
	repoCategory     = "Repository Management"
	replicCategory   = "Replication"
	otherCategory    = "Other"
	releaseBundlesV2 = "release-bundles-v2"
)

func GetCommands() []components.Command {
	commands := []components.Command{
		{
			Name:        "upload",
			Flags:       flagkit.GetCommandFlags(flagkit.Upload),
			Aliases:     []string{"u"},
			Description: upload.GetDescription(),
			Arguments:   upload.GetArguments(),
			Action:      uploadCmd,
			Category:    filesCategory,
		},
		{
			Name:        "download",
			Flags:       flagkit.GetCommandFlags(flagkit.Download),
			Aliases:     []string{"dl"},
			Description: download.GetDescription(),
			Arguments:   download.GetArguments(),
			Action:      downloadCmd,
			Category:    filesCategory,
		},
		{
			Name:        "direct-download",
			Flags:       flagkit.GetCommandFlags(flagkit.DirectDownload),
			Aliases:     []string{"ddl"},
			Description: directdownload.GetDescription(),
			Arguments:   directdownload.GetArguments(),
			Action:      directDownloadCmd,
			Category:    filesCategory,
		},
		{
			Name:        "move",
			Flags:       flagkit.GetCommandFlags(flagkit.Move),
			Aliases:     []string{"mv"},
			Description: move.GetDescription(),
			Arguments:   move.GetArguments(),
			Action:      moveCmd,
			Category:    filesCategory,
		},
		{
			Name:        "copy",
			Flags:       flagkit.GetCommandFlags(flagkit.Copy),
			Aliases:     []string{"cp"},
			Description: copydocs.GetDescription(),
			Arguments:   copydocs.GetArguments(),
			Action:      copyCmd,
			Category:    filesCategory,
		},
		{
			Name:        "delete",
			Flags:       flagkit.GetCommandFlags(flagkit.Delete),
			Aliases:     []string{"del"},
			Description: delete.GetDescription(),
			Arguments:   delete.GetArguments(),
			Action:      deleteCmd,
			Category:    filesCategory,
		},
		{
			Name:        "search",
			Flags:       flagkit.GetCommandFlags(flagkit.Search),
			Aliases:     []string{"s"},
			Description: search.GetDescription(),
			Arguments:   search.GetArguments(),
			Action:      searchCmd,
			Category:    filesCategory,
		},
		{
			Name:        "set-props",
			Flags:       flagkit.GetCommandFlags(flagkit.Properties),
			Aliases:     []string{"sp"},
			Description: setprops.GetDescription(),
			Arguments:   setprops.GetArguments(),
			Action:      setPropsCmd,
			Category:    filesCategory,
		},
		{
			Name:        "delete-props",
			Flags:       flagkit.GetCommandFlags(flagkit.Properties),
			Aliases:     []string{"delp"},
			Description: deleteprops.GetDescription(),
			Arguments:   deleteprops.GetArguments(),
			Action:      deletePropsCmd,
			Category:    filesCategory,
		},
		{
			Name:        "build-publish",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildPublish),
			Aliases:     []string{"bp"},
			Description: buildpublish.GetDescription(),
			Arguments:   buildpublish.GetArguments(),
			Action:      buildPublishCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-collect-env",
			Aliases:     []string{"bce"},
			Flags:       flagkit.GetCommandFlags(flagkit.BuildCollectEnv),
			Description: buildcollectenv.GetDescription(),
			Arguments:   buildcollectenv.GetArguments(),
			Action:      buildCollectEnvCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-append",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildAppend),
			Aliases:     []string{"ba"},
			Description: buildappend.GetDescription(),
			Arguments:   buildappend.GetArguments(),
			Action:      buildAppendCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-add-dependencies",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildAddDependencies),
			Aliases:     []string{"bad"},
			Description: buildadddependencies.GetDescription(),
			Arguments:   buildadddependencies.GetArguments(),
			Action:      buildAddDependenciesCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-add-git",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildAddGit),
			Aliases:     []string{"bag"},
			Description: buildaddgit.GetDescription(),
			Arguments:   buildaddgit.GetArguments(),
			Action:      buildAddGitCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-scan",
			Hidden:      true,
			Flags:       flagkit.GetCommandFlags(flagkit.BuildScanLegacy),
			Aliases:     []string{"bs"},
			Description: buildscan.GetDescription(),
			Arguments:   buildscan.GetArguments(),
			Action: func(c *components.Context) error {
				return commandWrappers.DeprecationCmdWarningWrapper("build-scan", "rt", c, buildScanLegacyCmd)
			},
		},
		{
			Name:        "build-clean",
			Aliases:     []string{"bc"},
			Description: buildclean.GetDescription(),
			Arguments:   buildclean.GetArguments(),
			Action:      buildCleanCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-promote",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildPromote),
			Aliases:     []string{"bpr"},
			Description: buildpromote.GetDescription(),
			Arguments:   buildpromote.GetArguments(),
			Action:      buildPromoteCmd,
			Category:    buildCategory,
		},
		{
			Name:        "build-discard",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildDiscard),
			Aliases:     []string{"bdi"},
			Description: builddiscard.GetDescription(),
			Arguments:   builddiscard.GetArguments(),
			Action:      buildDiscardCmd,
			Category:    buildCategory,
		},
		{
			Name:        "git-lfs-clean",
			Flags:       flagkit.GetCommandFlags(flagkit.GitLfsClean),
			Aliases:     []string{"glc"},
			Description: gitlfsclean.GetDescription(),
			Arguments:   gitlfsclean.GetArguments(),
			Action:      gitLfsCleanCmd,
			Category:    otherCategory,
		},
		{
			Name:        "docker-promote",
			Flags:       flagkit.GetCommandFlags(flagkit.DockerPromote),
			Aliases:     []string{"dpr"},
			Description: dockerpromote.GetDescription(),
			Arguments:   dockerpromote.GetArguments(),
			Action:      dockerPromoteCmd,
			Category:    buildCategory,
		},
		{
			Name:        "docker-push",
			Hidden:      true,
			Flags:       flagkit.GetCommandFlags(flagkit.ContainerPush),
			Aliases:     []string{"dp"},
			Description: dockerpush.GetDescription(),
			Arguments:   dockerpush.GetArguments(),
			Action: func(c *components.Context) error {
				return containerPushCmd(c, containerutils.DockerClient)
			},
		},
		{
			Name:        "docker-pull",
			Hidden:      true,
			Flags:       flagkit.GetCommandFlags(flagkit.ContainerPull),
			Aliases:     []string{"dpl"},
			Description: dockerpull.GetDescription(),
			Arguments:   dockerpull.GetArguments(),
			Action: func(c *components.Context) error {
				return containerPullCmd(c, containerutils.DockerClient)
			},
		},
		{
			Name:        "podman-push",
			Flags:       flagkit.GetCommandFlags(flagkit.ContainerPush),
			Aliases:     []string{"pp"},
			Description: podmanpush.GetDescription(),
			Arguments:   podmanpush.GetArguments(),
			Action: func(c *components.Context) error {
				return containerPushCmd(c, containerutils.Podman)
			},
			Category: otherCategory,
		},
		{
			Name:        "podman-pull",
			Flags:       flagkit.GetCommandFlags(flagkit.ContainerPull),
			Aliases:     []string{"ppl"},
			Description: podmanpull.GetDescription(),
			Arguments:   podmanpull.GetArguments(),
			Action: func(c *components.Context) error {
				return containerPullCmd(c, containerutils.Podman)
			},
			Category: otherCategory,
		},
		{
			Name:        "build-docker-create",
			Flags:       flagkit.GetCommandFlags(flagkit.BuildDockerCreate),
			Aliases:     []string{"bdc"},
			Description: builddockercreate.GetDescription(),
			Arguments:   builddockercreate.GetArguments(),
			Action:      BuildDockerCreateCmd,
			Category:    buildCategory,
		},
		{
			Name:            "oc", // Only 'oc start-build' is supported
			Flags:           flagkit.GetCommandFlags(flagkit.OcStartBuild),
			Aliases:         []string{"osb"},
			Description:     ocstartbuild.GetDescription(),
			SkipFlagParsing: true,
			Action:          ocStartBuildCmd,
			Category:        otherCategory,
		},
		{
			Name:        "nuget-deps-tree",
			Aliases:     []string{"ndt"},
			Description: nugettree.GetDescription(),
			Action:      nugetDepsTreeCmd,
			Category:    otherCategory,
		},
		{
			Name:        "ping",
			Flags:       flagkit.GetCommandFlags(flagkit.Ping),
			Aliases:     []string{"p"},
			Description: ping.GetDescription(),
			Action:      pingCmd,
		},
		{
			Name:            "curl",
			Flags:           flagkit.GetCommandFlags(flagkit.RtCurl),
			Aliases:         []string{"cl"},
			Description:     curldocs.GetDescription(),
			Arguments:       curldocs.GetArguments(),
			SkipFlagParsing: true,
			Action:          curlCmd,
		},
		{
			Name:        "repo-template",
			Aliases:     []string{"rpt"},
			Description: repotemplate.GetDescription(),
			Arguments:   repotemplate.GetArguments(),
			Action:      repoTemplateCmd,
			Category:    repoCategory,
		},
		{
			Name:        "repo-create",
			Aliases:     []string{"rc"},
			Flags:       flagkit.GetCommandFlags(flagkit.TemplateConsumer),
			Description: repocreate.GetDescription(),
			Arguments:   repocreate.GetArguments(),
			Action:      repoCreateCmd,
			Category:    repoCategory,
		},
		{
			Name:        "repo-update",
			Aliases:     []string{"ru"},
			Flags:       flagkit.GetCommandFlags(flagkit.TemplateConsumer),
			Description: repoupdate.GetDescription(),
			Arguments:   repoupdate.GetArguments(),
			Action:      repoUpdateCmd,
			Category:    repoCategory,
		},
		{
			Name:        "repo-delete",
			Aliases:     []string{"rdel"},
			Flags:       flagkit.GetCommandFlags(flagkit.RepoDelete),
			Description: repodelete.GetDescription(),
			Arguments:   repodelete.GetArguments(),
			Action:      repoDeleteCmd,
			Category:    repoCategory,
		},
		{
			Name:        "replication-template",
			Aliases:     []string{"rplt"},
			Flags:       flagkit.GetCommandFlags(flagkit.TemplateConsumer),
			Description: replicationtemplate.GetDescription(),
			Arguments:   replicationtemplate.GetArguments(),
			Action:      replicationTemplateCmd,
			Category:    replicCategory,
		},
		{
			Name:        "replication-create",
			Aliases:     []string{"rplc"},
			Flags:       flagkit.GetCommandFlags(flagkit.TemplateConsumer),
			Description: replicationcreate.GetDescription(),
			Arguments:   replicationcreate.GetArguments(),
			Action:      replicationCreateCmd,
			Category:    replicCategory,
		},
		{
			Name:        "replication-delete",
			Aliases:     []string{"rpldel"},
			Flags:       flagkit.GetCommandFlags(flagkit.ReplicationDelete),
			Description: replicationdelete.GetDescription(),
			Arguments:   replicationdelete.GetArguments(),
			Action:      replicationDeleteCmd,
			Category:    replicCategory,
		},
	}

	return commands
}

func getRetries(c *components.Context) (retries int, err error) {
	retries = flagkit.Retries
	if c.GetStringFlagValue("retries") != "" {
		retries, err = strconv.Atoi(c.GetStringFlagValue("retries"))
		if err != nil {
			err = errors.New("The '--retries' option should have a numeric value. " + common.GetDocumentationMessage())
			return 0, err
		}
	}

	return retries, nil
}

// getRetryWaitTime extract the given '--retry-wait-time' value and validate that it has a numeric value and a 's'/'ms' suffix.
// The returned wait time's value is in milliseconds.
func getRetryWaitTime(c *components.Context) (waitMilliSecs int, err error) {
	waitMilliSecs = flagkit.RetryWaitMilliSecs
	waitTimeStringValue := c.GetStringFlagValue("retry-wait-time")
	useSeconds := false
	if waitTimeStringValue != "" {
		switch {
		case strings.HasSuffix(waitTimeStringValue, "ms"):
			waitTimeStringValue = strings.TrimSuffix(waitTimeStringValue, "ms")

		case strings.HasSuffix(waitTimeStringValue, "s"):
			useSeconds = true
			waitTimeStringValue = strings.TrimSuffix(waitTimeStringValue, "s")
		default:
			err = getRetryWaitTimeVerificationError()
			return
		}
		waitMilliSecs, err = strconv.Atoi(waitTimeStringValue)
		if err != nil {
			err = getRetryWaitTimeVerificationError()
			return
		}
		// Convert seconds to milliseconds
		if useSeconds {
			waitMilliSecs *= 1000
		}
	}
	return
}

func getRetryWaitTimeVerificationError() error {
	return errorutils.CheckError(errors.New("The '--retry-wait-time' option should have a numeric value with 's'/'ms' suffix. " + common.GetDocumentationMessage()))
}

func dockerPromoteCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 3 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	params := services.NewDockerPromoteParams(c.GetArgumentAt(0), c.GetArgumentAt(1), c.GetArgumentAt(2))
	params.TargetDockerImage = c.GetStringFlagValue("target-docker-image")
	params.SourceTag = c.GetStringFlagValue("source-tag")
	params.TargetTag = c.GetStringFlagValue("target-tag")
	params.Copy = c.GetBoolFlagValue("copy")
	dockerPromoteCommand := container.NewDockerPromoteCommand()
	dockerPromoteCommand.SetParams(params).SetServerDetails(artDetails)

	return commands.Exec(dockerPromoteCommand)
}

func containerPushCmd(c *components.Context, containerManagerType containerutils.ContainerManagerType) (err error) {
	if c.GetNumberOfArgs() != 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return
	}
	imageTag := c.GetArgumentAt(0)
	targetRepo := c.GetArgumentAt(1)
	skipLogin := c.GetBoolFlagValue("skip-login")
	validateSha := c.GetBoolFlagValue("validate-sha")

	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return
	}
	dockerPushCommand := container.NewPushCommand(containerManagerType)
	threads, err := common.GetThreadsCount(c)
	if err != nil {
		return
	}
	outputFormat, err := getContainerPushOutputFormat(c)
	if err != nil {
		return
	}
	printDeploymentView, detailedSummary := log.IsStdErrTerminal(), c.GetBoolFlagValue("detailed-summary")
	// When a structured format is requested we need the per-layer transfer details reader,
	// so force detailed-summary mode regardless of the explicit flag.
	needDetailedReader := outputFormat != coreformat.None
	dockerPushCommand.SetThreads(threads).SetDetailedSummary(detailedSummary || printDeploymentView || needDetailedReader).SetCmdParams([]string{"push", imageTag}).SetSkipLogin(skipLogin).SetBuildConfiguration(buildConfiguration).SetRepo(targetRepo).SetServerDetails(artDetails).SetImageTag(imageTag).SetValidateSha(validateSha)
	err = commandWrappers.ShowDockerDeprecationMessageIfNeeded(containerManagerType, dockerPushCommand.IsGetRepoSupported)
	if err != nil {
		return
	}
	err = commands.Exec(dockerPushCommand)
	result := dockerPushCommand.Result()

	// Cleanup.
	defer common.CleanupResult(result, &err)
	if outputFormat == coreformat.None {
		err = common.PrintCommandSummary(dockerPushCommand.Result(), detailedSummary, printDeploymentView, false, err)
		return
	}
	err = printContainerPushResponse(result, outputFormat, os.Stdout, err)
	return
}

// getContainerPushOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (deployment view / JSON summary output via PrintCommandSummary).
func getContainerPushOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printContainerPushResponse renders the container push result in the requested output format.
func printContainerPushResponse(result *commandUtils.Result, outputFormat coreformat.OutputFormat, w io.Writer, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		return common.PrintCommandSummary(result, true, false, false, originalErr)
	case coreformat.Table:
		err := printContainerPushTable(result, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, result.SuccessCount(), result.FailCount(), false)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt podman-push. Acceptable values are: json, table", outputFormat)
	}
}

// containerPushTableRow is a table-printable representation of a pushed layer.
type containerPushTableRow struct {
	Target string `col-name:"TARGET"`
	Sha256 string `col-name:"SHA256"`
}

// printContainerPushTable renders pushed layers as a human-readable table.
func printContainerPushTable(result *commandUtils.Result, w io.Writer) error {
	reader := result.Reader()
	if reader == nil {
		// No per-layer details available (e.g. no build-info collection and no detailed-summary).
		// Fall back to a minimal counts table.
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
		_, _ = fmt.Fprintf(tw, "success\t%d\n", result.SuccessCount())
		_, _ = fmt.Fprintf(tw, "failure\t%d\n", result.FailCount())
		return tw.Flush()
	}
	var rows []containerPushTableRow
	for item := new(clientutils.FileTransferDetails); reader.NextRecord(item) == nil; item = new(clientutils.FileTransferDetails) {
		rows = append(rows, containerPushTableRow{
			Target: item.RtUrl + item.TargetPath,
			Sha256: item.Sha256,
		})
	}
	if err := reader.GetError(); err != nil {
		return err
	}
	reader.Reset()
	return coreutils.PrintTable(rows, "Push Results", "No layers were pushed.", false)
}

func containerPullCmd(c *components.Context, containerManagerType containerutils.ContainerManagerType) error {
	if c.GetNumberOfArgs() != 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	imageTag := c.GetArgumentAt(0)
	sourceRepo := c.GetArgumentAt(1)
	skipLogin := c.GetBoolFlagValue("skip-login")
	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return err
	}
	dockerPullCommand := container.NewPullCommand(containerManagerType)
	dockerPullCommand.SetCmdParams([]string{"pull", imageTag}).SetSkipLogin(skipLogin).SetImageTag(imageTag).SetRepo(sourceRepo).SetServerDetails(artDetails).SetBuildConfiguration(buildConfiguration)
	err = commandWrappers.ShowDockerDeprecationMessageIfNeeded(containerManagerType, dockerPullCommand.IsGetRepoSupported)
	if err != nil {
		return err
	}
	return commands.Exec(dockerPullCommand)
}

func BuildDockerCreateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	sourceRepo := c.GetArgumentAt(0)
	imageNameWithDigestFile := c.GetStringFlagValue("image-file")
	if imageNameWithDigestFile == "" {
		return common.PrintHelpAndReturnError("The '--image-file' command option was not provided.", c)
	}
	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return err
	}
	buildDockerCreateCommand := container.NewBuildDockerCreateCommand()
	if err = buildDockerCreateCommand.SetImageNameWithDigest(imageNameWithDigestFile); err != nil {
		return err
	}
	buildDockerCreateCommand.SetRepo(sourceRepo).SetServerDetails(artDetails).SetBuildConfiguration(buildConfiguration)
	return commands.Exec(buildDockerCreateCommand)
}

func ocStartBuildCmd(c *components.Context) error {
	args := common.ExtractCommand(c)

	// After the 'oc' command, only 'start-build' is allowed
	parentArgs := c.GetParent().Arguments
	if parentArgs[0] == "oc" {
		if len(parentArgs) < 2 || parentArgs[1] != "start-build" {
			return errorutils.CheckErrorf("invalid command. The only OpenShift CLI command supported by JFrog CLI is 'oc start-build'")
		}
		coreutils.RemoveFlagFromCommand(&args, 0, 0)
	}

	if show, err := common.ShowCmdHelpIfNeeded(c, args); show || err != nil {
		return err
	}
	if len(args) < 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	// Extract build configuration
	filteredOcArgs, buildConfiguration, err := build.ExtractBuildDetailsFromArgs(args)
	if err != nil {
		return err
	}

	// Extract repo
	flagIndex, valueIndex, repo, err := coreutils.FindFlag("--repo", filteredOcArgs)
	if err != nil {
		return err
	}
	coreutils.RemoveFlagFromCommand(&filteredOcArgs, flagIndex, valueIndex)
	if flagIndex == -1 {
		err = errorutils.CheckErrorf("the --repo option is mandatory")
		return err
	}

	// Extract server-id
	flagIndex, valueIndex, serverId, err := coreutils.FindFlag("--server-id", filteredOcArgs)
	if err != nil {
		return err
	}
	coreutils.RemoveFlagFromCommand(&filteredOcArgs, flagIndex, valueIndex)

	ocCmd := oc.NewOcStartBuildCommand().SetOcArgs(filteredOcArgs).SetRepo(repo).SetServerId(serverId).SetBuildConfiguration(buildConfiguration)
	return commands.Exec(ocCmd)
}

func nugetDepsTreeCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 0 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	return dotnet.DependencyTreeCmd()
}

func pingCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 0 {
		return common.PrintHelpAndReturnError("No arguments should be sent.", c)
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	cmd := coregeneric.NewPingCommand()
	cmd.SetServerDetails(artDetails)
	err = commands.Exec(cmd)
	resBody := cmd.Response()
	resString := clientutils.IndentJson(resBody)
	if err != nil {
		return errors.New(err.Error() + "\n" + resString)
	}
	outputFormat, fmtErr := getPingOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}
	return printPingResponse(resBody, outputFormat, os.Stdout)
}

// getPingOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (plain-text "OK" output).
func getPingOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printPingResponse renders the raw ping body in the requested output format.
func printPingResponse(body []byte, outputFormat coreformat.OutputFormat, w io.Writer) error {
	switch outputFormat {
	case coreformat.None:
		// Backward-compatible: print the raw (or indented) response as before.
		log.Output(clientutils.IndentJson(body))
		return nil
	case coreformat.Json:
		return printPingJSON(body)
	case coreformat.Table:
		return printPingTable(body, w)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt ping. Acceptable values are: json, table", outputFormat)
	}
}

// pingResponse is the structured representation emitted for --format json / table.
type pingResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
}

// pingResponseFromBody builds a pingResponse from the raw HTTP body.
// The body is expected to be plain text (e.g. "OK"). A 200 status is assumed
// because error responses are handled before this function is called.
func pingResponseFromBody(body []byte) pingResponse {
	msg := http.StatusText(http.StatusOK)
	if len(body) > 0 {
		msg = strings.TrimSpace(string(body))
	}
	return pingResponse{StatusCode: http.StatusOK, Message: msg}
}

// printPingJSON emits the ping result as indented JSON.
func printPingJSON(body []byte) error {
	resp := pingResponseFromBody(body)
	data, err := json.Marshal(resp)
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printPingTable renders the ping result as a two-column tabwriter table.
func printPingTable(body []byte, w io.Writer) error {
	resp := pingResponseFromBody(body)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "status_code\t%d\n", resp.StatusCode)
	_, _ = fmt.Fprintf(tw, "message\t%s\n", resp.Message)
	return tw.Flush()
}

func prepareDownloadCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 1 && c.GetNumberOfArgs() != 2 && (c.GetNumberOfArgs() != 0 || (!c.IsFlagSet("spec") && !c.IsFlagSet("build") && !c.IsFlagSet("bundle"))) {
		return nil, common.WrongNumberOfArgumentsHandler(c)
	}

	var downloadSpec *spec.SpecFiles
	var err error

	if c.IsFlagSet("spec") {
		downloadSpec, err = commonCliUtils.GetSpec(c, true, true)
	} else {
		downloadSpec, err = createDefaultDownloadSpec(c)
	}

	if err != nil {
		return nil, err
	}

	setTransitiveInDownloadSpec(downloadSpec)
	err = spec.ValidateSpec(downloadSpec.Files, false, true)
	if err != nil {
		return nil, err
	}
	return downloadSpec, nil
}

func prepareDirectDownloadCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 1 && c.GetNumberOfArgs() != 2 && (c.GetNumberOfArgs() != 0 || (!c.IsFlagSet("spec") && !c.IsFlagSet("build"))) {
		return nil, common.PrintHelpAndReturnError("Wrong number of arguments. Expected: <source-pattern> [target-path] OR --spec=<spec-file> OR --build=<build-name>/<build-number>", c)
	}

	var (
		downloadSpec *spec.SpecFiles
		err          error
	)

	if c.IsFlagSet("spec") {
		downloadSpec, err = commonCliUtils.GetSpec(c, true, true)
	} else {
		downloadSpec = createDirectDownloadSpec(c)
	}

	if err != nil {
		return nil, err
	}

	setTransitiveInDownloadSpec(downloadSpec)
	err = spec.ValidateSpec(downloadSpec.Files, false, true)
	if err != nil {
		return nil, err
	}
	return downloadSpec, nil
}

func createDirectDownloadSpec(c *components.Context) *spec.SpecFiles {
	excludeArtifactsString := c.GetStringFlagValue("exclude-artifacts")
	excludeArtifacts, err := parseStringToBool(excludeArtifactsString)
	if err != nil {
		log.Warn("Could not parse exclude-artifacts flag. Setting exclude-artifacts as false, error: ", err.Error())
	}

	includeDepsString := c.GetStringFlagValue("include-deps")
	includeDeps, err := parseStringToBool(includeDepsString)
	if err != nil {
		log.Warn("Could not parse include-deps flag. Setting include-deps as false, error: ", err.Error())
	}

	return spec.NewBuilder().
		Pattern(getSourcePattern(c)).
		Build(c.GetStringFlagValue("build")).
		Bundle(c.GetStringFlagValue("bundle")).
		ExcludeArtifacts(excludeArtifacts).
		IncludeDeps(includeDeps).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		Flat(c.GetBoolFlagValue("flat")).
		Explode(strconv.FormatBool(c.GetBoolFlagValue("explode"))).
		Target(c.GetArgumentAt(1)).
		BuildSpec()
}

func parseStringToBool(value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}

	return boolValue, nil
}

func directDownloadCmd(c *components.Context) error {
	downloadSpec, err := prepareDirectDownloadCommand(c)
	if err != nil {
		return err
	}
	fixWinPathsForDownloadCmd(downloadSpec, c)
	configuration, err := artifactoryUtils.CreateDownloadConfiguration(c)
	if err != nil {
		return err
	}
	serverDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}

	directDownloadCommand := generic.NewDirectDownloadCommand()
	directDownloadCommand.SetConfiguration(configuration).SetBuildConfiguration(buildConfiguration).SetSpec(downloadSpec).SetServerDetails(serverDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetSyncDeletesPath(c.GetStringFlagValue("sync-deletes")).SetQuiet(common.GetQuietValue(c)).SetDetailedSummary(c.GetBoolFlagValue("detailed-summary")).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	if directDownloadCommand.ShouldPrompt() && !coreutils.AskYesNo("Sync-deletes may delete some files in your local file system. Are you sure you want to continue?\n"+
		"You can avoid this confirmation message by adding --quiet to the command.", false) {
		return nil
	}

	err = progressbar.ExecWithProgress(directDownloadCommand)
	result := directDownloadCommand.Result()
	defer common.CleanupResult(result, &err)
	basicSummary, err := common.CreateSummaryReportString(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	if err != nil {
		return err
	}
	err = common.PrintDetailedSummaryReport(basicSummary, result.Reader(), false, err)
	return common.GetCliError(err, result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c))
}

func downloadCmd(c *components.Context) (err error) {
	downloadSpec, err := prepareDownloadCommand(c)
	if err != nil {
		return err
	}

	fixWinPathsForDownloadCmd(downloadSpec, c)
	configuration, err := artifactoryUtils.CreateDownloadConfiguration(c)
	if err != nil {
		return err
	}
	serverDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	outputFormat, err := getDownloadOutputFormat(c)
	if err != nil {
		return err
	}
	detailedSummary := common.GetDetailedSummary(c)
	// When a structured format is requested we need the per-file transfer details reader,
	// so force detailed-summary mode regardless of the explicit flag.
	needDetailedReader := outputFormat != coreformat.None
	downloadCommand := generic.NewDownloadCommand()
	downloadCommand.SetConfiguration(configuration).SetBuildConfiguration(buildConfiguration).SetSpec(downloadSpec).SetServerDetails(serverDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetSyncDeletesPath(c.GetStringFlagValue("sync-deletes")).SetQuiet(common.GetQuietValue(c)).SetDetailedSummary(detailedSummary || needDetailedReader).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	if downloadCommand.ShouldPrompt() && !coreutils.AskYesNo("Sync-deletes may delete some files in your local file system. Are you sure you want to continue?\n"+
		"You can avoid this confirmation message by adding --quiet to the command.", false) {
		return nil
	}
	// This error is being checked later on because we need to generate summary report before return.
	err = progressbar.ExecWithProgress(downloadCommand)
	result := downloadCommand.Result()
	defer common.CleanupResult(result, &err)
	if outputFormat == coreformat.None {
		basicSummary, sErr := common.CreateSummaryReportString(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
		if sErr != nil {
			return sErr
		}
		err = common.PrintDetailedSummaryReport(basicSummary, result.Reader(), false, err)
		return common.GetCliError(err, result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c))
	}
	err = printDownloadResponse(result, outputFormat, os.Stdout, common.IsFailNoOp(c), err)
	return
}

// getDownloadOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (JSON summary output via PrintDetailedSummaryReport).
func getDownloadOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printDownloadResponse renders the download result in the requested output format.
// It preserves the fail-no-op and error-accounting semantics of PrintDetailedSummaryReport.
func printDownloadResponse(result *commandUtils.Result, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		basicSummary, err := common.CreateSummaryReportString(result.SuccessCount(), result.FailCount(), failNoOp, originalErr)
		if err != nil {
			return err
		}
		return common.PrintDetailedSummaryReport(basicSummary, result.Reader(), false, originalErr)
	case coreformat.Table:
		err := printDownloadTable(result, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, result.SuccessCount(), result.FailCount(), failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt download. Acceptable values are: json, table", outputFormat)
	}
}

// downloadTableRow is a table-printable representation of a downloaded file.
type downloadTableRow struct {
	Source string `col-name:"SOURCE"`
	Target string `col-name:"TARGET"`
	Sha256 string `col-name:"SHA256"`
}

// printDownloadTable renders downloaded files as a human-readable table.
func printDownloadTable(result *commandUtils.Result, w io.Writer) error {
	reader := result.Reader()
	if reader == nil {
		// No per-file details available (e.g. dry-run without build-info collection).
		// Fall back to a minimal counts table.
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
		_, _ = fmt.Fprintf(tw, "success\t%d\n", result.SuccessCount())
		_, _ = fmt.Fprintf(tw, "failure\t%d\n", result.FailCount())
		return tw.Flush()
	}
	var rows []downloadTableRow
	for item := new(clientutils.FileTransferDetails); reader.NextRecord(item) == nil; item = new(clientutils.FileTransferDetails) {
		rows = append(rows, downloadTableRow{
			Source: item.RtUrl + item.SourcePath,
			Target: item.TargetPath,
			Sha256: item.Sha256,
		})
	}
	if err := reader.GetError(); err != nil {
		return err
	}
	reader.Reset()
	return coreutils.PrintTable(rows, "Download Results", "No files were downloaded.", false)
}

func checkRbExistenceInV2(c *components.Context) (bool, error) {
	bundleNameAndVersion := c.GetStringFlagValue("bundle")
	parts := strings.Split(bundleNameAndVersion, "/")
	rbName := parts[0]
	rbVersion := parts[1]

	lcDetails, err := createLifecycleDetailsByFlags(c)
	if err != nil {
		return false, err
	}

	lcServicesManager, err := utils.CreateLifecycleServiceManager(lcDetails, false)
	if err != nil {
		return false, err
	}

	return lcServicesManager.IsReleaseBundleExist(rbName, rbVersion, c.GetStringFlagValue("project"))
}

func createLifecycleDetailsByFlags(c *components.Context) (*config.ServerDetails, error) {
	lcDetails, err := common.CreateServerDetailsWithConfigOffer(c, true, commonCliUtils.Platform)
	if err != nil {
		return nil, err
	}
	if lcDetails.Url == "" {
		return nil, errors.New("platform URL is mandatory for lifecycle commands")
	}
	PlatformToLifecycleUrls(lcDetails)
	return lcDetails, nil
}

func PlatformToLifecycleUrls(lcDetails *config.ServerDetails) {
	// For tests only. in prod - this "if" will always return false
	if strings.Contains(lcDetails.Url, "artifactory/") {
		lcDetails.ArtifactoryUrl = clientutils.AddTrailingSlashIfNeeded(lcDetails.Url)
		lcDetails.LifecycleUrl = strings.Replace(
			clientutils.AddTrailingSlashIfNeeded(lcDetails.Url),
			"artifactory/",
			"lifecycle/",
			1,
		)
	} else {
		lcDetails.ArtifactoryUrl = clientutils.AddTrailingSlashIfNeeded(lcDetails.Url) + "artifactory/"
		lcDetails.LifecycleUrl = clientutils.AddTrailingSlashIfNeeded(lcDetails.Url) + "lifecycle/"
	}
	lcDetails.Url = ""
}

func uploadCmd(c *components.Context) (err error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 2 && (c.GetNumberOfArgs() != 0 || !c.IsFlagSet("spec")) {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	var uploadSpec *spec.SpecFiles
	if c.IsFlagSet("spec") {
		uploadSpec, err = commonCliUtils.GetSpec(c, false, true)
	} else {
		uploadSpec, err = createDefaultUploadSpec(c)
	}
	if err != nil {
		return
	}
	err = spec.ValidateSpec(uploadSpec.Files, true, false)
	if err != nil {
		return
	}
	common.FixWinPathsForFileSystemSourcedCmds(uploadSpec, c)
	configuration, err := artifactoryUtils.CreateUploadConfiguration(c)
	if err != nil {
		return
	}
	buildConfiguration, err := common.CreateBuildConfigurationWithModule(c)
	if err != nil {
		return
	}
	retries, err := getRetries(c)
	if err != nil {
		return
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return
	}
	outputFormat, err := getUploadOutputFormat(c)
	if err != nil {
		return
	}
	uploadCmd := generic.NewUploadCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return
	}
	printDeploymentView, detailedSummary := log.IsStdErrTerminal(), common.GetDetailedSummary(c)
	// When a structured format is requested we need the per-file transfer details reader,
	// so force detailed-summary mode regardless of the explicit flag.
	needDetailedReader := outputFormat != coreformat.None
	uploadCmd.SetUploadConfiguration(configuration).SetBuildConfiguration(buildConfiguration).SetSpec(uploadSpec).SetServerDetails(rtDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetSyncDeletesPath(c.GetStringFlagValue("sync-deletes")).SetQuiet(common.GetQuietValue(c)).SetDetailedSummary(detailedSummary || printDeploymentView || needDetailedReader).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	if uploadCmd.ShouldPrompt() && !coreutils.AskYesNo("Sync-deletes may delete some artifacts in Artifactory. Are you sure you want to continue?\n"+
		"You can avoid this confirmation message by adding --quiet to the command.", false) {
		return nil
	}
	// This error is being checked later on because we need to generate summary report before return.
	err = progressbar.ExecWithProgress(uploadCmd)
	result := uploadCmd.Result()
	defer common.CleanupResult(result, &err)
	if outputFormat == coreformat.None {
		err = common.PrintCommandSummary(uploadCmd.Result(), detailedSummary, printDeploymentView, common.IsFailNoOp(c), err)
		return
	}
	err = printUploadResponse(result, outputFormat, os.Stdout, common.IsFailNoOp(c), err)
	return
}

// getUploadOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (JSON summary output via PrintCommandSummary).
func getUploadOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printUploadResponse renders the upload result in the requested output format.
// It preserves the fail-no-op and error-accounting semantics of PrintCommandSummary.
func printUploadResponse(result *commandUtils.Result, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		return common.PrintCommandSummary(result, true, false, failNoOp, originalErr)
	case coreformat.Table:
		err := printUploadTable(result, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, result.SuccessCount(), result.FailCount(), failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt upload. Acceptable values are: json, table", outputFormat)
	}
}

// uploadTableRow is a table-printable representation of an uploaded file.
type uploadTableRow struct {
	Source string `col-name:"SOURCE"`
	Target string `col-name:"TARGET"`
	Sha256 string `col-name:"SHA256"`
}

// printUploadTable renders uploaded files as a human-readable table.
func printUploadTable(result *commandUtils.Result, w io.Writer) error {
	reader := result.Reader()
	if reader == nil {
		// No per-file details available (e.g. dry-run without build-info collection).
		// Fall back to a minimal counts table.
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
		_, _ = fmt.Fprintf(tw, "success\t%d\n", result.SuccessCount())
		_, _ = fmt.Fprintf(tw, "failure\t%d\n", result.FailCount())
		return tw.Flush()
	}
	var rows []uploadTableRow
	for item := new(clientutils.FileTransferDetails); reader.NextRecord(item) == nil; item = new(clientutils.FileTransferDetails) {
		rows = append(rows, uploadTableRow{
			Source: item.SourcePath,
			Target: item.RtUrl + item.TargetPath,
			Sha256: item.Sha256,
		})
	}
	if err := reader.GetError(); err != nil {
		return err
	}
	reader.Reset()
	return coreutils.PrintTable(rows, "Upload Results", "No files were uploaded.", false)
}

func prepareCopyMoveCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 2 && (c.GetNumberOfArgs() != 0 || !c.IsFlagSet("spec")) {
		return nil, common.WrongNumberOfArgumentsHandler(c)
	}

	var copyMoveSpec *spec.SpecFiles
	var err error
	if c.IsFlagSet("spec") {
		copyMoveSpec, err = commonCliUtils.GetSpec(c, false, true)
	} else {
		copyMoveSpec, err = createDefaultCopyMoveSpec(c)
	}
	if err != nil {
		return nil, err
	}
	err = spec.ValidateSpec(copyMoveSpec.Files, true, true)
	if err != nil {
		return nil, err
	}
	return copyMoveSpec, nil
}

func moveCmd(c *components.Context) error {
	moveSpec, err := prepareCopyMoveCommand(c)
	if err != nil {
		return err
	}
	mvCmd := generic.NewMoveCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	threads, err := common.GetThreadsCount(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	mvCmd.SetThreads(threads).SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails).SetSpec(moveSpec).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(mvCmd)
	result := mvCmd.Result()

	outputFormat, fmtErr := getMoveOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}
	if outputFormat == coreformat.None {
		return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	}
	return printMoveResponse(result.SuccessCount(), result.FailCount(), outputFormat, os.Stdout, common.IsFailNoOp(c), err)
}

// getMoveOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (brief summary output via printBriefSummaryAndGetError).
func getMoveOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printMoveResponse renders the move result in the requested output format.
func printMoveResponse(succeeded, failed int, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		err := printMoveJSON(succeeded, failed, failNoOp, originalErr)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	case coreformat.Table:
		err := printMoveTable(succeeded, failed, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt move. Acceptable values are: json, table", outputFormat)
	}
}

// printMoveJSON emits a JSON summary of the move operation to stdout via log.Output.
func printMoveJSON(succeeded, failed int, failNoOp bool, originalErr error) error {
	summaryReport := summary.GetSummaryReport(succeeded, failed, failNoOp, originalErr)
	data, err := summaryReport.Marshal()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printMoveTable renders a counts table for the move operation.
func printMoveTable(succeeded, failed int, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "success\t%d\n", succeeded)
	_, _ = fmt.Fprintf(tw, "failure\t%d\n", failed)
	return tw.Flush()
}

func copyCmd(c *components.Context) error {
	copySpec, err := prepareCopyMoveCommand(c)
	if err != nil {
		return err
	}

	copyCommand := generic.NewCopyCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	threads, err := common.GetThreadsCount(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	copyCommand.SetThreads(threads).SetSpec(copySpec).SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(copyCommand)
	result := copyCommand.Result()

	outputFormat, fmtErr := getCopyOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}
	if outputFormat == coreformat.None {
		return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	}
	return printCopyResponse(result.SuccessCount(), result.FailCount(), outputFormat, os.Stdout, common.IsFailNoOp(c), err)
}

// getCopyOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (brief summary output via printBriefSummaryAndGetError).
func getCopyOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printCopyResponse renders the copy result in the requested output format.
func printCopyResponse(succeeded, failed int, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		err := printCopyJSON(succeeded, failed, failNoOp, originalErr)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	case coreformat.Table:
		err := printCopyTable(succeeded, failed, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt copy. Acceptable values are: json, table", outputFormat)
	}
}

// printCopyJSON emits a JSON summary of the copy operation to stdout via log.Output.
func printCopyJSON(succeeded, failed int, failNoOp bool, originalErr error) error {
	summaryReport := summary.GetSummaryReport(succeeded, failed, failNoOp, originalErr)
	data, err := summaryReport.Marshal()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printCopyTable renders a counts table for the copy operation.
func printCopyTable(succeeded, failed int, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "success\t%d\n", succeeded)
	_, _ = fmt.Fprintf(tw, "failure\t%d\n", failed)
	return tw.Flush()
}

// Prints a 'brief' (not detailed) summary and returns the appropriate exit error.
func printBriefSummaryAndGetError(succeeded, failed int, failNoOp bool, originalErr error) error {
	err := common.PrintBriefSummaryReport(succeeded, failed, failNoOp, originalErr)
	return common.GetCliError(err, succeeded, failed, failNoOp)
}

func prepareDeleteCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 1 && (c.GetNumberOfArgs() != 0 || (!c.IsFlagSet("spec") && !c.IsFlagSet("build") && !c.IsFlagSet("bundle"))) {
		return nil, common.WrongNumberOfArgumentsHandler(c)
	}

	var deleteSpec *spec.SpecFiles
	var err error
	if c.IsFlagSet("spec") {
		deleteSpec, err = commonCliUtils.GetSpec(c, false, true)
	} else {
		deleteSpec, err = createDefaultDeleteSpec(c)
	}
	if err != nil {
		return nil, err
	}
	err = spec.ValidateSpec(deleteSpec.Files, false, true)
	if err != nil {
		return nil, err
	}
	return deleteSpec, nil
}

func deleteCmd(c *components.Context) error {
	deleteSpec, err := prepareDeleteCommand(c)
	if err != nil {
		return err
	}

	deleteCommand := generic.NewDeleteCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	threads, err := common.GetThreadsCount(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	deleteCommand.SetThreads(threads).SetQuiet(common.GetQuietValue(c)).SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails).SetSpec(deleteSpec).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(deleteCommand)
	result := deleteCommand.Result()

	outputFormat, fmtErr := getDeleteOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}
	if outputFormat == coreformat.None {
		return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	}
	return printDeleteResponse(result.SuccessCount(), result.FailCount(), outputFormat, os.Stdout, common.IsFailNoOp(c), err)
}

// getDeleteOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (brief summary output via printBriefSummaryAndGetError).
func getDeleteOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printDeleteResponse renders the delete result in the requested output format.
func printDeleteResponse(succeeded, failed int, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		err := printDeleteJSON(succeeded, failed, failNoOp, originalErr)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	case coreformat.Table:
		err := printDeleteTable(succeeded, failed, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt delete. Acceptable values are: json, table", outputFormat)
	}
}

// printDeleteJSON emits a JSON summary of the delete operation to stdout via log.Output.
func printDeleteJSON(succeeded, failed int, failNoOp bool, originalErr error) error {
	summaryReport := summary.GetSummaryReport(succeeded, failed, failNoOp, originalErr)
	data, err := summaryReport.Marshal()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printDeleteTable renders a counts table for the delete operation.
func printDeleteTable(succeeded, failed int, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "success\t%d\n", succeeded)
	_, _ = fmt.Fprintf(tw, "failure\t%d\n", failed)
	return tw.Flush()
}

func prepareSearchCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 1 && (c.GetNumberOfArgs() != 0 || (!c.IsFlagSet("spec") && !c.IsFlagSet("build") && !c.IsFlagSet("bundle"))) {
		return nil, common.WrongNumberOfArgumentsHandler(c)
	}

	var searchSpec *spec.SpecFiles
	var err error
	if c.IsFlagSet("spec") {
		searchSpec, err = commonCliUtils.GetSpec(c, false, true)
	} else {
		searchSpec, err = createDefaultSearchSpec(c)
	}
	if err != nil {
		return nil, err
	}
	err = spec.ValidateSpec(searchSpec.Files, false, true)
	if err != nil {
		return nil, err
	}
	return searchSpec, err
}

func searchCmd(c *components.Context) (err error) {
	searchSpec, err := prepareSearchCommand(c)
	if err != nil {
		return
	}
	artDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return
	}
	retries, err := getRetries(c)
	if err != nil {
		return
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return
	}
	cmd := generic.NewSearchCommand()
	cmd.SetServerDetails(artDetails).SetSpec(searchSpec).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(cmd)
	if err != nil {
		return
	}
	reader := cmd.Result().Reader()
	defer ioutils.Close(reader, &err)
	length, err := reader.Length()
	if err != nil {
		return err
	}
	err = common.GetCliError(err, length, 0, common.IsFailNoOp(c))
	if err != nil {
		return err
	}
	if c.GetBoolFlagValue("count") {
		log.Output(length)
		return nil
	}
	outputFormat, err := getSearchOutputFormat(c)
	if err != nil {
		return err
	}
	return printSearchResponse(reader, outputFormat)
}

// searchTableRow is a table-printable representation of a search result item.
type searchTableRow struct {
	Path     string `col-name:"PATH"`
	Type     string `col-name:"TYPE"`
	Size     string `col-name:"SIZE"`
	Created  string `col-name:"CREATED"`
	Modified string `col-name:"MODIFIED"`
	Sha256   string `col-name:"SHA256"`
}

// getSearchOutputFormat reads the --format flag and returns the resolved output format.
// Default is json to preserve backward-compatible behaviour (the command has always emitted JSON).
func getSearchOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.Json, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printSearchResponse renders ContentReader results in the requested output format.
func printSearchResponse(reader *content.ContentReader, outputFormat coreformat.OutputFormat) error {
	switch outputFormat {
	case coreformat.Json:
		return utils.PrintSearchResults(reader)
	case coreformat.Table:
		return printSearchTable(reader)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt search. Acceptable values are: json, table", outputFormat)
	}
}

// printSearchTable prints search results as a human-readable table using coreutils.PrintTable.
func printSearchTable(reader *content.ContentReader) error {
	var rows []searchTableRow
	for item := new(utils.SearchResult); reader.NextRecord(item) == nil; item = new(utils.SearchResult) {
		rows = append(rows, searchTableRow{
			Path:     item.Path,
			Type:     item.Type,
			Size:     fmt.Sprintf("%d", item.Size),
			Created:  item.Created,
			Modified: item.Modified,
			Sha256:   item.Sha256,
		})
	}
	if err := reader.GetError(); err != nil {
		return err
	}
	reader.Reset()
	return coreutils.PrintTable(rows, "Search Results", "No artifacts found.", false)
}

func preparePropsCmd(c *components.Context) (*generic.PropsCommand, error) {
	if c.GetNumberOfArgs() > 1 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("Only the 'artifact properties' argument should be sent when the spec option is used.", c)
	}
	if c.GetNumberOfArgs() != 2 && (c.GetNumberOfArgs() != 1 || (!c.IsFlagSet("spec") && !c.IsFlagSet("build") && !c.IsFlagSet("bundle"))) {
		return nil, common.WrongNumberOfArgumentsHandler(c)
	}

	var propsSpec *spec.SpecFiles
	var err error
	var props string
	if c.IsFlagSet("spec") {
		props = c.GetArgumentAt(0)
		propsSpec, err = commonCliUtils.GetSpec(c, false, true)
	} else {
		propsSpec, err = createDefaultPropertiesSpec(c)
		if c.GetNumberOfArgs() == 1 {
			props = c.GetArgumentAt(0)
			propsSpec.Get(0).Pattern = "*"
		} else {
			props = c.GetArgumentAt(1)
		}
	}
	if err != nil {
		return nil, err
	}
	err = spec.ValidateSpec(propsSpec.Files, false, true)
	if err != nil {
		return nil, err
	}

	command := generic.NewPropsCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return nil, err
	}
	threads, err := common.GetThreadsCount(c)
	if err != nil {
		return nil, err
	}

	cmd := command.SetProps(props)
	cmd.SetThreads(threads).SetSpec(propsSpec).SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails)
	return cmd, nil
}

func setPropsCmd(c *components.Context) error {
	cmd, err := preparePropsCmd(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	propsCmd := generic.NewSetPropsCommand().SetPropsCommand(*cmd).SetRepoOnly(c.GetBoolFlagValue("repo-only"))
	propsCmd.SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(propsCmd)
	result := propsCmd.Result()

	outputFormat, fmtErr := getSetPropsOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}
	if outputFormat == coreformat.None {
		return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	}
	return printSetPropsResponse(result.SuccessCount(), result.FailCount(), outputFormat, os.Stdout, common.IsFailNoOp(c), err)
}

// getSetPropsOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (brief summary output via printBriefSummaryAndGetError).
func getSetPropsOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printSetPropsResponse renders the set-props result in the requested output format.
func printSetPropsResponse(succeeded, failed int, outputFormat coreformat.OutputFormat, w io.Writer, failNoOp bool, originalErr error) error {
	switch outputFormat {
	case coreformat.Json:
		err := printSetPropsJSON(succeeded, failed, failNoOp, originalErr)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	case coreformat.Table:
		err := printSetPropsTable(succeeded, failed, w)
		if err != nil {
			return err
		}
		return common.GetCliError(originalErr, succeeded, failed, failNoOp)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt set-props. Acceptable values are: json, table", outputFormat)
	}
}

// printSetPropsJSON emits a JSON summary of the set-props operation to stdout via log.Output.
func printSetPropsJSON(succeeded, failed int, failNoOp bool, originalErr error) error {
	summaryReport := summary.GetSummaryReport(succeeded, failed, failNoOp, originalErr)
	data, err := summaryReport.Marshal()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printSetPropsTable renders a counts table for the set-props operation.
func printSetPropsTable(succeeded, failed int, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "success\t%d\n", succeeded)
	_, _ = fmt.Fprintf(tw, "failure\t%d\n", failed)
	return tw.Flush()
}

func deletePropsCmd(c *components.Context) error {
	cmd, err := preparePropsCmd(c)
	if err != nil {
		return err
	}
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	propsCmd := generic.NewDeletePropsCommand().DeletePropsCommand(*cmd).SetRepoOnly(c.GetBoolFlagValue("repo-only"))
	propsCmd.SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(propsCmd)
	result := propsCmd.Result()
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
}

func buildPublishCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	buildInfoConfiguration := createBuildInfoConfiguration(c)
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	outputFormat, fmtErr := getBuildPublishOutputFormat(c)
	if fmtErr != nil {
		return fmtErr
	}

	cmd := buildinfo.NewBuildPublishCommand().SetServerDetails(rtDetails).SetBuildConfiguration(buildConfiguration).SetConfig(buildInfoConfiguration).SetDetailedSummary(common.GetDetailedSummary(c))
	cmd.SetCollectEnv(c.GetBoolFlagValue("collect-env"))
	cmd.SetCollectGitInfo(c.GetBoolFlagValue("collect-git-info"))
	cmd.SetDotGitPath(c.GetStringFlagValue("dot-git-path"))
	cmd.SetConfigFilePath(c.GetStringFlagValue("git-config-file-path"))
	cmd.SetDepExcludeScopes(c.GetStringsArrFlagValue("dep-exclude-scopes"))

	// When --format is set, suppress the internal logJsonOutput call so that the
	// CLI layer can render the URL itself.
	if outputFormat != coreformat.None {
		cmd.SetSuppressOutput(true)
	}

	err = commands.Exec(cmd)

	// When --detailed-summary is set and --format is NOT set, keep the
	// existing SHA-256 summary behaviour.
	if cmd.IsDetailedSummary() && outputFormat == coreformat.None {
		if publishedSummary := cmd.GetSummary(); publishedSummary != nil {
			return summary.PrintBuildInfoSummaryReport(publishedSummary.IsSucceeded(), publishedSummary.GetSha256(), err)
		}
	}

	if outputFormat == coreformat.None {
		return err
	}
	if err != nil {
		return err
	}
	return printBuildPublishResponse(cmd.GetBuildInfoUiUrl(), outputFormat, os.Stdout)
}

// getBuildPublishOutputFormat reads the --format flag and returns the resolved output format.
// When the flag is not set the function returns coreformat.None, preserving the
// previous behaviour (JSON printed internally by Run()).
func getBuildPublishOutputFormat(c *components.Context) (coreformat.OutputFormat, error) {
	if !c.IsFlagSet(flagkit.Format) {
		return coreformat.None, nil
	}
	return common.ExtractOutputFormat(c, []coreformat.OutputFormat{coreformat.Json, coreformat.Table})
}

// printBuildPublishResponse renders the build-publish result in the requested output format.
func printBuildPublishResponse(buildInfoUiUrl string, outputFormat coreformat.OutputFormat, w io.Writer) error {
	switch outputFormat {
	case coreformat.Json:
		return printBuildPublishJSON(buildInfoUiUrl)
	case coreformat.Table:
		return printBuildPublishTable(buildInfoUiUrl, w)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for rt build-publish. Acceptable values are: json, table", outputFormat)
	}
}

// printBuildPublishJSON emits the build-publish result as indented JSON.
func printBuildPublishJSON(buildInfoUiUrl string) error {
	output := formats.BuildPublishOutput{BuildInfoUiUrl: buildInfoUiUrl}
	data, err := output.JSON()
	if err != nil {
		return errorutils.CheckError(err)
	}
	log.Output(clientutils.IndentJson(data))
	return nil
}

// printBuildPublishTable renders the build-publish result as a two-column tabwriter table.
func printBuildPublishTable(buildInfoUiUrl string, w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FIELD\tVALUE")
	_, _ = fmt.Fprintf(tw, "buildInfoUiUrl\t%s\n", buildInfoUiUrl)
	return tw.Flush()
}

func buildAppendCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 4 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	buildNameToAppend, buildNumberToAppend := c.GetArgumentAt(2), c.GetArgumentAt(3)
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildAppendCmd := buildinfo.NewBuildAppendCommand().SetServerDetails(rtDetails).SetBuildConfiguration(buildConfiguration).SetBuildNameToAppend(buildNameToAppend).SetBuildNumberToAppend(buildNumberToAppend)
	return commands.Exec(buildAppendCmd)
}

func buildAddDependenciesCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 2 && c.IsFlagSet("spec") {
		return common.PrintHelpAndReturnError("Only path or spec is allowed, not both.", c)
	}
	if c.IsFlagSet("regexp") && c.IsFlagSet("from-rt") {
		return common.PrintHelpAndReturnError("The --regexp option is not supported when --from-rt is set to true.", c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	// Odd number of args - Use pattern arg
	// Even number of args - Use spec flag
	if c.GetNumberOfArgs() > 3 || (c.GetNumberOfArgs()%2 != 1 && (c.GetNumberOfArgs()%2 != 0 || !c.IsFlagSet("spec"))) {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	var dependenciesSpec *spec.SpecFiles
	var rtDetails *config.ServerDetails
	var err error
	if c.IsFlagSet("spec") {
		dependenciesSpec, err = commonCliUtils.GetSpec(c, true, true)
		if err != nil {
			return err
		}
	} else {
		dependenciesSpec = createDefaultBuildAddDependenciesSpec(c)
	}
	if c.GetBoolFlagValue("from-rt") {
		rtDetails, err = common.CreateArtifactoryDetailsByFlags(c)
		if err != nil {
			return err
		}
	} else {
		common.FixWinPathsForFileSystemSourcedCmds(dependenciesSpec, c)
	}
	buildAddDependenciesCmd := buildinfo.NewBuildAddDependenciesCommand().SetDryRun(c.GetBoolFlagValue("dry-run")).SetBuildConfiguration(buildConfiguration).SetDependenciesSpec(dependenciesSpec).SetServerDetails(rtDetails)
	err = commands.Exec(buildAddDependenciesCmd)
	result := buildAddDependenciesCmd.Result()
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
}

func buildCollectEnvCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	buildCollectEnvCmd := buildinfo.NewBuildCollectEnvCommand().SetBuildConfiguration(buildConfiguration)

	return commands.Exec(buildCollectEnvCmd)
}

func buildAddGitCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 3 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}

	buildAddGitConfigurationCmd := buildinfo.NewBuildAddGitCommand().SetBuildConfiguration(buildConfiguration).SetConfigFilePath(c.GetStringFlagValue("config")).SetServerId(c.GetStringFlagValue("server-id"))
	if c.GetNumberOfArgs() == 3 {
		buildAddGitConfigurationCmd.SetDotGitPath(c.GetArgumentAt(2))
	} else if c.GetNumberOfArgs() == 1 {
		buildAddGitConfigurationCmd.SetDotGitPath(c.GetArgumentAt(0))
	}
	return commands.Exec(buildAddGitConfigurationCmd)
}

func buildScanLegacyCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildScanCmd := buildinfo.NewBuildScanLegacyCommand().SetServerDetails(rtDetails).SetFailBuild(c.GetBoolTFlagValue("fail")).SetBuildConfiguration(buildConfiguration)
	err = commands.Exec(buildScanCmd)

	return checkBuildScanError(err)
}

func checkBuildScanError(err error) error {
	// If the build was found vulnerable, exit with ExitCodeVulnerableBuild.
	if errors.Is(err, utils.GetBuildScanError()) {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeVulnerableBuild, ErrorMsg: err.Error()}
	}
	// If the scan operation failed, for example due to HTTP timeout, exit with ExitCodeError.
	if err != nil {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError, ErrorMsg: err.Error()}
	}
	return nil
}

func buildCleanCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 2 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	buildCleanCmd := buildinfo.NewBuildCleanCommand().SetBuildConfiguration(buildConfiguration)
	return commands.Exec(buildCleanCmd)
}

func buildPromoteCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 3 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	if c.IsFlagSet(flagkit.Format) {
		if _, fmtErr := coreformat.ParseOutputFormat(c.GetStringFlagValue(flagkit.Format), []coreformat.OutputFormat{coreformat.Json}); fmtErr != nil {
			return fmtErr
		}
	}

	configuration := createBuildPromoteConfiguration(c)
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildConfiguration := common.CreateBuildConfiguration(c)
	if err := buildConfiguration.ValidateBuildParams(); err != nil {
		return err
	}
	buildPromotionCmd := buildinfo.NewBuildPromotionCommand().SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails).SetPromotionParams(configuration).SetBuildConfiguration(buildConfiguration)
	if err = commands.Exec(buildPromotionCmd); err != nil {
		return err
	}

	// error == nil guarantees the server responded with 200.
	// The client layer discards the body, so we pass nil and let the helper
	// synthesize {"status_code": 200, "message": "OK"}.
	if c.IsFlagSet(flagkit.Format) {
		printBuildPromoteJSON()
	}
	return nil
}

// printBuildPromoteJSON emits a synthetic JSON success response for build-promote.
// The Artifactory promote API returns a body, but the client layer discards it;
// error == nil guarantees HTTP 200, so we synthesise {"status_code":200,"message":"OK"}.
func printBuildPromoteJSON() {
	synthetic := map[string]interface{}{
		"status_code": 200,
		"message":     "OK",
	}
	data, _ := json.Marshal(synthetic)
	log.Output(clientutils.IndentJson(data))
}

func buildDiscardCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	if c.IsFlagSet(flagkit.Format) {
		if _, fmtErr := coreformat.ParseOutputFormat(c.GetStringFlagValue(flagkit.Format), []coreformat.OutputFormat{coreformat.Json}); fmtErr != nil {
			return fmtErr
		}
	}

	configuration := createBuildDiscardConfiguration(c)
	if configuration.BuildName == "" {
		return common.PrintHelpAndReturnError("Build name is expected as a command argument or environment variable.", c)
	}
	buildDiscardCommand := buildinfo.NewBuildDiscardCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildDiscardCommand.SetServerDetails(rtDetails).SetDiscardBuildsParams(configuration)

	if err = commands.Exec(buildDiscardCommand); err != nil {
		return err
	}

	// error == nil guarantees the server responded with 204 No Content.
	// The client layer discards the body, so we pass nil and let the helper
	// synthesize {"status_code": 204, "message": "No Content"}.
	if c.IsFlagSet(flagkit.Format) {
		printBuildDiscardJSON()
	}
	return nil
}

// printBuildDiscardJSON emits a synthetic JSON success response for build-discard.
// The Artifactory discard API returns 204 No Content with no body; the client layer
// discards the response, so error == nil guarantees HTTP 204.
func printBuildDiscardJSON() {
	synthetic := map[string]interface{}{
		"status_code": 204,
		"message":     "No Content",
	}
	data, _ := json.Marshal(synthetic)
	log.Output(clientutils.IndentJson(data))
}

func gitLfsCleanCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	configuration := createGitLfsCleanConfiguration(c)
	retries, err := getRetries(c)
	if err != nil {
		return err
	}
	retryWaitTime, err := getRetryWaitTime(c)
	if err != nil {
		return err
	}
	gitLfsCmd := generic.NewGitLfsCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	gitLfsCmd.SetConfiguration(configuration).SetServerDetails(rtDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	return commands.Exec(gitLfsCmd)
}

func curlCmd(c *components.Context) error {
	if show, err := common.ShowCmdHelpIfNeeded(c, c.Arguments); show || err != nil {
		return err
	}
	if c.GetNumberOfArgs() < 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	rtCurlCommand, err := newRtCurlCommand(c)
	if err != nil {
		return err
	}

	// Check if --server-id is explicitly passed in arguments
	flagIndex, _, _, err := coreutils.FindFlag("--server-id", common.ExtractCommand(c))
	if err != nil {
		return err
	}
	// If --server-id is NOT present, then we check for JFROG_CLI_SERVER_ID env variable
	if flagIndex == -1 {
		if artDetails, err := common.CreateArtifactoryDetailsByFlags(c); err == nil && artDetails.ArtifactoryUrl != "" {
			rtCurlCommand.SetServerDetails(artDetails)
			rtCurlCommand.SetUrl(artDetails.ArtifactoryUrl)
		}
	}
	return commands.Exec(rtCurlCommand)
}

func newRtCurlCommand(c *components.Context) (*curl.RtCurlCommand, error) {
	curlCommand := commands.NewCurlCommand().SetArguments(common.ExtractCommand(c))
	rtCurlCommand := curl.NewRtCurlCommand(*curlCommand)
	rtDetails, err := rtCurlCommand.GetServerDetails()
	if err != nil {
		return nil, err
	}
	if rtDetails.ArtifactoryUrl == "" {
		return nil, errorutils.CheckErrorf("No Artifactory servers configured. Use the 'jf c add' command to set the Artifactory server details.")
	}
	rtCurlCommand.SetServerDetails(rtDetails)
	rtCurlCommand.SetUrl(rtDetails.ArtifactoryUrl)
	return rtCurlCommand, err
}

func repoTemplateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	// Run command.
	repoTemplateCmd := repository.NewRepoTemplateCommand()
	repoTemplateCmd.SetTemplatePath(c.GetArgumentAt(0))
	return commands.Exec(repoTemplateCmd)
}

func repoCreateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	// Run command.
	repoCreateCmd := repository.NewRepoCreateCommand()
	repoCreateCmd.SetTemplatePath(c.GetArgumentAt(0)).SetServerDetails(rtDetails).SetVars(c.GetStringFlagValue("vars"))
	return commands.Exec(repoCreateCmd)
}

func repoUpdateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	// Run command.
	repoUpdateCmd := repository.NewRepoUpdateCommand()
	repoUpdateCmd.SetTemplatePath(c.GetArgumentAt(0)).SetServerDetails(rtDetails).SetVars(c.GetStringFlagValue("vars"))
	return commands.Exec(repoUpdateCmd)
}

func repoDeleteCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}

	repoDeleteCmd := repository.NewRepoDeleteCommand()
	repoDeleteCmd.SetRepoPattern(c.GetArgumentAt(0)).SetServerDetails(rtDetails).SetQuiet(common.GetQuietValue(c))
	return commands.Exec(repoDeleteCmd)
}

func replicationTemplateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	replicationTemplateCmd := replication.NewReplicationTemplateCommand()
	replicationTemplateCmd.SetTemplatePath(c.GetArgumentAt(0))
	return commands.Exec(replicationTemplateCmd)
}

func replicationCreateCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	replicationCreateCmd := replication.NewReplicationCreateCommand()
	replicationCreateCmd.SetTemplatePath(c.GetArgumentAt(0)).SetServerDetails(rtDetails).SetVars(c.GetStringFlagValue("vars"))
	return commands.Exec(replicationCreateCmd)
}

func replicationDeleteCmd(c *components.Context) error {
	if c.GetNumberOfArgs() != 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	replicationDeleteCmd := replication.NewReplicationDeleteCommand()
	replicationDeleteCmd.SetRepoKey(c.GetArgumentAt(0)).SetServerDetails(rtDetails).SetQuiet(common.GetQuietValue(c))
	return commands.Exec(replicationDeleteCmd)
}

func createDefaultCopyMoveSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}
	return spec.NewBuilder().
		Pattern(c.GetArgumentAt(0)).
		Props(c.GetStringFlagValue("props")).
		ExcludeProps(c.GetStringFlagValue("exclude-props")).
		Build(c.GetStringFlagValue("build")).
		Project(common.GetProject(c)).
		ExcludeArtifacts(c.GetBoolFlagValue("exclude-artifacts")).
		IncludeDeps(c.GetBoolFlagValue("include-deps")).
		Bundle(c.GetStringFlagValue("bundle")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		Flat(c.GetBoolFlagValue("flat")).
		IncludeDirs(true).
		Target(c.GetArgumentAt(1)).
		ArchiveEntries(c.GetStringFlagValue("archive-entries")).
		BuildSpec(), nil
}

func createDefaultDeleteSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}
	return spec.NewBuilder().
		Pattern(c.GetArgumentAt(0)).
		Props(c.GetStringFlagValue("props")).
		ExcludeProps(c.GetStringFlagValue("exclude-props")).
		Build(c.GetStringFlagValue("build")).
		Project(common.GetProject(c)).
		ExcludeArtifacts(c.GetBoolFlagValue("exclude-artifacts")).
		IncludeDeps(c.GetBoolFlagValue("include-deps")).
		Bundle(c.GetStringFlagValue("bundle")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		ArchiveEntries(c.GetStringFlagValue("archive-entries")).
		BuildSpec(), nil
}

func createDefaultSearchSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}
	return spec.NewBuilder().
		Pattern(c.GetArgumentAt(0)).
		Props(c.GetStringFlagValue("props")).
		ExcludeProps(c.GetStringFlagValue("exclude-props")).
		Build(c.GetStringFlagValue("build")).
		Project(common.GetProject(c)).
		ExcludeArtifacts(c.GetBoolFlagValue("exclude-artifacts")).
		IncludeDeps(c.GetBoolFlagValue("include-deps")).
		Bundle(c.GetStringFlagValue("bundle")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		IncludeDirs(c.GetBoolFlagValue("include-dirs")).
		ArchiveEntries(c.GetStringFlagValue("archive-entries")).
		Transitive(c.GetBoolFlagValue("transitive")).
		Include(c.GetStringsArrFlagValue("include")).
		BuildSpec(), nil
}

func createDefaultPropertiesSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}
	return spec.NewBuilder().
		Pattern(c.GetArgumentAt(0)).
		Props(c.GetStringFlagValue("props")).
		ExcludeProps(c.GetStringFlagValue("exclude-props")).
		Build(c.GetStringFlagValue("build")).
		Project(common.GetProject(c)).
		ExcludeArtifacts(c.GetBoolFlagValue("exclude-artifacts")).
		IncludeDeps(c.GetBoolFlagValue("include-deps")).
		Bundle(c.GetStringFlagValue("bundle")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		IncludeDirs(c.GetBoolFlagValue("include-dirs")).
		ArchiveEntries(c.GetStringFlagValue("archive-entries")).
		RepoOnly(c.GetBoolTFlagValue("repo-only")).
		BuildSpec(), nil
}

func createBuildInfoConfiguration(c *components.Context) *buildinfocmd.Configuration {
	flags := new(buildinfocmd.Configuration)
	flags.BuildUrl = common.GetBuildUrl(c.GetStringFlagValue("build-url"))
	flags.DryRun = c.GetBoolFlagValue("dry-run")
	flags.EnvInclude = c.GetStringFlagValue("env-include")
	flags.EnvExclude = common.GetEnvExclude(c.GetStringFlagValue("env-exclude"))
	if flags.EnvInclude == "" {
		flags.EnvInclude = "*"
	}
	// Allow using `env-exclude=""` and get no filters
	if flags.EnvExclude == "" {
		flags.EnvExclude = "*password*;*psw*;*secret*;*key*;*token*;*auth*"
	}
	flags.Overwrite = c.GetBoolFlagValue("overwrite")
	return flags
}

func createBuildPromoteConfiguration(c *components.Context) services.PromotionParams {
	promotionParamsImpl := services.NewPromotionParams()
	promotionParamsImpl.Comment = c.GetStringFlagValue("comment")
	promotionParamsImpl.SourceRepo = c.GetStringFlagValue("source-repo")
	promotionParamsImpl.Status = c.GetStringFlagValue("status")
	promotionParamsImpl.IncludeDependencies = c.GetBoolFlagValue("include-dependencies")
	promotionParamsImpl.Copy = c.GetBoolFlagValue("copy")
	promotionParamsImpl.Properties = c.GetStringFlagValue("props")
	promotionParamsImpl.ProjectKey = common.GetProject(c)
	promotionParamsImpl.FailFast = c.GetBoolTFlagValue("fail-fast")

	// If the command received 3 args, read the build name, build number
	// and target repo as ags.
	buildName, buildNumber, targetRepo := c.GetArgumentAt(0), c.GetArgumentAt(1), c.GetArgumentAt(2)
	// But if the command received only one arg, the build name and build number
	// are expected as env vars, and only the target repo is received as an arg.
	if len(c.Arguments) == 1 {
		buildName, buildNumber, targetRepo = "", "", c.GetArgumentAt(0)
	}

	promotionParamsImpl.BuildName, promotionParamsImpl.BuildNumber = buildName, buildNumber
	promotionParamsImpl.TargetRepo = targetRepo
	return promotionParamsImpl
}

func createBuildDiscardConfiguration(c *components.Context) services.DiscardBuildsParams {
	discardParamsImpl := services.NewDiscardBuildsParams()
	discardParamsImpl.DeleteArtifacts = c.GetBoolFlagValue("delete-artifacts")
	discardParamsImpl.MaxBuilds = c.GetStringFlagValue("max-builds")
	discardParamsImpl.MaxDays = c.GetStringFlagValue("max-days")
	discardParamsImpl.ExcludeBuilds = c.GetStringFlagValue("exclude-builds")
	discardParamsImpl.Async = c.GetBoolFlagValue("async")
	discardParamsImpl.BuildName = common.GetBuildName(c.GetArgumentAt(0))
	discardParamsImpl.ProjectKey = common.GetProject(c)
	return discardParamsImpl
}

func createGitLfsCleanConfiguration(c *components.Context) (gitLfsCleanConfiguration *generic.GitLfsCleanConfiguration) {
	gitLfsCleanConfiguration = new(generic.GitLfsCleanConfiguration)

	gitLfsCleanConfiguration.Refs = c.GetStringFlagValue("refs")
	if len(gitLfsCleanConfiguration.Refs) == 0 {
		gitLfsCleanConfiguration.Refs = "refs/remotes/*"
	}

	gitLfsCleanConfiguration.Repo = c.GetStringFlagValue("repo")
	gitLfsCleanConfiguration.Quiet = common.GetQuietValue(c)
	dotGitPath := ""
	if c.GetNumberOfArgs() == 1 {
		dotGitPath = c.GetArgumentAt(0)
	}
	gitLfsCleanConfiguration.GitPath = dotGitPath
	return
}

func createDefaultDownloadSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}

	excludeArtifactsString := c.GetStringFlagValue("exclude-artifacts")
	if excludeArtifactsString == "" {
		excludeArtifactsString = "false"
	}
	excludeArtifacts, err := strconv.ParseBool(excludeArtifactsString)
	if err != nil {
		log.Warn("Could not parse exclude-artifacts flag. Setting exclude-artifacts as false, error: ", err.Error())
		excludeArtifacts = false
	}

	includeArtifactsString := c.GetStringFlagValue("include-deps")
	if includeArtifactsString == "" {
		includeArtifactsString = "false"
	}
	includeDeps, err := strconv.ParseBool(includeArtifactsString)
	if err != nil {
		log.Warn("Could not parse include-deps flag. Setting include-deps as false, error: ", err.Error())
		excludeArtifacts = false
	}

	return spec.NewBuilder().
		Pattern(getSourcePattern(c)).
		Props(c.GetStringFlagValue("props")).
		ExcludeProps(c.GetStringFlagValue("exclude-props")).
		Build(c.GetStringFlagValue("build")).
		Project(common.GetProject(c)).
		ExcludeArtifacts(excludeArtifacts).
		IncludeDeps(includeDeps).
		Bundle(c.GetStringFlagValue("bundle")).
		PublicGpgKey(c.GetStringFlagValue("gpg-key")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		Flat(c.GetBoolFlagValue("flat")).
		Explode(strconv.FormatBool(c.GetBoolFlagValue("explode"))).
		BypassArchiveInspection(c.GetBoolFlagValue("bypass-archive-inspection")).
		IncludeDirs(c.GetBoolFlagValue("include-dirs")).
		Target(c.GetArgumentAt(1)).
		ArchiveEntries(c.GetStringFlagValue("archive-entries")).
		ValidateSymlinks(c.GetBoolFlagValue("validate-symlinks")).
		BuildSpec(), nil
}

func getSourcePattern(c *components.Context) string {
	var source string
	var isRbv2 bool
	var err error

	if c.IsFlagSet("bundle") {
		// If the bundle flag is set, we need to check if the bundle exists in rbv2
		isRbv2, err = checkRbExistenceInV2(c)
		if err != nil {
			log.Error("Error occurred while checking if the bundle exists in rbv2:", err.Error())
		}
	}

	if isRbv2 {
		// RB2 will be downloaded like a regular artifact, path: projectKey-release-bundles-v2/rbName/rbVersion
		source = buildSourceForRbv2(c)
	} else {
		source = strings.TrimPrefix(c.GetArgumentAt(0), "/")
	}

	return source
}

func buildSourceForRbv2(c *components.Context) string {
	bundleNameAndVersion := c.GetStringFlagValue("bundle")
	projectKey := c.GetStringFlagValue("project")
	source := projectKey

	// Reset bundle flag
	c.SetStringFlagValue("bundle", "")

	// If projectKey is not empty, append "-" to it
	if projectKey != "" {
		source += "-"
	}
	// Build RB path: projectKey-release-bundles-v2/rbName/rbVersion/
	source += releaseBundlesV2 + "/" + bundleNameAndVersion + "/"
	return source
}

func setTransitiveInDownloadSpec(downloadSpec *spec.SpecFiles) {
	transitive := os.Getenv(coreutils.TransitiveDownload)
	if transitive == "" {
		if transitive = os.Getenv(coreutils.TransitiveDownloadExperimental); transitive == "" {
			return
		}
	}
	for fileIndex := 0; fileIndex < len(downloadSpec.Files); fileIndex++ {
		downloadSpec.Files[fileIndex].Transitive = transitive
	}
}

func createDefaultUploadSpec(c *components.Context) (*spec.SpecFiles, error) {
	offset, limit, err := getOffsetAndLimitValues(c)
	if err != nil {
		return nil, err
	}
	return spec.NewBuilder().
		Pattern(c.GetArgumentAt(0)).
		Props(c.GetStringFlagValue("props")).
		TargetProps(c.GetStringFlagValue("target-props")).
		Offset(offset).
		Limit(limit).
		SortOrder(c.GetStringFlagValue("sort-order")).
		SortBy(c.GetStringsArrFlagValue("sort-by")).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		Flat(c.GetBoolFlagValue("flat")).
		Explode(strconv.FormatBool(c.GetBoolFlagValue("explode"))).
		Regexp(c.GetBoolFlagValue("regexp")).
		Ant(c.GetBoolFlagValue("ant")).
		IncludeDirs(c.GetBoolFlagValue("include-dirs")).
		Target(strings.TrimPrefix(c.GetArgumentAt(1), "/")).
		Symlinks(c.GetBoolFlagValue("symlinks")).
		Archive(c.GetStringFlagValue("archive")).
		BuildSpec(), nil
}

func createDefaultBuildAddDependenciesSpec(c *components.Context) *spec.SpecFiles {
	pattern := c.GetArgumentAt(2)
	if pattern == "" {
		// Build name and build number from env
		pattern = c.GetArgumentAt(0)
	}
	return spec.NewBuilder().
		Pattern(pattern).
		Recursive(c.GetBoolTFlagValue("recursive")).
		Exclusions(c.GetStringsArrFlagValue("exclusions")).
		Regexp(c.GetBoolFlagValue("regexp")).
		Ant(c.GetBoolFlagValue("ant")).
		BuildSpec()
}

func fixWinPathsForDownloadCmd(uploadSpec *spec.SpecFiles, c *components.Context) {
	if coreutils.IsWindows() {
		for i, file := range uploadSpec.Files {
			uploadSpec.Files[i].Target = commonCliUtils.FixWinPathBySource(file.Target, c.IsFlagSet("spec"))
		}
	}
}

func getOffsetAndLimitValues(c *components.Context) (offset, limit int, err error) {
	offset, err = c.WithDefaultIntFlagValue("offset", 0)
	if err != nil {
		return 0, 0, err
	}
	limit, err = c.WithDefaultIntFlagValue("limit", 0)
	if err != nil {
		return 0, 0, err
	}

	return
}

func GetOffsetAndLimitValues(c *components.Context) (offset, limit int, err error) {
	return getOffsetAndLimitValues(c)
}
