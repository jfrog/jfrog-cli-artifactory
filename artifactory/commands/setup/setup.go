package setup

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	bidotnet "github.com/jfrog/build-info-go/build/utils/dotnet"
	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/golang"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ruby"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
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
	"gopkg.in/yaml.v3"
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
	project.UV:     repository.Pypi,

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

	project.Ruby: repository.Gems,
}

// SetupCommand configures registries and authentication for various package manager (npm, Yarn, Pip, Pipenv, Poetry, UV, Go)
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

// GetRepositoryPackageType gets the package type and returns the corresponding repository package type.
// For example, for pip or poetry the repository package type is "pypi".
func GetRepositoryPackageType(packageManager project.ProjectType) (string, error) {
	packageType, exists := packageManagerToRepositoryPackageType[packageManager]
	if !exists {
		return "", errorutils.CheckErrorf("unsupported package manager: %s", packageManager)
	}
	return packageType, nil
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
	case project.UV:
		err = sc.configureUV()
	case project.Ruby:
		err = sc.configureRuby()
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
		return fmt.Errorf("failed to get PyPI repository URL: %w", err)
	}
	// If PIP_CONFIG_FILE is set, write the configuration to the custom config file manually.
	// Using 'pip config set' native command is not supported together with PIP_CONFIG_FILE.
	if customPipConfigPath := os.Getenv("PIP_CONFIG_FILE"); customPipConfigPath != "" {
		if err := python.CreatePipConfigManually(customPipConfigPath, repoWithCredsUrl); err != nil {
			return fmt.Errorf("failed to create pip config file at %s: %w", customPipConfigPath, err)
		}
		return nil
	}
	if err := python.RunConfigCommand(project.Pip, []string{"set", "global.index-url", repoWithCredsUrl}); err != nil {
		return fmt.Errorf("failed to configure pip index-url: %w", err)
	}
	return nil
}

// configurePoetry configures Poetry to use the specified repository and authentication credentials.
// Runs the following commands:
//
//	poetry config repositories.<repo-name> https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/
//	poetry config http-basic.<repo-name> <user> <password/token>
//
// Note: The URL is set WITHOUT /simple suffix for publishing support.
// Resolution uses /simple (configured in pyproject.toml), but publishing requires the base URL.
// Custom configuration file can be set by setting the POETRY_CONFIG_DIR environment variable.
func (sc *SetupCommand) configurePoetry() error {
	repoUrl, username, password, err := python.GetPypiRepoUrlWithCredentials(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return fmt.Errorf("failed to get PyPI repository URL with credentials: %w", err)
	}
	// Strip "simple" and trailing slash from URL for publishing support (same as Twine)
	// Resolution URL (with /simple) should be configured in pyproject.toml
	// Publishing URL (without /simple) is configured in Poetry config
	publishUrl := strings.TrimSuffix(repoUrl.String(), "simple")
	publishUrl = strings.TrimSuffix(publishUrl, "/")
	if err := python.RunPoetryConfig(publishUrl, username, password, sc.repoName); err != nil {
		return fmt.Errorf("failed to configure Poetry repository: %w", err)
	}
	return nil
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
	// Strip "simple" from url as its not needed for Twine.
	trimmedUrl := strings.TrimSuffix(repoUrl.String(), "simple")

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
// Supports Yarn Classic (v1.x),  Yarn Berry (v2+) is project-specific
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
		return fmt.Errorf("failed to get Go repository URL: %w", err)
	}
	if err := biutils.RunGo([]string{"env", "-w", "GOPROXY=" + repoWithCredsUrl}, ""); err != nil {
		return fmt.Errorf("failed to set GOPROXY environment variable: %w", err)
	}
	return nil
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
	if err = dotnet.AddSourceToNugetConfig(toolchainType, sourceUrl, user, password); err != nil {
		return err
	}

	// Set source as the default push source to eliminate the need for --source flag
	return dotnet.SetDefaultPushSource(toolchainType)
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
	registryHost, err := deriveContainerRegistryHost(sc.serverDetails.GetArtifactoryUrl(), sc.serverDetails.GetUrl())
	if err != nil {
		return err
	}
	if err := container.ContainerManagerLogin(
		registryHost,
		&container.ContainerManagerLoginConfig{ServerDetails: sc.serverDetails},
		containerManagerType,
		false,
	); err != nil {
		return fmt.Errorf("failed to login to container registry: %w", err)
	}
	return nil
}

