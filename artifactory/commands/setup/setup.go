package setup

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	bidotnet "github.com/jfrog/build-info-go/build/utils/dotnet"
	biutils "github.com/jfrog/build-info-go/utils"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/cargo"
	aptcommand "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/apt"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/golang"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	container "github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ruby"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/permissions"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/npm"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/yarn"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
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
	project.UV:     {location: "your user-level uv configuration (uv.toml)", overrideEnv: python.UVConfigFileEnv},
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
	project.Apt:    {location: "your apt configuration"},
	project.Apk:    {location: "your apk configuration"},
	// configureRuby writes ~/.gemrc and ~/.bundle/config directly, always under the user's
	// home directory, and honours no override variable of its own.
	project.Ruby: {location: "your user-level RubyGems and Bundler configuration (.gemrc and .bundle/config)"},
}

// configScopeNote describes what the command changed and how widely it applies, or
// an empty string for a package manager we have nothing accurate to say about.
func configScopeNote(packageManager project.ProjectType) string {
	cfg, ok := packageManagerConfigs[packageManager]
	if !ok {
		return ""
	}
	if cfg.credentialsOnly {
		return fmt.Sprintf("Credentials were saved to %s for your user account.", cfg.location)
	}
	// A redirected configuration is not user-level, so report where it actually went
	// rather than promising a scope that may not hold.
	if cfg.overrideEnv != "" {
		if overridePath := os.Getenv(cfg.overrideEnv); overridePath != "" {
			return fmt.Sprintf("This updated the %s configuration at %s, because %s is set, so its scope follows that path rather than your user-level configuration.",
				packageManager.String(), overridePath, cfg.overrideEnv)
		}
	}
	return fmt.Sprintf("This updated %s, so it applies to every %s project for this user, not only the current directory.",
		cfg.location, packageManager.String())
}

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

	project.Apt: repository.Debian,

	project.Gradle: repository.Gradle,
	project.Maven:  repository.Maven,

	project.Cargo: repository.Cargo,

	project.Ruby: repository.Gems,

	project.Apk: repository.Alpine,
}

