package npm

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"github.com/jfrog/build-info-go/build"
	biutils "github.com/jfrog/build-info-go/build/utils"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/gofrog/version"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/npm"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DistTagPropKey = "npm.disttag"
	// The --pack-destination argument of npm pack was introduced in npm version 7.18.0.
	packDestinationNpmMinVersion = "7.18.0"
)

type NpmPublishCommandArgs struct {
	CommonArgs
	executablePath         string
	workingDirectory       string
	collectBuildInfo       bool
	packedFilePaths        []string
	packageInfo            *biutils.PackageInfo
	publishPath            string
	tarballProvided        bool
	artifactsDetailsReader *content.ContentReader
	xrayScan               bool
	scanOutputFormat       format.OutputFormat
	distTag                string
	useNpmRc               bool
}

type NpmPublishCommand struct {
	configFilePath  string
	commandName     string
	result          *commandsutils.Result
	detailedSummary bool
	npmVersion      *version.Version
	*NpmPublishCommandArgs
}

func NewNpmPublishCommand() *NpmPublishCommand {
	return &NpmPublishCommand{NpmPublishCommandArgs: NewNpmPublishCommandArgs(), commandName: "rt_npm_publish", result: new(commandsutils.Result)}
}

func NewNpmPublishCommandArgs() *NpmPublishCommandArgs {
	return &NpmPublishCommandArgs{}
}

func (npc *NpmPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return npc.serverDetails, nil
}

func (npc *NpmPublishCommand) SetConfigFilePath(configFilePath string) *NpmPublishCommand {
	npc.configFilePath = configFilePath
	return npc
}

func (npc *NpmPublishCommand) SetArgs(args []string) *NpmPublishCommand {
	npc.NpmPublishCommandArgs.npmArgs = args
	return npc
}

func (npc *NpmPublishCommand) SetDetailedSummary(detailedSummary bool) *NpmPublishCommand {
	npc.detailedSummary = detailedSummary
	return npc
}

func (npc *NpmPublishCommand) SetXrayScan(xrayScan bool) *NpmPublishCommand {
	npc.xrayScan = xrayScan
	return npc
}

func (npc *NpmPublishCommand) GetXrayScan() bool {
	return npc.xrayScan
}

func (npc *NpmPublishCommand) SetScanOutputFormat(format format.OutputFormat) *NpmPublishCommand {
	npc.scanOutputFormat = format
	return npc
}

func (npc *NpmPublishCommand) SetDistTag(tag string) *NpmPublishCommand {
	npc.distTag = tag
	return npc
}

func (npc *NpmPublishCommand) Result() *commandsutils.Result {
	return npc.result
}

func (nru *NpmPublishCommand) ShouldUseNpmRc() bool {
	return nru.useNpmRc
}

func (npc *NpmPublishCommand) setUseNpmRc(useNpmRc bool) *NpmPublishCommand {
	npc.useNpmRc = useNpmRc
	return npc
}

func (npc *NpmPublishCommand) IsDetailedSummary() bool {
	return npc.detailedSummary
}

func (npc *NpmPublishCommand) Init() error {
	var err error
	npc.npmVersion, npc.executablePath, err = biutils.GetNpmVersionAndExecPath(log.Logger)
	if err != nil {
		return err
	}
	detailedSummary, xrayScan, scanOutputFormat, filteredNpmArgs, buildConfiguration, err := commandsutils.ExtractNpmOptionsFromArgs(npc.NpmPublishCommandArgs.npmArgs)
	if err != nil {
		return err
	}
	filteredNpmArgs, useNpmRc, err := commandsutils.ExtractNpmConfigType(filteredNpmArgs)
	if err != nil {
		return err
	}
	filteredNpmArgs, tag, err := coreutils.ExtractTagFromArgs(filteredNpmArgs)
	if err != nil {
		return err
	}
	if npc.configFilePath != "" {
		// Read config file.
		log.Debug("Preparing to read the config file", npc.configFilePath)
		vConfig, err := project.ReadConfigFile(npc.configFilePath, project.YAML)
		if err != nil {
			return err
		}
		deployerParams, err := project.GetRepoConfigByPrefix(npc.configFilePath, project.ProjectConfigDeployerPrefix, vConfig)
		if err != nil {
			return err
		}
		rtDetails, err := deployerParams.ServerDetails()
		if err != nil {
			return errorutils.CheckError(err)
		}
		npc.SetBuildConfiguration(buildConfiguration).SetRepo(deployerParams.TargetRepo()).SetNpmArgs(filteredNpmArgs).SetServerDetails(rtDetails)
	}
	npc.SetDetailedSummary(detailedSummary).SetXrayScan(xrayScan).SetScanOutputFormat(scanOutputFormat).SetDistTag(tag).setUseNpmRc(useNpmRc)
	return nil
}