// deriveContainerRegistryHost returns the docker/podman registry hostname
// (no scheme, no path) for `docker login` / `podman login`.
//
// createServerDetailsFromFlags (jfrog-cli/utils/cliutils/utils.go) clears the
// platform Url for the Rt domain after copying it into ArtifactoryUrl, so on
// the --url path GetUrl() is empty and we must read GetArtifactoryUrl().
// GetUrl() IS populated on the --server-id path (loaded from saved config),
// so we fall back to it there. Returning an explicit error when both are
// empty avoids the historical failure mode where `docker login ""` was
// resolved by the daemon to Docker Hub and produced a misleading 401.
func deriveContainerRegistryHost(artifactoryUrl, platformUrl string) (string, error) {
	rawUrl := artifactoryUrl
	if rawUrl == "" {
		rawUrl = platformUrl
	}
	if rawUrl == "" {
		return "", errorutils.CheckErrorf("server URL is empty; provide --url or --server-id")
	}
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		return "", fmt.Errorf("failed to parse server URL: %w", err)
	}
	// url.Parse accepts scheme-less inputs (e.g. "acme.jfrog.io/artifactory") and
	// treats the whole string as Path with an empty Host. Surface a specific
	// error so users know to add http:// or https://.
	if parsedUrl.Scheme == "" {
		return "", errorutils.CheckErrorf("server URL %q is missing a scheme; expected http:// or https://", rawUrl)
	}
	if parsedUrl.Host == "" {
		return "", errorutils.CheckErrorf("server URL %q has no host component", rawUrl)
	}
	return parsedUrl.Host, nil
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
	if err = settingsXml.ConfigureArtifactoryRepository(sc.serverDetails.GetArtifactoryUrl(), sc.repoName, username, password); err != nil {
		return fmt.Errorf("failed to update Artifactory mirror in Maven settings.xml: %w", err)
	}
	return nil
}

// configureGradle configures Gradle to use the specified Artifactory repository for both dependency resolution and publishing.
func (sc *SetupCommand) configureGradle() error {
	password := sc.serverDetails.GetPassword()
	username := sc.serverDetails.GetUser()
	if sc.serverDetails.GetAccessToken() != "" {
		password = sc.serverDetails.GetAccessToken()
		username = auth.ExtractUsernameFromAccessToken(password)
	}
	initScriptAuthConfig := gradle.InitScriptAuthConfig{
		ArtifactoryURL:         sc.serverDetails.GetArtifactoryUrl(),
		GradleRepoName:         sc.repoName,
		ArtifactoryAccessToken: password,
		ArtifactoryUsername:    username,
	}
	initScript, err := gradle.GenerateInitScript(initScriptAuthConfig)
	if err != nil {
		return fmt.Errorf("failed to generate Gradle init script: %w", err)
	}

	if err := gradle.WriteInitScript(initScript); err != nil {
		return fmt.Errorf("failed to write Gradle init script: %w", err)
	}
	return nil
}

// configureUV configures UV to use the Artifactory PyPI repository.
// 1. Stores credentials via UV's native credential store:
//
//	uv auth login <artifactory-domain> --username <user> --password <token>
//
// 2. Writes a [[index]] entry and a global publish-url to the user-level uv.toml
// (~/.config/uv/uv.toml) pointing to the Artifactory PyPI repository:
//
//	publish-url = "https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>"
//
//	[[index]]
//	name = "jfrog-pypi"
//	url = "https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/simple"
//	default = true
func (sc *SetupCommand) configureUV() error {
	repoUrl, username, password, err := python.GetPypiRepoUrlWithCredentials(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return fmt.Errorf("failed to get PyPI repository URL with credentials: %w", err)
	}

	serviceURL := repoUrl.Scheme + "://" + repoUrl.Host

	// Best-effort: remove any stale credentials for this service URL before (re-)configuring.
	// Prevents leftover tokens from a previous setup from being used when switching to anonymous.
	_ = python.RunUVAuthLogout(serviceURL, username)

	// Store credentials only when authentication is configured (skip for anonymous access)
	if username != "" && password != "" {
		if err := python.RunUVAuthLogin(serviceURL, username, password); err != nil {
			return fmt.Errorf("failed to store UV credentials: %w", err)
		}
	}

	// Write the index entry to user-level uv.toml (URL without credentials)
	indexURL := repoUrl.String()
	if err := python.ConfigureUVIndex(indexURL); err != nil {
		return fmt.Errorf("failed to configure UV index: %w", err)
	}
	return nil
}