// SetupCommand configures registries and authentication for various package manager (npm, Yarn, Pip, Pipenv, Poetry, UV, Go)
type SetupCommand struct {
	// packageManager represents the type of package manager (e.g., NPM, Yarn).
	packageManager project.ProjectType
	// repoName is the name of the repository used for configuration.
	repoName string
	// deployRepoName is the name of the repository used for publishing/deploying, when the package
	// manager separates resolution and deployment repos (currently only Cargo, whose remote resolves
	// crates.io and whose local is the publish target). Empty for single-repo package managers.
	deployRepoName string
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
	// Alpine (Apk) handles its own repo-type-first interactive flow inside configureApk().
	if sc.repoName == "" && sc.packageManager != project.Docker && sc.packageManager != project.Podman && sc.packageManager != project.Apk {
		// Cargo has no virtual repositories and separates resolution (remote) from deployment
		// (local), so it selects both instead of a single virtual repo.
		if sc.packageManager == project.Cargo {
			if err = sc.promptUserToSelectCargoRepositories(); err != nil {
				return err
			}
		} else if err = sc.promptUserToSelectRepository(); err != nil {
			// Prompt the user to select a virtual repository that matches the package manager.
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
	case project.Cargo:
		err = sc.configureCargo()
	case project.Ruby:
		err = sc.configureRuby()
	case project.Apt:
		err = sc.configureApt()
	case project.Apk:
		err = sc.configureApk()
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

// noMatchingRepositoriesErrSubstring matches the error returned by utils.SelectRepositoryInteractively
// when no repository satisfies the filter (see jfrog-cli-core/artifactory/utils/repositoryutils.go).
// Used to detect that case and fall back to a manual repository name prompt, e.g. for Cargo, which
// Artifactory doesn't support as a virtual package type - a virtual-repo filter always returns zero results.
const noMatchingRepositoriesErrSubstring = "no repositories were found that match"

// promptUserToSelectRepository prompts the user to select a compatible virtual repository.
// If none is found, falls back to asking the user to type an existing repository name directly.
func (sc *SetupCommand) promptUserToSelectRepository() (err error) {
	return sc.promptUserToSelectRepositoryFiltered(utils.Virtual.String())
}

// promptUserToSelectRepositoryFiltered prompts for a repository of the given type
// (virtual/local/remote, or "" for any). Falls back to a manual-name prompt when no
// matching repository is found — needed for package managers whose projects may not
// have a matching Artifactory repo configured yet at setup time.
func (sc *SetupCommand) promptUserToSelectRepositoryFiltered(repoType string) (err error) {
	repoFilterParams := services.RepositoriesFilterParams{
		RepoType:    repoType,
		PackageType: packageManagerToRepositoryPackageType[sc.packageManager],
		ProjectKey:  sc.projectKey,
	}

	promptMessage := fmt.Sprintf("To configure %s, we need you to select a %s repository:", repoFilterParams.PackageType, repoFilterParams.RepoType)
	if repoType == "" {
		promptMessage = fmt.Sprintf("To configure %s, we need you to select a repository:", repoFilterParams.PackageType)
	}

	// Prompt for repository selection based on filter parameters.
	repoName, err := utils.SelectRepositoryInteractively(sc.serverDetails, repoFilterParams, promptMessage)
	if err == nil {
		sc.repoName = repoName
		return nil
	}
	if !strings.Contains(err.Error(), noMatchingRepositoriesErrSubstring) {
		return err
	}

	// No matching repository was found — fall back to asking the user to type an existing name.
	if repoType != "" {
		log.Info(fmt.Sprintf("No %s %s repository was found.", repoType, repoFilterParams.PackageType))
	} else {
		log.Info(fmt.Sprintf("No %s repository was found.", repoFilterParams.PackageType))
	}
	repoName = ioutils.AskString("", "Please enter the name of an existing repository to use", false, false)
	serviceDetails, err := sc.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return err
	}
	if err = utils.ValidateRepoExists(repoName, serviceDetails); err != nil {
		return err
	}
	sc.repoName = repoName
	return nil
}


// promptUserToSelectCargoRepositories selects the repositories Cargo needs when --repo is not
// given. Cargo has two orthogonal roles that map to two different Artifactory repo types:
//
//   - REMOTE (required)  — the *resolution / download* registry. Every crate cargo pulls in
//     (`cargo build`, `install`, `update`, `fetch`, transitive deps of `publish`, …) is downloaded
//     from here. crates.io is redirected onto this repo, so it is the sole source of third-party
//     dependencies. Downloads NEVER go through the local repo.
//   - LOCAL  (optional)  — the *publish / upload* registry. `cargo publish --registry jfrog-local`
//     uploads the user's own crate to this repo. It is upload-only. Skipping this prompt is a
//     valid "I don't publish from this machine" choice; nothing is written for publish and cargo
//     will refuse `cargo publish` until the user either re-runs `jf setup cargo` with a local
//     repo selected or adds a `[registries.…]` entry themselves.
//
// Artifactory has no virtual Cargo repositories, so — unlike the generic flow — this lists
// local/remote repos directly. The remote falls back to a manual name prompt when none exists.
func (sc *SetupCommand) promptUserToSelectCargoRepositories() error {
	packageType := packageManagerToRepositoryPackageType[sc.packageManager]

	// Resolution repository — a remote Cargo repo (proxies crates.io).
	remote, err := utils.SelectRepositoryInteractively(
		sc.serverDetails,
		services.RepositoriesFilterParams{RepoType: utils.Remote.String(), PackageType: packageType, ProjectKey: sc.projectKey},
		"To configure cargo, select a remote repository for resolving dependencies:")
	if err != nil {
		if !strings.Contains(err.Error(), noMatchingRepositoriesErrSubstring) {
			return err
		}
		log.Info(fmt.Sprintf("No remote %s repository was found.", packageType))
		remote = ioutils.AskString("", "Please enter the name of an existing repository to resolve dependencies from", false, false)
		serviceDetails, sErr := sc.serverDetails.CreateArtAuthConfig()
		if sErr != nil {
			return sErr
		}
		if vErr := utils.ValidateRepoExists(remote, serviceDetails); vErr != nil {
			return vErr
		}
	}
	sc.repoName = remote

	// Deployment repository — a local Cargo repo (publish target). Optional.
	// Ask up-front so the user can skip publishing even when local repos exist —
	// SelectRepositoryInteractively has no "none" entry and would otherwise force a choice.
	if !coreutils.AskYesNo("Configure a local repository for publishing crates?", false) {
		log.Info("Skipping publish configuration; configuring resolution only.")
		return nil
	}
	local, err := utils.SelectRepositoryInteractively(
		sc.serverDetails,
		services.RepositoriesFilterParams{RepoType: utils.Local.String(), PackageType: packageType, ProjectKey: sc.projectKey},
		"Select a local repository for publishing crates:")
	if err != nil {
		if !strings.Contains(err.Error(), noMatchingRepositoriesErrSubstring) {
			return err
		}
		log.Info(fmt.Sprintf("No local %s repository was found; configuring resolution only (publishing not configured).", packageType))
		return nil
	}
	sc.deployRepoName = local
	return nil
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
	// pip config set creates the file at 0644; harden to 0600 because index-url
	// embeds credentials. `pip config set` writes the user-level file, so the
	// derived path matches it without parsing pip's human-readable output.
	python.HardenPipConfigPermissions()
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
		if err := npm.ConfigSet(authKey, authValue, sc.packageManager.String()); err != nil {
			return err
		}
	}
	// npm writes ~/.npmrc at 0600 already, so only pnpm needs hardening here: it
	// stores the _authToken in auth.ini at 0644.
	if sc.packageManager == project.Pnpm {
		hardenPnpmAuthConfig()
	}
	return nil
}

// pnpmConfigFileNames are the files pnpm may write credentials into. pnpm stores
// the _authToken differently across versions (auth.ini in v9+, otherwise the
// rc/config file it reports as `globalconfig`), so all known names are hardened.
var pnpmConfigFileNames = []string{"auth.ini", "rc", "config.yaml", ".npmrc"}

// hardenPnpmAuthConfig best-effort restricts the pnpm config files that may hold
// the _authToken in cleartext at 0644 to owner-only.
func hardenPnpmAuthConfig() {
	for _, path := range pnpmCredentialFiles() {
		permissions.RestrictExisting(path)
	}
}

// pnpmCredentialFiles returns the existing pnpm config files (see
// pnpmConfigFileNames) in pnpm's own config directory. There is no first-party Go
// resolver for that directory, so it is derived from the file pnpm reports as
// `globalconfig`. This is best-effort: it returns nil (rather than surfacing an
// error) when pnpm cannot be queried or nothing was written (e.g. anonymous
// access), so a resolution miss never fails an otherwise-successful setup.
// Restricting a file without secrets is a harmless no-op.
func pnpmCredentialFiles() []string {
	out, err := exec.Command("pnpm", "config", "get", "globalconfig").Output()
	if err != nil {
		log.Warn("Could not resolve pnpm's config directory to restrict its permissions. " +
			"If it holds an access token, restrict it to owner-only access manually.")
		return nil
	}
	globalConfig := strings.TrimSpace(string(out))
	if globalConfig == "" {
		return nil
	}
	configDir := filepath.Dir(globalConfig)
	var existing []string
	for _, name := range pnpmConfigFileNames {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, path)
		}
	}
	return existing
}

