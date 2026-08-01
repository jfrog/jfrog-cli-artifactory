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
	apmcommon "github.com/jfrog/jfrog-cli-artifactory/agent/apm/common"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/golang"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
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
)

// packageManagerConfig describes the configuration `jf setup` writes for one package
// manager, so the command can say what it changed and how far the change reaches.
// Saying so matters because nothing else in the output reveals that the change is not
// scoped to the directory the command was run in.
type packageManagerConfig struct {
	// location names the configuration in the user's terms rather than as an absolute
	// path: the real path is platform dependent (pip alone has three per-user
	// locations) and the package manager resolves it itself, so a path spelled out
	// here would be wrong on some machines.
	location string
	// credentialsOnly marks the package managers that only store credentials instead
	// of redirecting resolution. Their projects do not start resolving through
	// Artifactory — an unqualified `docker pull alpine` still goes to Docker Hub — so
	// the note must not claim otherwise.
	credentialsOnly bool
	// overrideEnv is the environment variable that moves this configuration off its
	// user-level default. When it is set the configuration can live anywhere the user
	// pointed it, including inside the current project, so the note has to describe
	// that path instead of claiming user-wide scope. Only set where the configure
	// function or the tool it drives really honors the variable — the per-entry
	// comments record what was verified.
	overrideEnv string
}

// One entry per package manager in packageManagerToRepositoryPackageType;
// TestPackageManagerConfigs_CoversEverySupportedPackageManager asserts the two
// stay in step.
var packageManagerConfigs = map[project.ProjectType]packageManagerConfig{
	// npm resolves the file `npm config set` writes through NPM_CONFIG_USERCONFIG.
	project.Npm: {location: "your user-level npm configuration (.npmrc)", overrideEnv: "NPM_CONFIG_USERCONFIG"},
	// pnpm is deliberately left without an override: `pnpm config set` writes to
	// pnpm's own config directory (auth.ini) and ignores NPM_CONFIG_USERCONFIG, which
	// it consults only as a fallback when reading credentials.
	// That file also outranks ~/.npmrc for pnpm, which matters across two setups: a
	// machine with no pnpm setup inherits npm's ~/.npmrc through the fallback, but
	// once `jf setup pnpm` has written auth.ini, a later `jf setup npm` moves npm
	// alone and pnpm keeps resolving from the repository it was given. Both tools are
	// individually correct, and nothing in either command's output says they now
	// disagree. Verified against pnpm 11.
	project.Pnpm: {location: "your user-level pnpm configuration (auth.ini in pnpm's config directory)"},
	// Yarn Classic writes ~/.yarnrc, and YARN_RC_FILENAME does not redirect it.
	project.Yarn: {location: "your user-level Yarn configuration (.yarnrc)"},
	// configurePip writes the file itself when PIP_CONFIG_FILE is set, because
	// `pip config set` does not support that variable.
	project.Pip:    {location: "your user-level pip configuration (pip.conf)", overrideEnv: "PIP_CONFIG_FILE"},
	project.Pipenv: {location: "your user-level pip configuration (pip.conf)", overrideEnv: "PIP_CONFIG_FILE"},
	// `poetry config` writes config.toml into POETRY_CONFIG_DIR when it is set.
	project.Poetry: {location: "your user-level Poetry configuration (config.toml)", overrideEnv: "POETRY_CONFIG_DIR"},
	// Twine's .pypirc path is chosen per invocation (--config-file), not by the environment.
	project.Twine: {location: "your user-level Twine configuration (.pypirc)"},
	// ConfigureUVIndex writes to UV_CONFIG_FILE when it is set.
	project.UV:     {location: "your user-level uv configuration (uv.toml)", overrideEnv: "UV_CONFIG_FILE"},
	project.Nuget:  {location: "your user-level NuGet configuration (NuGet.Config)"},
	project.Dotnet: {location: "your user-level NuGet configuration (NuGet.Config)"},
	// `go env -w` writes to the file GOENV points at, defaulting to the per-user Go env file.
	project.Go: {location: "your user-level Go environment (GOPROXY in your Go env file)", overrideEnv: "GOENV"},
	// gradle.WriteInitScript drops the script under GRADLE_USER_HOME when it is set.
	project.Gradle: {location: "your user-level Gradle configuration (an init script in your Gradle user home)", overrideEnv: gradle.UserHomeEnv},
	// Maven picks the settings file per invocation (-s), not from the environment.
	project.Maven:  {location: "your user-level Maven settings (settings.xml)"},
	project.Docker: {location: "your Docker credential store", credentialsOnly: true},
	project.Podman: {location: "your Podman credential store", credentialsOnly: true},
	project.Helm:   {location: "your Helm registry credential store", credentialsOnly: true},
}

// configScopeNote describes what the command changed and how widely it applies, or
// an empty string for a package manager we have nothing accurate to say about.
func configScopeNote(packageManager project.ProjectType) string {
	packageManagerConfig, ok := packageManagerConfigs[packageManager]
	if !ok {
		return ""
	}
	if packageManagerConfig.credentialsOnly {
		return fmt.Sprintf("Credentials were saved to %s for your user account.", packageManagerConfig.location)
	}
	// A redirected configuration is not user-level, so report where it actually went
	// rather than promising a scope that may not hold.
	if packageManagerConfig.overrideEnv != "" {
		if overridePath := os.Getenv(packageManagerConfig.overrideEnv); overridePath != "" {
			return fmt.Sprintf("This updated the %s configuration at %s, because %s is set, so its scope follows that path rather than your user-level configuration.",
				packageManager.String(), overridePath, packageManagerConfig.overrideEnv)
		}
	}
	return fmt.Sprintf("This updated %s, so it applies to every %s project for this user, not only the current directory.",
		packageManagerConfig.location, packageManager.String())
}