func (npc *NpmPublishCommand) Run() (err error) {
	log.Info("Running npm Publish")
	err = npc.preparePrerequisites()
	if err != nil {
		return err
	}

	var npmBuild *build.Build
	var buildName, buildNumber, projectKey string
	if npc.collectBuildInfo {
		buildName, err = npc.buildConfiguration.GetBuildName()
		if err != nil {
			return err
		}
		buildNumber, err = npc.buildConfiguration.GetBuildNumber()
		if err != nil {
			return err
		}
		projectKey = npc.buildConfiguration.GetProject()
		buildInfoService := buildUtils.CreateBuildInfoService()
		npmBuild, err = buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, projectKey)
		if err != nil {
			return errorutils.CheckError(err)
		}
	}

	if !npc.tarballProvided {
		if err = npc.pack(); err != nil {
			return err
		}
	}

	publishStrategy := NpmPublishStrategy(npc.ShouldUseNpmRc(), npc)

	err = publishStrategy.upload()
	if err != nil {
		if npc.tarballProvided {
			return err
		}
		// We should delete the tarball we created
		return errors.Join(err, deleteCreatedTarball(npc.packedFilePaths))
	}

	if !npc.tarballProvided {
		if err = deleteCreatedTarball(npc.packedFilePaths); err != nil {
			return err
		}
	}

	if !npc.collectBuildInfo {
		log.Info("npm publish finished successfully.")
		return nil
	}

	npmModule, err := npmBuild.AddNpmModule("")
	if err != nil {
		return errorutils.CheckError(err)
	}
	if npc.buildConfiguration.GetModule() != "" {
		npmModule.SetName(npc.buildConfiguration.GetModule())
	}

	buildArtifacts, err := publishStrategy.getBuildArtifacts()
	if err != nil {
		return err
	}
	defer gofrogcmd.Close(npc.artifactsDetailsReader, &err)
	err = npmModule.AddArtifacts(buildArtifacts...)
	if err != nil {
		return errorutils.CheckError(err)
	}

	log.Info("npm publish finished successfully.")
	return nil
}

func (npc *NpmPublishCommand) CommandName() string {
	return npc.commandName
}

func (npc *NpmPublishCommand) preparePrerequisites() error {
	npc.packedFilePaths = make([]string, 0)
	currentDir, err := os.Getwd()
	if err != nil {
		return errorutils.CheckError(err)
	}

	currentDir, err = filepath.Abs(currentDir)
	if err != nil {
		return errorutils.CheckError(err)
	}

	npc.workingDirectory = currentDir
	log.Debug("Working directory set to:", npc.workingDirectory)
	npc.collectBuildInfo, err = npc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if err = npc.setPublishPath(); err != nil {
		return err
	}

	artDetails, err := npc.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return err
	}

	if err = utils.ValidateRepoExists(npc.repo, artDetails); err != nil {
		return err
	}

	return npc.setPackageInfo()
}