// userFile joins parts onto the current user's home directory. jf setup uses it
// to locate the credential files other modules write there (~/.m2/settings.xml,
// ~/.yarnrc) so it can harden them afterwards.
func userFile(parts ...string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	return filepath.Join(append([]string{homeDir}, parts...)...), nil
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
		if err = yarn.ConfigSet(authKey, authValue, "yarn", false); err != nil {
			return err
		}
	}
	// Yarn Classic writes ~/.yarnrc (YARN_RC_FILENAME does not redirect it) with the
	// auth token in cleartext; restrict it to owner-only. Yarn Berry does not use
	// ~/.yarnrc, so RestrictExisting warns and moves on there.
	yarnrc, err := userFile(".yarnrc")
	if err != nil {
		return err
	}
	permissions.RestrictExisting(yarnrc)
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
	// GOPROXY embeds user:token@ in cleartext in the Go env file; restrict it to
	// owner-only. Best-effort: `go env -w` already succeeded, so a failure to
	// resolve or tighten the file must not fail an otherwise-configured setup.
	if goEnvPath, err := goEnvFilePath(); err != nil {
		log.Warn("Could not resolve the Go environment file to restrict its permissions: " + err.Error() +
			". If it holds credentials, restrict it to owner-only access manually.")
	} else {
		permissions.RestrictExisting(goEnvPath)
	}
	return nil
}

