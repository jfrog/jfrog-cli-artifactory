package pnpm

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/build"
	biutils "github.com/jfrog/build-info-go/build/utils"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/gofrog/version"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	DistTagPropKey = "npm.disttag"
	// The --pack-destination argument of pnpm pack was introduced in pnpm version 7.0.0.
	packDestinationPnpmMinVersion = "7.0.0"
)

type PnpmPublishCommandArgs struct {
	CommonArgs
	executablePath         string
	workingDirectory       string
	collectBuildInfo       bool
	packedFilePaths        []string
	packageInfo            *biutils.PackageInfo
	publishPath            string
	tarballProvided        bool
	artifactsDetailsReader []*content.ContentReader
	xrayScan               bool
	scanOutputFormat       format.OutputFormat
	distTag                string
}

type PnpmPublishCommand struct {
	configFilePath  string
	commandName     string
	result          *commandsutils.Result
	detailedSummary bool
	pnpmVersion     *version.Version
	*PnpmPublishCommandArgs
}

// packageJsonInfo holds information about a package.json file found in a tarball
type packageJsonInfo struct {
	path    string
	content []byte
}

func NewPnpmPublishCommand() *PnpmPublishCommand {
	return &PnpmPublishCommand{PnpmPublishCommandArgs: NewPnpmPublishCommandArgs(), commandName: "rt_pnpm_publish", result: new(commandsutils.Result)}
}

func NewPnpmPublishCommandArgs() *PnpmPublishCommandArgs {
	return &PnpmPublishCommandArgs{}
}

func (ppc *PnpmPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return ppc.serverDetails, nil
}

func (ppc *PnpmPublishCommand) SetConfigFilePath(configFilePath string) *PnpmPublishCommand {
	ppc.configFilePath = configFilePath
	return ppc
}

func (ppc *PnpmPublishCommand) SetArgs(args []string) *PnpmPublishCommand {
	ppc.pnpmArgs = args
	return ppc
}

func (ppc *PnpmPublishCommand) SetDetailedSummary(detailedSummary bool) *PnpmPublishCommand {
	ppc.detailedSummary = detailedSummary
	return ppc
}

func (ppc *PnpmPublishCommand) SetXrayScan(xrayScan bool) *PnpmPublishCommand {
	ppc.xrayScan = xrayScan
	return ppc
}

func (ppc *PnpmPublishCommand) GetXrayScan() bool {
	return ppc.xrayScan
}

func (ppc *PnpmPublishCommand) SetScanOutputFormat(format format.OutputFormat) *PnpmPublishCommand {
	ppc.scanOutputFormat = format
	return ppc
}

func (ppc *PnpmPublishCommand) SetDistTag(tag string) *PnpmPublishCommand {
	ppc.distTag = tag
	return ppc
}

func (ppc *PnpmPublishCommand) Result() *commandsutils.Result {
	return ppc.result
}

func (ppc *PnpmPublishCommand) IsDetailedSummary() bool {
	return ppc.detailedSummary
}

