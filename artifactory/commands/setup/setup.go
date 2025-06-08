package setup

import (
	_ "embed"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	bidotnet "github.com/jfrog/build-info-go/build/utils/dotnet"
	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/golang"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/container"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/npm"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/yarn"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"golang.org/x/exp/maps"
)

// packageManagerToRepositoryPackageType maps project types to corresponding Artifactory repository package types.
var packageManagerToRepositoryPackageType = map[project.ProjectType]string{
	// Npm package managers
	project.Npm:  repository.Npm,
	project.Pnpm: repository.Npm,
	project.Yarn: repository.Npm,

	// Python (pypi) package managers
	project.Pip:    repository.Pypi,
	project.Pipenv: repository.Pypi,
	project.Poetry: repository.Pypi,
	project.Twine:  repository.Pypi,

	// Nuget package managers
	project.Nuget:  repository.Nuget,
	project.Dotnet: repository.Nuget,

	// Docker package managers
	project.Docker: repository.Docker,
	project.Podman: repository.Docker,

	project.Helm: repository.Helm,

	project.Go: repository.Go,

	project.Gradle: repository.Gradle,
	project.Maven:  repository.Maven,
}

// SetupCommand configures registries and authentication for various package manager (npm, Yarn, Pip, Pipenv, Poetry, Go)
type SetupCommand struct {
	// packageManager represents the type of package manager (e.g., NPM, Yarn).
	packageManager project.ProjectType
	// repoName is the name of the repository used for configuration.
	repoName string
	// projectKey is the JFrog Project key in JFrog Platform.
	projectKey string
	// serverDetails contains Artifactory server configuration.
	serverDetails *config.ServerDetails
	// commandName specifies the command for this instance.
	commandName string
}

// NewSetupCommand initializes a new SetupCommand for the specified package manager
func NewSetupCommand(packageManager project.ProjectType) *SetupCommand {
	return &SetupCommand{
		packageManager: packageManager,
		commandName:    "setup_" + packageManager.String(),
	}
}

// GetSupportedPackageManagersList returns a sorted list of supported package manager names as strings.
func GetSupportedPackageManagersList() []string {
	allSupportedPackageManagers := maps.Keys(packageManagerToRepositoryPackageType)
	// Sort keys based on their natural enum order
	slices.SortFunc(allSupportedPackageManagers, func(a, b project.ProjectType) int {
		return int(a) - int(b)
	})
	// Convert enums to their string representation
	result := make([]string, len(allSupportedPackageManagers))
	for i, manager := range allSupportedPackageManagers {
		result[i] = manager.String()
	}
	return result
}

func IsSupportedPackageManager(packageManager project.ProjectType) bool {
	_, exists := packageManagerToRepositoryPackageType[packageManager]
	return exists
}

// CommandName returns the name of the login command.
func (sc *SetupCommand) CommandName() string {
	return sc.commandName
}

// SetServerDetails assigns the server configuration details to the command.
func (sc *SetupCommand) SetServerDetails(serverDetails *config.ServerDetails) *SetupCommand {
	sc.serverDetails = serverDetails
	return sc
}

// ServerDetails returns the stored server configuration details.
func (sc *SetupCommand) ServerDetails() (*config.ServerDetails, error) {
	return sc.serverDetails, nil
}

// SetRepoName assigns the repository name to the command.
func (sc *SetupCommand) SetRepoName(repoName string) *SetupCommand {
	sc.repoName = repoName
	return sc
}

// SetProjectKey assigns the project key to the command.
func (sc *SetupCommand) SetProjectKey(projectKey string) *SetupCommand {
	sc.projectKey = projectKey
	return sc
}