// goEnvFilePath returns the file `go env -w` persists to (honoring GOENV), which
// now holds the credential-bearing GOPROXY value. `go env GOENV` is the only
// authoritative source for this path: it applies the same GOENV/default
// resolution the write used.
func goEnvFilePath() (string, error) {
	out, err := exec.Command("go", "env", "GOENV").Output()
	if err != nil {
		return "", errorutils.CheckErrorf("failed to resolve the Go environment file path: %s", err.Error())
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", errorutils.CheckErrorf("`go env GOENV` returned an empty path")
	}
	return path, nil
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

	// NewSettingsXmlManager resolves this same ~/.m2/settings.xml path internally;
	// resolving it here too lets us harden the file afterwards, since settings.xml
	// stores the password/access token in cleartext.
	settingsXmlPath, err := userFile(".m2", "settings.xml")
	if err != nil {
		return err
	}
	settingsXml, err := maven.NewSettingsXmlManagerWithPath(settingsXmlPath)
	if err != nil {
		return fmt.Errorf("failed to create a new Maven settings.xml manager: %w", err)
	}
	if err = settingsXml.ConfigureArtifactoryRepository(sc.serverDetails.GetArtifactoryUrl(), sc.repoName, username, password); err != nil {
		return fmt.Errorf("failed to update Artifactory mirror in Maven settings.xml: %w", err)
	}
	permissions.RestrictExisting(settingsXmlPath)
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

	// WriteInitScript writes the token-bearing init script owner-only (0600) itself.
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
	// The URL must end in a slash. RubyGems resolves index files relative to the source, so
	// without one the final path segment is replaced and it requests
	// .../api/gems/specs.4.8.gz — losing the repository name — which makes a plain
	// `gem install` fail with "server did not return a valid file". Bundler normalises the
	// trailing slash itself, so this is equally correct for the mirror and the Gemfile.
	if !strings.HasSuffix(repoUrl.Path, "/") {
		repoUrl.Path += "/"
	}
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
	home, err := ruby.UserHomeDir()
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
	return permissions.WriteFileOwnerOnly(configPath, out)
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
	home, err := ruby.UserHomeDir()
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
	return permissions.WriteFileOwnerOnly(gemrcPath, out)
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

// configureCargo configures Cargo (Rust) to use the Artifactory repository for both dependency
// resolution and publishing, by writing the user-level cargo config and credentials files.
// It writes:
//
//	~/.cargo/config.toml      — [registries.jfrog] index, [registry] default, [source.crates-io] replace-with
//	~/.cargo/credentials.toml — [registries.jfrog] token
//
// These are cargo's OWN persistent config files: after `jf setup cargo` returns, plain `cargo`
// (not just `jf cargo`) reads them on every invocation. So `cargo build`, `cargo update`,
// `cargo fetch`, `cargo publish --registry jfrog-local`, `cargo install <crate>` etc. all resolve
// through Artifactory and authenticate with the written token — no `jf` prefix required. This is
// the persistent counterpart to per-run env-var injection in `jf cargo <cmd>`; the two are
// complementary (env vars win for jf-run commands, files apply everywhere else).
//
// Coverage lives in artifactory/commands/cargo/setup_test.go: TestConfigureNativeRegistry_*
// asserts config.toml + credentials.toml are written with the correct registries, that
// re-running is idempotent and preserves unrelated user keys, and that anonymous setup skips
// credentials. The end-to-end path (setup → native `cargo publish` → uploaded artifact + build
// info) is exercised against a live Artifactory tenant in the jfrog-cli integration suite.
func (sc *SetupCommand) configureCargo() error {
	return cargo.ConfigureNativeRegistry(sc.serverDetails, sc.repoName, sc.deployRepoName)
}

// configureApt interactively prompts for repo, dist, component, and GPG mode,
// then delegates to AptSetupCommand which writes the persistent sources.list entry and pinning file.
// sc.repoName was pre-selected by promptUserToSelectRepository; we let the user confirm or change it.
func (sc *SetupCommand) configureApt() error {
	// Show the auto-selected repo and let the user confirm or override.
	ioutils.ScanFromConsole("Repository name", &sc.repoName, sc.repoName)

	var dist string
	for dist == "" {
		ioutils.ScanFromConsole("Distribution name (e.g. noble, jammy, bookworm)", &dist, "")
	}

	var component string
	ioutils.ScanFromConsole("Component (e.g. main, contrib, non-free — leave empty for 'main')", &component, "main")

	var gpgChoice string
	ioutils.ScanFromConsole("GPG mode — 'import' (auto-fetch key), 'trusted' (skip GPG, for testing), or leave empty to skip", &gpgChoice, "")

	cmd := aptcommand.NewAptSetupCommand().
		SetServerDetails(sc.serverDetails).
		SetRepoName(sc.repoName).
		SetDist(dist).
		SetComponent(component)
	switch strings.ToLower(strings.TrimSpace(gpgChoice)) {
	case "":
		// Leave GPG unconfigured.
	case "import":
		cmd.SetImportKey(true)
	case "trusted":
		cmd.SetTrusted(true)
	default:
		return errorutils.CheckErrorf("invalid GPG mode %q — expected 'import', 'trusted', or empty", gpgChoice)
	}
	return cmd.Run()
}

// ── Alpine (APK) ─────────────────────────────────────────────────────────────

const (
	apkKeysDir          = "/etc/apk/keys"
	apkRepositoriesFile = "/etc/apk/repositories"
	alpineReleaseFile   = "/etc/alpine-release"
	apkDefaultBranch    = "main"
)

// configureApk sets up APK to use an Artifactory Alpine repository.
func (sc *SetupCommand) configureApk() error {
	if sc.repoName == "" {
		repoType, err := sc.resolveApkRepoType()
		if err != nil {
			return err
		}
		if err = sc.promptUserToSelectRepositoryFiltered(repoType); err != nil {
			return err
		}
	} else if err := apkValidateRepositoryExists(sc.serverDetails.GetArtifactoryUrl(), sc.repoName, sc.serverDetails); err != nil {
		return err
	}

	rtURL := strings.TrimRight(sc.serverDetails.GetArtifactoryUrl(), "/")

	alpineVersion := detectAlpineVersion()

	var repoURL string
	if alpineVersion != "" {
		repoURL = fmt.Sprintf("%s/%s/%s/%s/", rtURL, sc.repoName, alpineVersion, apkDefaultBranch)
	} else {
		repoURL = fmt.Sprintf("%s/%s/", rtURL, sc.repoName)
	}

	username, password := apkResolveCredentials(sc.serverDetails)
	repoURLWithCreds, err := apkEmbedCredentials(repoURL, username, password)
	if err != nil {
		return err
	}

	if err := apkWriteSigningKey(rtURL, sc.repoName, sc.serverDetails); err != nil {
		log.Warn(fmt.Sprintf("Could not fetch RSA signing key for repo %q: %v\n"+
			"APK will not be able to verify package signatures. "+
			"Configure a key pair on the repository in Artifactory to fix this.", sc.repoName, err))
	}

	return apkUpdateRepositories(repoURLWithCreds)
}

func apkValidateRepositoryExists(rtURL, repoKey string, serverDetails *config.ServerDetails) error {
	endpoint := fmt.Sprintf("%s/api/repositories/%s", strings.TrimRight(rtURL, "/"), repoKey)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return errorutils.CheckErrorf("failed to validate --repo %q: %w", repoKey, err)
	}
	apkSetAuth(req, serverDetails)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errorutils.CheckErrorf("failed to validate --repo %q: %w", repoKey, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest, http.StatusNotFound:
		return errorutils.CheckErrorf("repository %q not found — check --repo or create the repository in Artifactory", repoKey)
	default:
		return errorutils.CheckErrorf("failed to validate --repo %q: Artifactory returned HTTP %d", repoKey, resp.StatusCode)
	}
}

