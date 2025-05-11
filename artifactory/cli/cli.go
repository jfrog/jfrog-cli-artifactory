package cli

import (
	"os"
	"strconv"
	"strings"

	ioutils "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/buildinfo"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/container"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/curl"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/generic"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/oc"
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
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	containerutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/container"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/cliutils/summary"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/common/progressbar"
	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	buildinfocmd "github.com/jfrog/jfrog-client-go/artifactory/buildinfo"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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
	return []components.Command{
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
	return errorutils.CheckErrorf("The '--retry-wait-time' option should have a numeric value with 's'/'ms' suffix. " + common.GetDocumentationMessage())
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
	printDeploymentView, detailedSummary := log.IsStdErrTerminal(), c.GetBoolFlagValue("detailed-summary")
	dockerPushCommand.SetThreads(threads).SetDetailedSummary(detailedSummary || printDeploymentView).SetCmdParams([]string{"push", imageTag}).SetSkipLogin(skipLogin).SetBuildConfiguration(buildConfiguration).SetRepo(targetRepo).SetServerDetails(artDetails).SetImageTag(imageTag).SetValidateSha(validateSha)
	err = commandWrappers.ShowDockerDeprecationMessageIfNeeded(containerManagerType, dockerPushCommand.IsGetRepoSupported)
	if err != nil {
		return
	}
	err = commands.Exec(dockerPushCommand)
	result := dockerPushCommand.Result()

	// Cleanup.
	defer common.CleanupResult(result, &err)
	err = common.PrintCommandSummary(dockerPushCommand.Result(), detailedSummary, printDeploymentView, false, err)
	return
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
	pingCmd := coregeneric.NewPingCommand()
	pingCmd.SetServerDetails(artDetails)
	err = commands.Exec(pingCmd)
	resString := clientutils.IndentJson(pingCmd.Response())
	if err != nil {
		return errors.New(err.Error() + "\n" + resString)
	}
	log.Output(resString)

	return err
}

func prepareDownloadCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if !(c.GetNumberOfArgs() == 1 || c.GetNumberOfArgs() == 2 || (c.GetNumberOfArgs() == 0 && (c.IsFlagSet("spec") || c.IsFlagSet("build") || c.IsFlagSet("bundle")))) {
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

func downloadCmd(c *components.Context) error {
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
	downloadCommand := generic.NewDownloadCommand()
	downloadCommand.SetConfiguration(configuration).SetBuildConfiguration(buildConfiguration).SetSpec(downloadSpec).SetServerDetails(serverDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetSyncDeletesPath(c.GetStringFlagValue("sync-deletes")).SetQuiet(common.GetQuietValue(c)).SetDetailedSummary(c.GetBoolFlagValue("detailed-summary")).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	if downloadCommand.ShouldPrompt() && !coreutils.AskYesNo("Sync-deletes may delete some files in your local file system. Are you sure you want to continue?\n"+
		"You can avoid this confirmation message by adding --quiet to the command.", false) {
		return nil
	}
	// This error is being checked later on because we need to generate summary report before return.
	err = progressbar.ExecWithProgress(downloadCommand)
	result := downloadCommand.Result()
	defer common.CleanupResult(result, &err)
	basicSummary, err := common.CreateSummaryReportString(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
	if err != nil {
		return err
	}
	err = common.PrintDetailedSummaryReport(basicSummary, result.Reader(), false, err)
	return common.GetCliError(err, result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c))
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

func createLifecycleDetailsByFlags(c *components.Context) (*coreConfig.ServerDetails, error) {
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

func PlatformToLifecycleUrls(lcDetails *coreConfig.ServerDetails) {
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
	if !(c.GetNumberOfArgs() == 2 || (c.GetNumberOfArgs() == 0 && c.IsFlagSet("spec"))) {
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
	uploadCmd := generic.NewUploadCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return
	}
	printDeploymentView, detailedSummary := log.IsStdErrTerminal(), common.GetDetailedSummary(c)
	uploadCmd.SetUploadConfiguration(configuration).SetBuildConfiguration(buildConfiguration).SetSpec(uploadSpec).SetServerDetails(rtDetails).SetDryRun(c.GetBoolFlagValue("dry-run")).SetSyncDeletesPath(c.GetStringFlagValue("sync-deletes")).SetQuiet(common.GetQuietValue(c)).SetDetailedSummary(detailedSummary || printDeploymentView).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)

	if uploadCmd.ShouldPrompt() && !coreutils.AskYesNo("Sync-deletes may delete some artifacts in Artifactory. Are you sure you want to continue?\n"+
		"You can avoid this confirmation message by adding --quiet to the command.", false) {
		return nil
	}
	// This error is being checked later on because we need to generate summary report before return.
	err = progressbar.ExecWithProgress(uploadCmd)
	result := uploadCmd.Result()
	defer common.CleanupResult(result, &err)
	err = common.PrintCommandSummary(uploadCmd.Result(), detailedSummary, printDeploymentView, common.IsFailNoOp(c), err)
	return
}

func prepareCopyMoveCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if !(c.GetNumberOfArgs() == 2 || (c.GetNumberOfArgs() == 0 && (c.IsFlagSet("spec")))) {
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
	moveCmd := generic.NewMoveCommand()
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
	moveCmd.SetThreads(threads).SetDryRun(c.GetBoolFlagValue("dry-run")).SetServerDetails(rtDetails).SetSpec(moveSpec).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(moveCmd)
	result := moveCmd.Result()
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
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
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
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
	if !(c.GetNumberOfArgs() == 1 || (c.GetNumberOfArgs() == 0 && (c.IsFlagSet("spec") || c.IsFlagSet("build") || c.IsFlagSet("bundle")))) {
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
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
}

func prepareSearchCommand(c *components.Context) (*spec.SpecFiles, error) {
	if c.GetNumberOfArgs() > 0 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("No arguments should be sent when the spec option is used.", c)
	}
	if !(c.GetNumberOfArgs() == 1 || (c.GetNumberOfArgs() == 0 && (c.IsFlagSet("spec") || c.IsFlagSet("build") || c.IsFlagSet("bundle")))) {
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
	searchCmd := generic.NewSearchCommand()
	searchCmd.SetServerDetails(artDetails).SetSpec(searchSpec).SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(searchCmd)
	if err != nil {
		return
	}
	reader := searchCmd.Result().Reader()
	defer ioutils.Close(reader, &err)
	length, err := reader.Length()
	if err != nil {
		return err
	}
	err = common.GetCliError(err, length, 0, common.IsFailNoOp(c))
	if err != nil {
		return err
	}
	if !c.GetBoolFlagValue("count") {
		return utils.PrintSearchResults(reader)
	}
	log.Output(length)
	return nil
}

func preparePropsCmd(c *components.Context) (*generic.PropsCommand, error) {
	if c.GetNumberOfArgs() > 1 && c.IsFlagSet("spec") {
		return nil, common.PrintHelpAndReturnError("Only the 'artifact properties' argument should be sent when the spec option is used.", c)
	}
	if !(c.GetNumberOfArgs() == 2 || (c.GetNumberOfArgs() == 1 && (c.IsFlagSet("spec") || c.IsFlagSet("build") || c.IsFlagSet("bundle")))) {
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
	propsCmd := generic.NewSetPropsCommand().SetPropsCommand(*cmd)
	propsCmd.SetRetries(retries).SetRetryWaitMilliSecs(retryWaitTime)
	err = commands.Exec(propsCmd)
	result := propsCmd.Result()
	return printBriefSummaryAndGetError(result.SuccessCount(), result.FailCount(), common.IsFailNoOp(c), err)
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
	propsCmd := generic.NewDeletePropsCommand().DeletePropsCommand(*cmd)
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
	buildPublishCmd := buildinfo.NewBuildPublishCommand().SetServerDetails(rtDetails).SetBuildConfiguration(buildConfiguration).SetConfig(buildInfoConfiguration).SetDetailedSummary(common.GetDetailedSummary(c))

	err = commands.Exec(buildPublishCmd)
	if buildPublishCmd.IsDetailedSummary() {
		if publishedSummary := buildPublishCmd.GetSummary(); publishedSummary != nil {
			return summary.PrintBuildInfoSummaryReport(publishedSummary.IsSucceeded(), publishedSummary.GetSha256(), err)
		}
	}
	return err
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
	if c.GetNumberOfArgs() > 3 || !(c.GetNumberOfArgs()%2 == 1 || (c.GetNumberOfArgs()%2 == 0 && c.IsFlagSet("spec"))) {
		return common.WrongNumberOfArgumentsHandler(c)
	}

	var dependenciesSpec *spec.SpecFiles
	var rtDetails *coreConfig.ServerDetails
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
	return commands.Exec(buildPromotionCmd)
}

func buildDiscardCmd(c *components.Context) error {
	if c.GetNumberOfArgs() > 1 {
		return common.WrongNumberOfArgumentsHandler(c)
	}
	configuration := createBuildDiscardConfiguration(c)
	if configuration.BuildName == "" {
		return common.PrintHelpAndReturnError("Build name is expected as a command argument or environment variable.", c)
	}
	buildDiscardCmd := buildinfo.NewBuildDiscardCommand()
	rtDetails, err := common.CreateArtifactoryDetailsByFlags(c)
	if err != nil {
		return err
	}
	buildDiscardCmd.SetServerDetails(rtDetails).SetDiscardBuildsParams(configuration)

	return commands.Exec(buildDiscardCmd)
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