// Run executes the configuration method corresponding to the package manager specified for the command.
func (sc *SetupCommand) Run() (err error) {
	if !IsSupportedPackageManager(sc.packageManager) {
		return errorutils.CheckErrorf("unsupported package manager: %s", sc.packageManager)
	}

	// If the repository name is not provided, and the package manager is not Docker or Podman, prompt the user to select a repository.
	// Docker and Podman do not require a repository name as they authenticate directly with the platform and require the repository name as part of the image name.
	if sc.repoName == "" && sc.packageManager != project.Docker && sc.packageManager != project.Podman {
		// Prompt the user to select a virtual repository that matches the package manager.
		if err = sc.promptUserToSelectRepository(); err != nil {
			return err
		}
	}

	// Configure the appropriate package manager based on the package manager.
	switch sc.packageManager {
	case project.Npm, project.Pnpm:
		err = sc.configureNpmPnpm()
	case project.Yarn:
		err = sc.configureYarn()
	case project.Pip, project.Pipenv:
		err = sc.configurePip()
	case project.Poetry:
		err = sc.configurePoetry()
	case project.Twine:
		err = sc.configureTwine()
	case project.Go:
		err = sc.configureGo()
	case project.Nuget, project.Dotnet:
		err = sc.configureDotnetNuget()
	case project.Docker, project.Podman:
		err = sc.configureContainer()
	case project.Helm:
		err = sc.configureHelm()
	case project.Gradle:
		err = sc.configureGradle()
	case project.Maven:
		err = sc.configureMaven()
	default:
		err = errorutils.CheckErrorf("unsupported package manager: %s", sc.packageManager)
	}
	if err != nil {
		return fmt.Errorf("failed to configure %s: %w", sc.packageManager.String(), err)
	}
	repoPrefix := ""
	if sc.packageManager != project.Docker && sc.packageManager != project.Podman {
		repoPrefix = coreutils.PrintBoldTitle(fmt.Sprintf(" repository '%s'", sc.repoName))
	}
	log.Output(fmt.Sprintf("Successfully configured %s to use JFrog%s.", coreutils.PrintBoldTitle(sc.packageManager.String()), repoPrefix))
	return nil
}

// promptUserToSelectRepository prompts the user to select a compatible virtual repository.
func (sc *SetupCommand) promptUserToSelectRepository() (err error) {
	repoFilterParams := services.RepositoriesFilterParams{
		RepoType:    utils.Virtual.String(),
		PackageType: packageManagerToRepositoryPackageType[sc.packageManager],
		ProjectKey:  sc.projectKey,
	}

	// Prompt for repository selection based on filter parameters.
	sc.repoName, err = utils.SelectRepositoryInteractively(
		sc.serverDetails,
		repoFilterParams,
		fmt.Sprintf("To configure %s, we need you to select a %s repository:", repoFilterParams.PackageType, repoFilterParams.RepoType))

	return err
}

// configurePip sets the global index-url for pip and pipenv to use the Artifactory PyPI repository.
// Runs the following command:
//
//	pip config set global.index-url https://<user>:<token>@<your-artifactory-url>/artifactory/api/pypi/<repo-name>/simple
//
// Note: Custom configuration file can be set by setting the PIP_CONFIG_FILE environment variable.
func (sc *SetupCommand) configurePip() error {
	repoWithCredsUrl, err := python.GetPypiRepoUrl(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return err
	}
	// If PIP_CONFIG_FILE is set, write the configuration to the custom config file manually.
	// Using 'pip config set' native command is not supported together with PIP_CONFIG_FILE.
	if customPipConfigPath := os.Getenv("PIP_CONFIG_FILE"); customPipConfigPath != "" {
		return python.CreatePipConfigManually(customPipConfigPath, repoWithCredsUrl)
	}
	return python.RunConfigCommand(project.Pip, []string{"set", "global.index-url", repoWithCredsUrl})
}

// configurePoetry configures Poetry to use the specified repository and authentication credentials.
// Runs the following commands:
//
//	poetry config repositories.<repo-name> https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/simple
//	poetry config http-basic.<repo-name> <user> <password/token>
//
// Note: Custom configuration file can be set by setting the POETRY_CONFIG_DIR environment variable.
func (sc *SetupCommand) configurePoetry() error {
	repoUrl, username, password, err := python.GetPypiRepoUrlWithCredentials(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return err
	}
	return python.RunPoetryConfig(repoUrl.String(), username, password, sc.repoName)
}

// configureTwine configures Twine to use the specified Artifactory PyPI repository.
// Creates or updates the .pypirc file in the user's home directory with the following structure:
//
// [distutils]
// index-servers =
//
//	pypi
//
// [pypi]
// repository = https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/
// username = <user>
// password = <token-or-password>
//
// Using the name "pypi" as the repository section makes it the default for Twine,
// allowing users to run `twine upload` without specifying a repository.
func (sc *SetupCommand) configureTwine() error {
	// Get the Artifactory PyPI repository URL and credentials.
	// The returned URL is intended for installs (ends with "/simple"),
	// but Twine requires the base repository URL for uploads.
	repoUrl, username, password, err := python.GetPypiRepoUrlWithCredentials(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return err
	}
	// Strip "/simple" to get the correct upload endpoint for Twine.
	trimmedUrl := strings.TrimSuffix(repoUrl.String(), "/simple")

	// Configure Twine using the .pypirc file
	return python.ConfigurePypirc(trimmedUrl, sc.repoName, username, password)
}