// packageManagerToRepositoryPackageType maps project types to corresponding Artifactory repository package types.
var packageManagerToRepositoryPackageType = map[project.ProjectType]string{
	// Npm package managers
	project.Npm:  repository.Npm,
	project.Pnpm: repository.Npm,
	project.Yarn: repository.Npm,

	// Python (pypi) package managers
	project.Pip:      repository.Pypi,
	project.Pipenv:   repository.Pypi,
	project.Poetry:   repository.Pypi,
	project.Twine:    repository.Pypi,
	project.UV:       repository.Pypi,
	project.AgentApm: repository.AgentPackages,

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
	case project.AgentApm:
		err = sc.configureAgentApm()
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
	if note := configScopeNote(sc.packageManager); note != "" {
		log.Output(note)
	}
	return nil
}

// promptUserToSelectRepository prompts the user to select a compatible repository - virtual for
// every package manager except AgentApm, which is local-only (agentpackages has no remote/virtual
// support in Artifactory at all, so a virtual-repo search can never find a match for it).
func (sc *SetupCommand) promptUserToSelectRepository() (err error) {
	repoType := utils.Virtual
	if sc.packageManager == project.AgentApm {
		repoType = utils.Local
	}
	repoFilterParams := services.RepositoriesFilterParams{
		RepoType:    repoType.String(),
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

// goProxySeparators are the two characters that delimit GOPROXY entries: a comma
// falls through only on 404/410, a pipe on any error.
const goProxySeparators = ",|"

// maskGoProxyCredentials replaces the credentials of every entry in a GOPROXY
// value, keeping the scheme and host so the message still says which proxy is set.
//
// GOPROXY is a separator-delimited list, so masking only up to the first '@'
// would print every later entry's token verbatim. Within one entry the LAST '@'
// delimits the credentials, because a password may itself contain '@'.
func maskGoProxyCredentials(goProxy string) string {
	var masked strings.Builder
	entryStart := 0
	for i, char := range goProxy {
		if !strings.ContainsRune(goProxySeparators, char) {
			continue
		}
		masked.WriteString(maskGoProxyEntry(goProxy[entryStart:i]))
		masked.WriteRune(char)
		entryStart = i + len(string(char))
	}
	masked.WriteString(maskGoProxyEntry(goProxy[entryStart:]))
	return masked.String()
}

// maskGoProxyEntry masks the credentials of a single GOPROXY entry. Entries
// without credentials — including the bare `direct` and `off` keywords — are
// returned unchanged.
func maskGoProxyEntry(entry string) string {
	credentialsEnd := strings.LastIndex(entry, "@")
	if credentialsEnd == -1 {
		return entry
	}
	scheme := ""
	if schemeEnd := strings.Index(entry, "://"); schemeEnd != -1 && schemeEnd < credentialsEnd {
		scheme = entry[:schemeEnd+len("://")]
	}
	return scheme + "****" + entry[credentialsEnd:]
}

// configureGo configures Go to use the Artifactory repository for GOPROXY.
// Runs the following command:
//
//	go env -w GOPROXY=https://<user>:<token>@<your-artifactory-url>/artifactory/go/<repo-name>,direct
//
// The comma is deliberate. Unlike `jf go`, which resolves through the CLI for a
// single invocation and exposes --no-fallback, this writes a persistent global
// GOPROXY consumed by the native go command with no opt-out. A comma limits the
// fallback to 404/410 (module not proxied); a pipe would fall through on ANY
// error, so a 403 from Artifactory Curation would be silently satisfied from the
// module's public source.
func (sc *SetupCommand) configureGo() error {
	if goProxyVal := os.Getenv("GOPROXY"); goProxyVal != "" {
		// Remove the variable so it won't override the newly configured proxy (temporarily).
		if err := os.Unsetenv("GOPROXY"); err != nil {
			return errorutils.CheckErrorf("failed to unset GOPROXY environment variable: %w", err)
		}
		// Log a warning about the existing GOPROXY environment variable so the user can unset it permanently
		log.Warn(fmt.Sprintf("A local GOPROXY='%s' is set and will override the global setting.\n"+
			"Unset it in your shell config (e.g., .zshrc, .bashrc).", maskGoProxyCredentials(goProxyVal)))
	}
	repoWithCredsUrl, err := golang.GetArtifactoryRemoteRepoUrl(sc.serverDetails, sc.repoName,
		golang.GoProxyUrlParams{Direct: true, FallbackOnlyIfNotFound: true})
	if err != nil {
		return fmt.Errorf("failed to get Go repository URL: %w", err)
	}
	if err := biutils.RunGo([]string{"env", "-w", "GOPROXY=" + repoWithCredsUrl}, ""); err != nil {
		return fmt.Errorf("failed to set GOPROXY environment variable: %w", err)
	}
	// This is a behavior change worth surfacing: previously any proxy error fell
	// back to the module's public source, so an unreachable Artifactory still
	// produced a working build.
	log.Info("GOPROXY falls back to the module's source only for modules the repository does not serve (404/410). " +
		"Any other error, including a Curation block or an unreachable Artifactory, now fails the command instead of " +
		"resolving from the public internet.")
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

// configureAgentApm persistently configures the APM (Agent Package Manager) global config
// (~/.apm/config.json) to authenticate against the specified Artifactory agentpackages repository.
// This is the only APM operation that writes to the real home directory; all other APM commands
// use a temporary HOME to avoid persistent side-effects.
func (sc *SetupCommand) configureAgentApm() error {
	if err := apmcommon.ValidateApmPrerequisites(); err != nil {
		return err
	}
	return apmcommon.ConfigureApmRegistryPersistent(sc.serverDetails, sc.repoName)
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