// rubygemsDefaultSource is the public source that RubyGems and Bundler use by default.
// It stays first in ~/.gemrc's :sources: list, and is the source mirrored to Artifactory
// so that unmodified Gemfiles resolve through Artifactory.
const rubygemsDefaultSource = "https://rubygems.org"

// configureRuby points RubyGems and Bundler at Artifactory, so that plain `gem` and
// `bundle` commands resolve and authenticate through it with no edit to the Gemfile.
//
// Everything is written by editing the config files directly, never by shelling out to
// `gem`/`bundle`, because their CLI syntax differs across versions (notably
// `bundle config set`, which does not exist before Bundler 2.0):
//
//  1. ~/.bundle/config — a mirror redirecting https://rubygems.org to the Artifactory
//     repository, plus per-host credentials.
//  2. ~/.gemrc — the Artifactory repository added to :sources:, for bare `gem install`.
func (sc *SetupCommand) configureRuby() error {
	repoUrl, username, password, err := ruby.GetRubyGemsRepoUrlWithCredentials(sc.serverDetails, sc.repoName)
	if err != nil {
		return fmt.Errorf("failed to get RubyGems repository URL with credentials: %w", err)
	}

	// sourceURL stays credential-free: it is what gets printed for the user to paste into
	// a shared Gemfile. authenticatedURL is the same repository with credentials embedded,
	// which is what the local config files need.
	sourceURL := repoUrl.String()
	authenticatedURL := sourceURL
	if password != "" {
		withCredentials := *repoUrl
		withCredentials.User = url.UserPassword(username, password)
		authenticatedURL = withCredentials.String()
	}
	settings := map[string]string{}

	// Mirror the public RubyGems source to Artifactory, so a Gemfile that says
	// `source "https://rubygems.org"` resolves through Artifactory unchanged. Credentials
	// are embedded in the mirror value: Bundler keeps a mirror URI's own userinfo instead
	// of looking credentials up separately, which behaves identically on every version.
	settings[bundleMirrorKey(rubygemsDefaultSource)] = authenticatedURL

	// Per-host credentials, for Gemfiles that name the Artifactory source explicitly.
	if password != "" {
		credential := username + ":" + password
		for _, key := range ruby.BundleCredentialKeys(repoUrl.Hostname()) {
			settings[key] = credential
		}
	}

	if bundleErr := writeBundleSettings(settings); bundleErr != nil {
		return fmt.Errorf("failed to configure Bundler: %w", bundleErr)
	}
	log.Info(fmt.Sprintf("Bundler configured: %s is mirrored to %s", rubygemsDefaultSource, sourceURL))

	if gemrcErr := addGemrcSource(authenticatedURL); gemrcErr != nil {
		return fmt.Errorf("failed to update ~/.gemrc: %w", gemrcErr)
	}
	log.Info("RubyGems configured: source added to ~/.gemrc")

	log.Output(fmt.Sprintf(
		"\nBundler and RubyGems now resolve through Artifactory.\n"+
			"  A Gemfile using `source \"%s\"` needs no change.\n"+
			"  To depend on this repository explicitly, use:\n      source \"%s\"\n",
		rubygemsDefaultSource, sourceURL))
	return nil
}

// bundleMirrorKey returns the ~/.bundle/config key Bundler reads a mirror from for the
// given upstream source. Bundler builds it from "mirror.<uri>" by normalizing the URI to
// a trailing slash, replacing "." with "__", and upcasing:
//
//	https://rubygems.org → BUNDLE_MIRROR__HTTPS://RUBYGEMS__ORG/
func bundleMirrorKey(sourceURL string) string {
	normalized := strings.TrimSuffix(sourceURL, "/") + "/"
	return "BUNDLE_" + strings.ToUpper(strings.ReplaceAll("mirror."+normalized, ".", "__"))
}