func (npc *NpmPublishCommand) pack() error {
	log.Debug("Creating npm package.")
	packedFileNames, err := npm.Pack(npc.npmArgs, npc.executablePath)
	if err != nil {
		return err
	}

	tarballDir, err := npc.getTarballDir()
	if err != nil {
		return err
	}

	for _, packageFileName := range packedFileNames {
		npc.packedFilePaths = append(npc.packedFilePaths, filepath.Join(tarballDir, packageFileName))
	}

	return nil
}

func (npc *NpmPublishCommand) getTarballDir() (string, error) {
	if npc.npmVersion == nil || npc.npmVersion.Compare(packDestinationNpmMinVersion) > 0 {
		return npc.workingDirectory, nil
	}

	// Extract pack destination argument from the args.
	flagIndex, _, dest, err := coreutils.FindFlag("--pack-destination", npc.NpmPublishCommandArgs.npmArgs)
	if err != nil || flagIndex == -1 {
		return npc.workingDirectory, err
	}
	return dest, nil
}

func (npc *NpmPublishCommand) setPublishPath() error {
	log.Debug("Reading Package Json.")

	npc.publishPath = npc.workingDirectory
	if len(npc.npmArgs) > 0 && !strings.HasPrefix(strings.TrimSpace(npc.npmArgs[0]), "-") {
		path := strings.TrimSpace(npc.npmArgs[0])
		path = clientutils.ReplaceTildeWithUserHome(path)

		if filepath.IsAbs(path) {
			npc.publishPath = path
		} else {
			npc.publishPath = filepath.Join(npc.workingDirectory, path)
		}
	}
	return nil
}

func (npc *NpmPublishCommand) setPackageInfo() error {
	log.Debug("Setting Package Info.")
	fileInfo, err := os.Stat(npc.publishPath)
	if err != nil {
		return errorutils.CheckError(err)
	}

	if fileInfo.IsDir() {
		npc.packageInfo, err = biutils.ReadPackageInfoFromPackageJsonIfExists(npc.publishPath, npc.npmVersion)
		return err
	}
	log.Debug("The provided path is not a directory, we assume this is a compressed npm package")
	npc.tarballProvided = true
	// Sets the location of the provided tarball
	npc.packedFilePaths = []string{npc.publishPath}
	return npc.readPackageInfoFromTarball(npc.publishPath)
}

func (npc *NpmPublishCommand) readPackageInfoFromTarball(packedFilePath string) (err error) {
	log.Debug("Extracting info from npm package:", packedFilePath)
	tarball, err := os.Open(packedFilePath)
	if err != nil {
		return errorutils.CheckError(err)
	}
	defer func() {
		err = errors.Join(err, errorutils.CheckError(tarball.Close()))
	}()
	gZipReader, err := gzip.NewReader(tarball)
	if err != nil {
		return errorutils.CheckError(err)
	}

	tarReader := tar.NewReader(gZipReader)
	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return errorutils.CheckErrorf("Could not find 'package.json' in the compressed npm package: " + packedFilePath)
			}
			return errorutils.CheckError(err)
		}
		if hdr.Name == "package/package.json" {
			packageJson, err := io.ReadAll(tarReader)
			if err != nil {
				return errorutils.CheckError(err)
			}
			npc.packageInfo, err = biutils.ReadPackageInfo(packageJson, npc.npmVersion)
			return err
		}
	}
}

func deleteCreatedTarball(packedFilesPath []string) error {
	for _, packedFilePath := range packedFilesPath {
		if err := os.Remove(packedFilePath); err != nil {
			return errorutils.CheckError(err)
		}
		log.Debug("Successfully deleted the created npm package:", packedFilePath)
	}
	return nil
}

func (npc *NpmPublishCommand) getBuildPropsForArtifact() (string, error) {
	buildName, err := npc.buildConfiguration.GetBuildName()
	if err != nil {
		return "", err
	}
	buildNumber, err := npc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return "", err
	}
	err = buildUtils.SaveBuildGeneralDetails(buildName, buildNumber, npc.buildConfiguration.GetProject())
	if err != nil {
		return "", err
	}
	return buildUtils.CreateBuildProperties(buildName, buildNumber, npc.buildConfiguration.GetProject())
}