func (ppc *PnpmPublishCommand) Init() error {
	var err error
	ppc.pnpmVersion, ppc.executablePath, err = biutils.GetPnpmVersionAndExecPath(log.Logger)
	if err != nil {
		return err
	}
	detailedSummary, xrayScan, scanOutputFormat, filteredPnpmArgs, buildConfiguration, err := commandsutils.ExtractNpmOptionsFromArgs(ppc.pnpmArgs)
	if err != nil {
		return err
	}
	filteredPnpmArgs, useNative, err := coreutils.ExtractUseNativeFromArgs(filteredPnpmArgs)
	if err != nil {
		return err
	}
	filteredPnpmArgs, tag, err := coreutils.ExtractTagFromArgs(filteredPnpmArgs)
	if err != nil {
		return err
	}
	if ppc.configFilePath != "" {
		// Read config file.
		log.Debug("Preparing to read the config file", ppc.configFilePath)
		vConfig, err := project.ReadConfigFile(ppc.configFilePath, project.YAML)
		if err != nil {
			return err
		}
		deployerParams, err := project.GetRepoConfigByPrefix(ppc.configFilePath, project.ProjectConfigDeployerPrefix, vConfig)
		if err != nil {
			return err
		}
		rtDetails, err := deployerParams.ServerDetails()
		if err != nil {
			return errorutils.CheckError(err)
		}
		ppc.SetBuildConfiguration(buildConfiguration).SetRepo(deployerParams.TargetRepo()).SetPnpmArgs(filteredPnpmArgs).SetServerDetails(rtDetails)
	}
	ppc.SetDetailedSummary(detailedSummary).SetXrayScan(xrayScan).SetScanOutputFormat(scanOutputFormat).SetDistTag(tag).SetUseNative(useNative)
	return nil
}

func (ppc *PnpmPublishCommand) Run() (err error) {
	log.Info("Running pnpm Publish")
	err = ppc.preparePrerequisites()
	if err != nil {
		return err
	}

	var pnpmBuild *build.Build
	var buildName, buildNumber, projectKey string
	if ppc.collectBuildInfo {
		buildName, err = ppc.buildConfiguration.GetBuildName()
		if err != nil {
			return err
		}
		buildNumber, err = ppc.buildConfiguration.GetBuildNumber()
		if err != nil {
			return err
		}
		projectKey = ppc.buildConfiguration.GetProject()
		buildInfoService := buildUtils.CreateBuildInfoService()
		pnpmBuild, err = buildInfoService.GetOrCreateBuildWithProject(buildName, buildNumber, projectKey)
		if err != nil {
			return errorutils.CheckError(err)
		}
	}

	if !ppc.tarballProvided {
		if err = ppc.pack(); err != nil {
			return err
		}
	}

	publishStrategy := NewPnpmPublishStrategy(ppc.UseNative(), ppc)

	err = publishStrategy.Publish()
	if err != nil {
		if ppc.tarballProvided {
			return err
		}
		// We should delete the tarball we created
		return errors.Join(err, deleteCreatedTarball(ppc.packedFilePaths))
	}

	if !ppc.tarballProvided {
		if err = deleteCreatedTarball(ppc.packedFilePaths); err != nil {
			return err
		}
	}

	if !ppc.collectBuildInfo {
		log.Info("pnpm publish finished successfully.")
		return nil
	}

	// Use PnpmModule to run pnpm commands
	pnpmModule, err := pnpmBuild.AddPnpmModule("")
	if err != nil {
		return errorutils.CheckError(err)
	}
	if ppc.buildConfiguration.GetModule() != "" {
		pnpmModule.SetName(ppc.buildConfiguration.GetModule())
	}

	buildArtifacts := publishStrategy.GetBuildArtifacts()
	for _, artifactReader := range ppc.artifactsDetailsReader {
		gofrogcmd.Close(artifactReader, &err)
	}
	err = pnpmModule.AddArtifacts(buildArtifacts...)
	if err != nil {
		return errorutils.CheckError(err)
	}

	log.Info("pnpm publish finished successfully.")
	return nil
}

func (ppc *PnpmPublishCommand) CommandName() string {
	return ppc.commandName
}

func (ppc *PnpmPublishCommand) preparePrerequisites() error {
	ppc.packedFilePaths = make([]string, 0)
	currentDir, err := os.Getwd()
	if err != nil {
		return errorutils.CheckError(err)
	}

	currentDir, err = filepath.Abs(currentDir)
	if err != nil {
		return errorutils.CheckError(err)
	}

	ppc.workingDirectory = currentDir
	log.Debug("Working directory set to:", ppc.workingDirectory)
	ppc.collectBuildInfo, err = ppc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if err = ppc.setPublishPath(); err != nil {
		return err
	}

	artDetails, err := ppc.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return err
	}

	if err = utils.ValidateRepoExists(ppc.repo, artDetails); err != nil {
		return err
	}

	return ppc.setPackageInfo()
}