func apkResolveCredentials(serverDetails *config.ServerDetails) (username, password string) {
	if serverDetails == nil {
		return "", ""
	}
	username = serverDetails.GetUser()
	if storedPassword := serverDetails.GetPassword(); storedPassword != "" {
		return username, storedPassword
	}
	token := serverDetails.GetAccessToken()
	if token == "" {
		return username, ""
	}
	if username == "" {
		username = auth.ExtractUsernameFromAccessToken(token)
	}
	log.Warn(fmt.Sprintf("Embedding an access token in %s. Native apk commands will fail with "+
		"\"permission denied\" once the token expires, because nothing refreshes this file. "+
		"Re-run 'jf setup apk' to refresh it, or configure the server with a username and "+
		"password (or a long-lived token) to avoid this.", apkRepositoriesFile))
	return username, token
}

func apkEmbedCredentials(repoURL, username, password string) (string, error) {
	if username == "" && password == "" {
		return repoURL, nil
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", errorutils.CheckErrorf("invalid repository URL %q: %s", repoURL, err.Error())
	}
	parsed.User = url.UserPassword(username, password)
	return parsed.String(), nil
}

func (sc *SetupCommand) resolveApkRepoType() (string, error) {
	if sc.projectKey != "" {
		return "", nil
	}
	return promptApkRepoType()
}