// writeBundleSettings merges entries into ~/.bundle/config, preserving every setting
// already present. The file holds credentials, so it is written 0600.
func writeBundleSettings(entries map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bundleDir := filepath.Join(home, ".bundle")
	configPath := filepath.Join(bundleDir, "config")

	existing, readErr := os.ReadFile(configPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}

	config := map[string]interface{}{}
	if len(existing) > 0 {
		if unmarshalErr := yaml.Unmarshal(existing, &config); unmarshalErr != nil {
			return fmt.Errorf("parse existing %s: %w", configPath, unmarshalErr)
		}
	}
	for key, value := range entries {
		config[key] = value
	}

	out, marshalErr := marshalBundleConfig(config)
	if marshalErr != nil {
		return marshalErr
	}
	if mkdirErr := os.MkdirAll(bundleDir, 0755); mkdirErr != nil {
		return mkdirErr
	}
	return os.WriteFile(configPath, out, 0600)
}

// marshalBundleConfig renders Bundler's config as YAML that Bundler's own parser accepts.
// Bundler reads this file with a line-based stub serializer rather than a real YAML
// parser: it needs each setting on a single line, and it measures nesting depth in
// two-space units.
func marshalBundleConfig(config map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(config); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// addGemrcSource adds sourceURL to ~/.gemrc's :sources: list, moving it to the front
// (behind rubygemsDefaultSource, if present) so `gem install` tries it first. Different
// repositories configured across separate runs are meant to coexist here, because
// `gem install` natively searches every listed source.
//
// sourceURL embeds credentials when the server has them: unlike Bundler, RubyGems has no
// separate credential store for installing, so the source URL is the only way a plain
// `gem install` can authenticate. That is why the file is written 0600.
func addGemrcSource(sourceURL string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	gemrcPath := filepath.Join(home, ".gemrc")

	existing, readErr := os.ReadFile(gemrcPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}

	config := map[string]interface{}{}
	if len(existing) > 0 {
		if unmarshalErr := yaml.Unmarshal(existing, &config); unmarshalErr != nil {
			return fmt.Errorf("parse existing %s: %w", gemrcPath, unmarshalErr)
		}
	}

	var currentSources []string
	if raw, ok := config[":sources"]; ok {
		if rawList, ok := raw.([]interface{}); ok {
			for _, item := range rawList {
				if s, ok := item.(string); ok {
					currentSources = append(currentSources, s)
				}
			}
		}
	}

	config[":sources"] = reorderGemrcSources(currentSources, sourceURL)

	out, marshalErr := yaml.Marshal(config)
	if marshalErr != nil {
		return marshalErr
	}
	// The source URL may embed credentials, so this file must not be world-readable.
	return os.WriteFile(gemrcPath, out, 0600)
}

// gemSourceIdentity strips embedded credentials and any trailing slash from a gem source
// URL, so that two entries pointing at the same repository compare equal even when their
// credentials differ. Without this, re-running setup after a token rotation would leave
// the stale entry behind and `gem install` would keep trying the old credentials.
func gemSourceIdentity(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.TrimSuffix(rawURL, "/")
	}
	parsed.User = nil
	return strings.TrimSuffix(parsed.String(), "/")
}

// reorderGemrcSources puts sourceURL first, replaces any existing entry for the same
// repository, and removes the public RubyGems source.
//
// Removing https://rubygems.org is deliberate. RubyGems queries sources in list order, so
// leaving the public source in front means `gem install` reaches rubygems.org before
// Artifactory and setup has no practical effect. Dropping it matches what the Bundler
// mirror already does, and what `jf setup` does for npm and cargo, which replace the
// public registry outright rather than racing it. The configured repository is expected
// to be virtual or remote-backed so it can still serve public gems.
//
// Artifactory repositories configured across separate runs still coexist, most recently
// configured first, because `gem install` genuinely does search several sources.
func reorderGemrcSources(sources []string, sourceURL string) []string {
	target := gemSourceIdentity(sourceURL)
	result := make([]string, 0, len(sources)+1)
	result = append(result, sourceURL)

	for _, s := range sources {
		identity := gemSourceIdentity(s)
		if identity == rubygemsDefaultSource || identity == target {
			continue
		}
		result = append(result, s)
	}
	return result
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