func (ppc *PnpmPublishCommand) pack() error {
	log.Debug("Creating pnpm package.")
	packedFileNames, err := Pack(ppc.pnpmArgs, ppc.executablePath)
	if err != nil {
		return err
	}

	tarballDir, err := ppc.getTarballDir()
	if err != nil {
		return err
	}

	for _, packageFileName := range packedFileNames {
		ppc.packedFilePaths = append(ppc.packedFilePaths, filepath.Join(tarballDir, packageFileName))
	}

	return nil
}

func (ppc *PnpmPublishCommand) getTarballDir() (string, error) {
	if ppc.pnpmVersion == nil || ppc.pnpmVersion.Compare(packDestinationPnpmMinVersion) > 0 {
		return ppc.workingDirectory, nil
	}

	// Extract pack destination argument from the args.
	flagIndex, _, dest, err := coreutils.FindFlag("--pack-destination", ppc.pnpmArgs)
	if err != nil || flagIndex == -1 {
		return ppc.workingDirectory, err
	}
	return dest, nil
}

func (ppc *PnpmPublishCommand) setPublishPath() error {
	log.Debug("Reading Package Json.")

	ppc.publishPath = ppc.workingDirectory
	if len(ppc.pnpmArgs) > 0 && !strings.HasPrefix(strings.TrimSpace(ppc.pnpmArgs[0]), "-") {
		path := strings.TrimSpace(ppc.pnpmArgs[0])
		path = clientutils.ReplaceTildeWithUserHome(path)

		if filepath.IsAbs(path) {
			ppc.publishPath = path
		} else {
			ppc.publishPath = filepath.Join(ppc.workingDirectory, path)
		}
	}
	return nil
}

func (ppc *PnpmPublishCommand) setPackageInfo() error {
	log.Debug("Setting Package Info.")
	fileInfo, err := os.Stat(ppc.publishPath)
	if err != nil {
		return errorutils.CheckError(err)
	}

	if fileInfo.IsDir() {
		ppc.packageInfo, err = biutils.ReadPackageInfoFromPackageJsonIfExists(ppc.publishPath, ppc.pnpmVersion)
		return err
	}
	log.Debug("The provided path is not a directory, we assume this is a compressed pnpm package")
	ppc.tarballProvided = true
	// Sets the location of the provided tarball
	ppc.packedFilePaths = []string{ppc.publishPath}
	return ppc.readPackageInfoFromTarball(ppc.publishPath)
}