// promptApkRepoType interactively asks the user whether they want a local, remote, or virtual repo.
func promptApkRepoType() (string, error) {
	repoTypes := []string{
		utils.Virtual.String(),
		utils.Local.String(),
		utils.Remote.String(),
	}
	var selected string
	var items []ioutils.PromptItem
	for _, rt := range repoTypes {
		rt := rt
		items = append(items, ioutils.PromptItem{Option: rt, TargetValue: &selected})
	}
	if err := ioutils.SelectString(items,
		"Select the Artifactory Alpine repository type you want to use (virtual is recommended):",
		false,
		func(item ioutils.PromptItem) { selected = item.Option },
	); err != nil {
		return "", err
	}
	return selected, nil
}

// detectAlpineVersion reads /etc/alpine-release and returns the version tag (e.g. "v3.21").
// Returns an empty string if the file is missing or unparseable — callers treat that as
// "version unknown, omit from URL".
func detectAlpineVersion() string {
	data, err := os.ReadFile(alpineReleaseFile)
	if err != nil {
		return ""
	}
	ver := strings.TrimSpace(string(data))
	// Normalise "3.21.0" → "v3.21", "v3.21.0" → "v3.21".
	ver = strings.TrimPrefix(ver, "v")
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		return ""
	}
	return "v" + parts[0] + "." + parts[1]
}