// configureNpmPnpm configures npm to use the Artifactory repository URL and sets authentication. Pnpm supports the same commands.
// Runs the following commands:
//
//	npm/pnpm config set registry https://<your-artifactory-url>/artifactory/api/npm/<repo-name>/
//
// For token-based auth:
//
//	npm/pnpm config set //your-artifactory-url/artifactory/api/npm/<repo-name>/:_authToken "<token>"
//
// For basic auth:
//
//	npm/pnpm config set //your-artifactory-url/artifactory/api/npm/<repo-name>/:_auth "<base64-encoded-username:password>"
//
// Note: Custom configuration file can be set by setting the NPM_CONFIG_USERCONFIG environment variable.
func (sc *SetupCommand) configureNpmPnpm() error {
	repoUrl := commandsutils.GetNpmRepositoryUrl(sc.repoName, sc.serverDetails.ArtifactoryUrl) + "/"
	if err := npm.ConfigSet(commandsutils.NpmConfigRegistryKey, repoUrl, sc.packageManager.String()); err != nil {
		return err
	}

	authKey, authValue := commandsutils.GetNpmAuthKeyValue(sc.serverDetails, repoUrl)
	if authKey != "" && authValue != "" {
		return npm.ConfigSet(authKey, authValue, sc.packageManager.String())
	}
	return nil
}

// configureYarn configures Yarn to use the specified Artifactory repository and sets authentication.
// Runs the following commands:
//
//	yarn config set registry https://<your-artifactory-url>/artifactory/api/npm/<repo-name>
//
// For token-based auth:
//
//	yarn config set //your-artifactory-url/artifactory/api/npm/<repo-name>/:_authToken "<token>"
//
// For basic auth:
//
//	yarn config set //your-artifactory-url/artifactory/api/npm/<repo-name>/:_auth "<base64-encoded-username:password>"
func (sc *SetupCommand) configureYarn() (err error) {
	repoUrl := commandsutils.GetNpmRepositoryUrl(sc.repoName, sc.serverDetails.ArtifactoryUrl)
	if err = yarn.ConfigSet(commandsutils.NpmConfigRegistryKey, repoUrl, "yarn", false); err != nil {
		return err
	}

	authKey, authValue := commandsutils.GetNpmAuthKeyValue(sc.serverDetails, repoUrl)
	if authKey != "" && authValue != "" {
		return yarn.ConfigSet(authKey, authValue, "yarn", false)
	}
	return nil
}

// configureGo configures Go to use the Artifactory repository for GOPROXY.
// Runs the following command:
//
//	go env -w GOPROXY=https://<user>:<token>@<your-artifactory-url>/artifactory/go/<repo-name>,direct
func (sc *SetupCommand) configureGo() error {
	if goProxyVal := os.Getenv("GOPROXY"); goProxyVal != "" {
		// Remove the variable so it won't override the newly configured proxy (temporarily).
		if err := os.Unsetenv("GOPROXY"); err != nil {
			return errorutils.CheckErrorf("failed to unset GOPROXY environment variable: %w", err)
		}
		// Mask credentials in the GOPROXY value
		if i := strings.Index(goProxyVal, "@"); i != -1 {
			goProxyVal = "****" + goProxyVal[i:]
		}
		// Log a warning about the existing GOPROXY environment variable so the user can unset it permanently
		log.Warn(fmt.Sprintf("A local GOPROXY='%s' is set and will override the global setting.\n"+
			"Unset it in your shell config (e.g., .zshrc, .bashrc).", goProxyVal))
	}
	repoWithCredsUrl, err := golang.GetArtifactoryRemoteRepoUrl(sc.serverDetails, sc.repoName, golang.GoProxyUrlParams{Direct: true})
	if err != nil {
		return err
	}
	return biutils.RunGo([]string{"env", "-w", "GOPROXY=" + repoWithCredsUrl}, "")
}

// configureDotnetNuget configures NuGet or .NET Core to use the specified Artifactory repository with credentials.
// Adds the repository source to the NuGet configuration file, using appropriate credentials for authentication.
// The following command is run for dotnet:
//
//	dotnet nuget add source --name <JFrog-Artifactory> "https://acme.jfrog.io/artifactory/api/nuget/{repository-name}" --username <your-username> --password <your-password>
//
// For NuGet:
//
//	nuget sources add -Name <JFrog-Artifactory> -Source "https://acme.jfrog.io/artifactory/api/nuget/{repository-name}" -Username <your-username> -Password <your-password>
func (sc *SetupCommand) configureDotnetNuget() error {
	// Retrieve repository URL and credentials for NuGet or .NET Core.
	sourceUrl, user, password, err := dotnet.GetSourceDetails(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return err
	}

	// Determine toolchain type based on the package manager
	toolchainType := bidotnet.DotnetCore
	if sc.packageManager == project.Nuget {
		toolchainType = bidotnet.Nuget
	}

	// Remove existing source if it exists
	if err = dotnet.RemoveSourceFromNugetConfigIfExists(toolchainType); err != nil {
		return err
	}

	// Add the repository as a source in the NuGet configuration with credentials for authentication
	return dotnet.AddSourceToNugetConfig(toolchainType, sourceUrl, user, password)
}

