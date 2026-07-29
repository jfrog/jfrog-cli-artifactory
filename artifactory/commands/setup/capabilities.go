package setup

import (
	"encoding/json"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// Roles a package manager can be configured for.
const (
	// RoleInstall means setup redirects dependency resolution through Artifactory.
	RoleInstall = "install"
	// RolePublish means setup configures uploads, not resolution.
	RolePublish = "publish"
)

// Configuration-file groups. Every package manager in one group writes the SAME
// file, so a caller setting up more than one of them must serialize those runs.
const (
	ConfigGroupPipConf     = "pip-conf"
	ConfigGroupNugetConfig = "nuget-config"
)

// defaultVersionArg is what most clients accept to print their version.
var defaultVersionArg = []string{"--version"}

// Capability is the machine-readable description of one package manager that
// `jf setup` supports.
//
// It exists because every downstream consumer had grown its own copy of this
// table — Fly Desktop (TypeScript), the Fly client (Go) and the JFrog agent
// hooks each hand-maintain a package-manager list, binary names, shared-config
// groupings and which package managers need a client on PATH. None of those
// copies can be checked against this package, so each one drifts silently the
// day a package manager is added here. Publishing the table makes those copies
// deletable.
type Capability struct {
	// PackageManager is the token accepted by `jf setup <package manager>`.
	PackageManager string `json:"packageManager"`
	// RepoPackageType is the Artifactory repository package type it resolves
	// against, e.g. "pypi" for pip, pipenv, poetry, twine and uv.
	RepoPackageType string `json:"repoPackageType"`
	// Aliases are other accepted names for the same package manager.
	Aliases []string `json:"aliases,omitempty"`
	// ClientBinaries are the executables to look for on PATH, in preference order.
	ClientBinaries []string `json:"clientBinaries"`
	// VersionCmd is the argument list that makes the client print its version.
	// Not every client accepts --version.
	VersionCmd []string `json:"versionCmd"`
	// MinVersion, when set, is the oldest client release setup can configure.
	MinVersion string `json:"minVersion,omitempty"`
	// RequiresClient reports whether setup runs the client. When false the
	// configuration file is written directly and setup works with no client
	// installed — which is what wrapper-only projects (./mvnw, ./gradlew) rely on.
	RequiresClient bool `json:"requiresClient"`
	// Role is RoleInstall or RolePublish.
	Role string `json:"role"`
	// CredentialsOnly reports that setup stores credentials without redirecting
	// resolution, so an unqualified `docker pull alpine` still goes to Docker Hub.
	CredentialsOnly bool `json:"credentialsOnly"`
	// ConfigGroup, when set, names the configuration file shared with other
	// package managers. Setups within one group must not run concurrently.
	ConfigGroup string `json:"configGroup,omitempty"`
	// ConfigLocation describes the configuration in the user's terms. It is not an
	// absolute path: the real path is platform dependent and the package manager
	// resolves it itself.
	ConfigLocation string `json:"configLocation"`
	// OverrideEnv, when set, is the environment variable that moves the
	// configuration off its user-level default.
	OverrideEnv string `json:"overrideEnv,omitempty"`
}

// GetCapabilities returns one Capability per supported package manager, in the
// same order as GetSupportedPackageManagersList.
func GetCapabilities() []Capability {
	packageManagers := maps.Keys(packageManagerToRepositoryPackageType)
	slices.SortFunc(packageManagers, func(a, b project.ProjectType) int {
		return int(a) - int(b)
	})

	capabilities := make([]Capability, 0, len(packageManagers))
	for _, packageManager := range packageManagers {
		capabilities = append(capabilities, buildCapability(packageManager))
	}
	return capabilities
}

// GetCapability returns the Capability for one package manager, or an error when
// `jf setup` does not support it.
func GetCapability(packageManager project.ProjectType) (Capability, error) {
	if !IsSupportedPackageManager(packageManager) {
		return Capability{}, errorutils.CheckErrorf("unsupported package manager: %s", packageManager)
	}
	return buildCapability(packageManager), nil
}

// MarshalCapabilities renders the whole table as indented JSON, for callers that
// read it as the output of a command rather than as a Go value.
func MarshalCapabilities() (string, error) {
	encoded, err := json.MarshalIndent(GetCapabilities(), "", "  ")
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	return string(encoded), nil
}

func buildCapability(packageManager project.ProjectType) Capability {
	name := packageManager.String()
	// A package manager present in packageManagerToRepositoryPackageType but absent
	// from packageManagerConfigs yields the zero config, which would publish empty
	// binaries and an empty location. TestGetCapabilities_EveryEntryIsFullyPopulated
	// keeps the two maps in step, so that is a test failure rather than bad output.
	config := packageManagerConfigs[packageManager]

	clientBinaries := config.clientBinaries
	if len(clientBinaries) == 0 {
		clientBinaries = []string{name}
	}
	versionCmd := config.versionCmd
	if len(versionCmd) == 0 {
		versionCmd = defaultVersionArg
	}
	role := RoleInstall
	if config.publishOnly {
		role = RolePublish
	}

	return Capability{
		PackageManager:  name,
		RepoPackageType: packageManagerToRepositoryPackageType[packageManager],
		Aliases:         slices.Clone(config.aliases),
		ClientBinaries:  slices.Clone(clientBinaries),
		VersionCmd:      slices.Clone(versionCmd),
		MinVersion:      config.minVersion,
		RequiresClient:  !config.configOnly,
		Role:            role,
		CredentialsOnly: config.credentialsOnly,
		ConfigGroup:     config.configGroup,
		ConfigLocation:  config.location,
		OverrideEnv:     config.overrideEnv,
	}
}