// apkWriteSigningKey fetches the RSA public key for the repository from Artifactory and
// writes it to /etc/apk/keys/.  Returns an error if the repo has no keypair configured
// or the key cannot be downloaded — the caller decides whether to warn or fail.
func apkWriteSigningKey(rtURL, repoKey string, serverDetails *config.ServerDetails) error {
	keyPairRef, err := apkFetchKeyPairRef(rtURL, repoKey, serverDetails)
	if err != nil {
		return err
	}

	keyEndpoint := fmt.Sprintf("%s/api/security/keypair/public/repositories/%s", rtURL, repoKey)
	pemKey, err := apkDownloadRSAKey(keyEndpoint, serverDetails)
	if err != nil {
		return err
	}

	if err = apkMkdirAll(apkKeysDir); err != nil {
		return fmt.Errorf("failed to create %s: %w", apkKeysDir, err)
	}
	keyFilePath := filepath.Join(apkKeysDir, keyPairRef+".rsa.pub")
	if err = apkWriteFile(keyFilePath, pemKey, 0644); err != nil {
		return fmt.Errorf("failed to write RSA key to %s: %w", keyFilePath, err)
	}
	log.Info("RSA signing key written to", keyFilePath)
	return nil
}

// apkSetAuth attaches the appropriate Authorization header to the request.
// Prefers Bearer token; falls back to Basic auth when only username+password are set.
func apkSetAuth(req *http.Request, serverDetails *config.ServerDetails) {
	if token := serverDetails.GetAccessToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if user := serverDetails.GetUser(); user != "" {
		req.SetBasicAuth(user, serverDetails.GetPassword())
	}
}

// apkFetchKeyPairRef queries GET /api/repositories/<repo> and returns the primaryKeyPairRef.
func apkFetchKeyPairRef(rtURL, repoKey string, serverDetails *config.ServerDetails) (string, error) {
	endpoint := fmt.Sprintf("%s/api/repositories/%s", rtURL, repoKey)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	apkSetAuth(req, serverDetails)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned HTTP %d", endpoint, resp.StatusCode)
	}

	var repoConfig struct {
		PrimaryKeyPairRef string `json:"primaryKeyPairRef"`
	}
	if err = json.Unmarshal(body, &repoConfig); err != nil {
		return "", err
	}
	if repoConfig.PrimaryKeyPairRef == "" {
		return "", fmt.Errorf("no primaryKeyPairRef configured on repo %q — attach a key pair in Artifactory first", repoKey)
	}
	return repoConfig.PrimaryKeyPairRef, nil
}

// apkDownloadRSAKey downloads the RSA public key PEM from the Artifactory keypair API.
func apkDownloadRSAKey(endpoint string, serverDetails *config.ServerDetails) (string, error) {
	if serverDetails.GetAccessToken() == "" && serverDetails.GetUser() == "" {
		return "", fmt.Errorf("no credentials configured — run 'jf c add' first")
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	apkSetAuth(req, serverDetails)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("RSA key download failed with HTTP %d: %s", resp.StatusCode, string(body))
	}
	pem := string(body)
	if !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		return "", fmt.Errorf("unexpected response from RSA key endpoint (is a signing keypair configured on the repo?)")
	}
	return pem, nil
}