// configureContainer configures container managers like Docker or Podman to authenticate with JFrog Artifactory.
// It performs a login using the container manager's CLI command.
//
// For Docker:
//
//	echo <password> | docker login <artifactory-url-without-scheme> -u <username> --password-stdin
//
// For Podman:
//
//	echo <password> | podman login <artifactory-url-without-scheme> -u <username> --password-stdin
func (sc *SetupCommand) configureContainer() error {
	var containerManagerType container.ContainerManagerType
	switch sc.packageManager {
	case project.Docker:
		containerManagerType = container.DockerClient
	case project.Podman:
		containerManagerType = container.Podman
	default:
		return errorutils.CheckErrorf("unsupported container manager: %s", sc.packageManager)
	}
	// Parse the URL to remove the scheme (https:// or http://)
	parsedPlatformURL, err := url.Parse(sc.serverDetails.GetUrl())
	if err != nil {
		return err
	}
	urlWithoutScheme := parsedPlatformURL.Host + parsedPlatformURL.Path
	return container.ContainerManagerLogin(
		strings.TrimPrefix(urlWithoutScheme, "/"),
		&container.ContainerManagerLoginConfig{ServerDetails: sc.serverDetails},
		containerManagerType,
	)
}

// configureMaven updates the Maven settings.xml file to use the repo Url as mirror.
func (sc *SetupCommand) configureMaven() error {
	username := sc.serverDetails.GetUser()
	password := sc.serverDetails.GetPassword()

	// Get credentials from access-token if exists.
	if sc.serverDetails.GetAccessToken() != "" {
		if username == "" {
			username = auth.ExtractUsernameFromAccessToken(sc.serverDetails.GetAccessToken())
		}
		password = sc.serverDetails.GetAccessToken()
	}

	settingsXml, err := maven.NewSettingsXmlManager()
	if err != nil {
		return fmt.Errorf("failed to create a new Maven settings.xml manager: %w", err)
	}
	if err = settingsXml.ConfigureArtifactoryMirror(sc.serverDetails.GetArtifactoryUrl(), sc.repoName, username, password); err != nil {
		return fmt.Errorf("failed to update Artifactory mirror in Maven settings.xml: %w", err)
	}
	return nil
}

// configureGradle configures Gradle to use the specified Artifactory repository.
func (sc *SetupCommand) configureGradle() error {
	gradleBuildFile, err := gradle.CreateGradleBuildFile(sc.repoName, sc.serverDetails, sc.projectKey)
	if err != nil {
		return err
	}
	log.Output(fmt.Sprintf("Gradle build file created at: %s", gradleBuildFile))
	return nil
}

// configureHelm configures Helm to use Artifactory as an OCI registry.
// It executes:
//
//	helm registry login <registry-url> --username <user> --password-stdin
//
// If anonymous access is enabled for the repository, no login is performed.
func (sc *SetupCommand) configureHelm() error {
	// Parse the URL to get the registry domain without scheme or path
	parsedURL, err := url.Parse(sc.serverDetails.GetUrl())
	if err != nil {
		return err
	}
	// Use just the hostname part for OCI registry
	registryURL := parsedURL.Host

	// Prepare credentials
	user := sc.serverDetails.GetUser()
	pass := sc.serverDetails.GetPassword()
	if token := sc.serverDetails.GetAccessToken(); token != "" {
		if user == "" {
			user = auth.ExtractUsernameFromAccessToken(token)
		}
		pass = token
	}

	// If no credentials are provided, throw an error
	if user == "" && pass == "" {
		return errorutils.CheckErrorf("credentials are required for Helm registry login")
	}

	// Login to the Helm OCI registry
	cmdLogin := exec.Command("helm", "registry", "login", registryURL, "--username", user, "--password-stdin")

	// Pipe password to stdin
	cmdLogin.Stdin = strings.NewReader(pass)

	// Suppress success output, retain errors only
	cmdLogin.Stdout = io.Discard
	cmdLogin.Stderr = os.Stderr

	return cmdLogin.Run()
}