func (ppc *PnpmPublishCommand) readPackageInfoFromTarball(packedFilePath string) (err error) {
	log.Debug("Extracting info from pnpm package:", packedFilePath)
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

	// First pass: Collect all package.json files and validate their content
	var standardLocation *packageJsonInfo
	var rootLevelLocations []*packageJsonInfo
	var otherLocations []*packageJsonInfo

	tarReader := tar.NewReader(gZipReader)
	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errorutils.CheckError(err)
		}

		// Skip files that don't end with package.json
		if !strings.HasSuffix(hdr.Name, "package.json") {
			continue
		}

		// Skip macOS resource fork files (._package.json)
		baseName := filepath.Base(hdr.Name)
		if strings.HasPrefix(baseName, "._") {
			continue
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return errorutils.CheckError(err)
		}

		// Validate JSON before storing
		if err := validatePackageJson(content); err != nil {
			log.Debug("Invalid package.json found at", hdr.Name+":", err.Error())
			continue
		}

		info := &packageJsonInfo{
			path:    hdr.Name,
			content: content,
		}

		// Categorize based on location
		switch {
		case hdr.Name == "package/package.json":
			standardLocation = info
		case strings.Count(hdr.Name, "/") == 1:
			rootLevelLocations = append(rootLevelLocations, info)
		default:
			otherLocations = append(otherLocations, info)
		}
	}

	// Use package.json based on priority
	switch {
	case standardLocation != nil:
		log.Debug("Found package.json in standard location:", standardLocation.path)
		ppc.packageInfo, err = biutils.ReadPackageInfo(standardLocation.content, ppc.pnpmVersion)
		return err

	case len(rootLevelLocations) > 0:
		if len(rootLevelLocations) > 1 {
			log.Debug("Found multiple package.json files in root-level directories:", formatPaths(rootLevelLocations))
			log.Debug("Using first found:", rootLevelLocations[0].path)
		} else {
			log.Debug("Using package.json found in root-level directory:", rootLevelLocations[0].path)
		}
		ppc.packageInfo, err = biutils.ReadPackageInfo(rootLevelLocations[0].content, ppc.pnpmVersion)
		return err

	case len(otherLocations) > 0:
		if len(otherLocations) > 1 {
			log.Debug("Found multiple package.json files in non-standard locations:", formatPaths(otherLocations))
			log.Debug("Using first found:", otherLocations[0].path)
		} else {
			log.Debug("Found package.json in non-standard location:", otherLocations[0].path)
		}
		ppc.packageInfo, err = biutils.ReadPackageInfo(otherLocations[0].content, ppc.pnpmVersion)
		return err
	}

	return errorutils.CheckError(errors.New("Could not find valid 'package.json' in the compressed pnpm package: " + packedFilePath))
}

// validatePackageJson checks if the content is valid JSON and has required npm fields
func validatePackageJson(content []byte) error {
	var packageJson map[string]interface{}
	if err := json.Unmarshal(content, &packageJson); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	// Check required fields
	name, hasName := packageJson["name"].(string)
	version, hasVersion := packageJson["version"].(string)

	if !hasName || name == "" {
		return fmt.Errorf("missing or empty 'name' field")
	}
	if !hasVersion || version == "" {
		return fmt.Errorf("missing or empty 'version' field")
	}

	return nil
}

// formatPaths returns a formatted string of package.json paths for logging
func formatPaths(infos []*packageJsonInfo) string {
	paths := make([]string, len(infos))
	for i, info := range infos {
		paths[i] = info.path
	}
	return strings.Join(paths, ", ")
}

func deleteCreatedTarball(packedFilesPath []string) error {
	for _, packedFilePath := range packedFilesPath {
		if err := os.Remove(packedFilePath); err != nil {
			return errorutils.CheckError(err)
		}
		log.Debug("Successfully deleted the created pnpm package:", packedFilePath)
	}
	return nil
}

func (ppc *PnpmPublishCommand) getBuildPropsForArtifact() (string, error) {
	buildName, err := ppc.buildConfiguration.GetBuildName()
	if err != nil {
		return "", err
	}
	buildNumber, err := ppc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return "", err
	}
	err = buildUtils.SaveBuildGeneralDetails(buildName, buildNumber, ppc.buildConfiguration.GetProject())
	if err != nil {
		return "", err
	}
	return buildUtils.CreateBuildProperties(buildName, buildNumber, ppc.buildConfiguration.GetProject())
}

// Pack runs 'pnpm pack' and returns the list of created tarball filenames.
func Pack(pnpmArgs []string, executablePath string) ([]string, error) {
	packArgs := append([]string{"pack"}, pnpmArgs...)
	packCmd := gofrogcmd.NewCommand(executablePath, "", packArgs)
	output, err := packCmd.RunWithOutput()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}

	// Parse output to get the tarball filename(s)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var packedFiles []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".tgz") {
			packedFiles = append(packedFiles, line)
		}
	}

	if len(packedFiles) == 0 {
		return nil, errorutils.CheckErrorf("failed to find packed tarball in pnpm pack output")
	}

	return packedFiles, nil
}