// apkMkdirAll creates a directory, using sudo if the current process is not root.
func apkMkdirAll(path string) error {
	if os.Getuid() == 0 {
		return os.MkdirAll(path, 0755)
	}
	cmd := exec.Command("sudo", "mkdir", "-p", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// apkWriteFile writes content to path with the given permission bits, using sudo
// when the current process is not root. On the sudo path the file is created with an
// owner-only umask *before* any bytes are written, so a credential-bearing file is never
// briefly world-readable; the exact mode is then enforced with chmod.
func apkWriteFile(path, content string, perm os.FileMode) error {
	if os.Getuid() == 0 {
		if err := os.WriteFile(path, []byte(content), perm); err != nil { // #nosec G703 -- path is a hardcoded system file constant, not user input
			return err
		}
		// os.WriteFile only applies perm when creating a new file; an existing file keeps
		// its old mode. Chmod explicitly so the credential-bearing file is always locked down.
		return os.Chmod(path, perm)
	}
	// `umask 077` creates the file owner-only from the start, avoiding the world-readable
	// window a bare `sudo tee` would leave. path is a positional arg, not shell-interpolated.
	cmd := exec.Command("sudo", "sh", "-c", `umask 077; cat > "$1"`, "sh", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	// Widen to the exact requested mode (e.g. 0644 for the public signing key).
	chmod := exec.Command("sudo", "chmod", fmt.Sprintf("%o", perm), path)
	chmod.Stdout = io.Discard
	chmod.Stderr = os.Stderr
	return chmod.Run()
}

func apkUpdateRepositories(repoURL string) error {
	if err := apkMkdirAll(filepath.Dir(apkRepositoriesFile)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(apkRepositoriesFile), err)
	}

	existing, err := os.ReadFile(apkRepositoriesFile)
	fileExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", apkRepositoriesFile, err)
	}
	originalContent := string(existing)
	content := apkMergeRepositoriesContent(originalContent, repoURL)

	if err = apkWriteFile(apkRepositoriesFile, content, 0600); err != nil {
		// Restore whenever the file existed before — including when it was empty — so a
		// partial write is never left behind. `originalContent != ""` would skip the
		// restore for a previously-empty file.
		if fileExisted {
			if restoreErr := apkWriteFile(apkRepositoriesFile, originalContent, 0600); restoreErr != nil {
				return fmt.Errorf("failed to write %s: %w (also failed to restore original content: %v)", apkRepositoriesFile, err, restoreErr)
			}
		}
		return fmt.Errorf("failed to write %s: %w", apkRepositoriesFile, err)
	}
	log.Info(fmt.Sprintf("APK repository configured: %s → %s", apkRepositoriesFile, apkRedactCredentials(repoURL)))
	return nil
}

func apkMergeRepositoriesContent(existing, repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if existing == "" {
		return repoURL + "\n"
	}

	artHost := apkRepoHostname(repoURL)
	existing = strings.TrimSuffix(existing, "\n")
	lines := strings.Split(existing, "\n")
	out := make([]string, 0, len(lines)+1)
	inserted := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, line)
			continue
		}
		if artHost != "" && apkRepoHostname(trimmed) == artHost {
			if !inserted {
				out = append(out, repoURL)
				inserted = true
			}
			continue
		}
		out = append(out, line)
	}

	if !inserted {
		out = append([]string{repoURL}, out...)
	}
	return strings.Join(out, "\n") + "\n"
}

func apkRepoHostname(repoLine string) string {
	fields := strings.Fields(strings.TrimSpace(repoLine))
	if len(fields) == 0 {
		return ""
	}
	candidate := fields[0]
	if strings.HasPrefix(candidate, "@") {
		if len(fields) < 2 {
			return ""
		}
		candidate = fields[1]
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Hostname()
}

func apkRedactCredentials(repoURL string) string {
	parsed, err := url.Parse(repoURL)
	if err != nil || parsed.User == nil {
		return repoURL
	}
	// Splice in a literal masked userinfo instead of going through url.UserPassword + String(),
	// which percent-encodes the mask ("*" -> "%2A") and prints noisy "%2A%2A%2A:%2A%2A%2A@host".
	parsed.User = nil
	return fmt.Sprintf("%s://***:***@%s", parsed.Scheme, strings.TrimPrefix(parsed.String(), parsed.Scheme+"://"))
}
