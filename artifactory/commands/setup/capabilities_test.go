package setup

import (
	"encoding/json"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The point of publishing the table: adding a package manager to `jf setup`
// without describing it here would ship an incomplete contract to the consumers
// that replaced their own copies with it.
func TestGetCapabilities_CoversEverySupportedPackageManager(t *testing.T) {
	capabilities := GetCapabilities()
	supported := GetSupportedPackageManagersList()

	require.Len(t, capabilities, len(supported),
		"every package manager `jf setup` supports needs exactly one Capability")

	byName := make(map[string]Capability, len(capabilities))
	for _, capability := range capabilities {
		byName[capability.PackageManager] = capability
	}
	for _, packageManager := range supported {
		require.Contains(t, byName, packageManager,
			"`jf setup %s` is supported but has no Capability entry", packageManager)
	}

	// Order has to match, so callers can zip the two lists.
	for i, packageManager := range supported {
		assert.Equal(t, packageManager, capabilities[i].PackageManager,
			"GetCapabilities must use the same order as GetSupportedPackageManagersList")
	}
}

// Every field a consumer is told it can rely on must actually be populated —
// an entry added with only a location would otherwise pass the coverage test
// above while publishing empty binaries and version arguments.
func TestGetCapabilities_EveryEntryIsFullyPopulated(t *testing.T) {
	for _, capability := range GetCapabilities() {
		t.Run(capability.PackageManager, func(t *testing.T) {
			assert.NotEmpty(t, capability.RepoPackageType, "repoPackageType must be set")
			assert.NotEmpty(t, capability.ConfigLocation, "configLocation must be set")
			assert.NotEmpty(t, capability.ClientBinaries, "clientBinaries must never be empty")
			assert.NotEmpty(t, capability.VersionCmd, "versionCmd must never be empty")
			assert.Contains(t, []string{RoleInstall, RolePublish}, capability.Role)
			for _, binary := range capability.ClientBinaries {
				assert.NotEmpty(t, binary, "clientBinaries must not contain empty strings")
			}
		})
	}
}

// Defaults exist so entries only spell out what differs. Assert the defaults are
// applied rather than silently left empty.
func TestGetCapabilities_Defaults(t *testing.T) {
	npm, err := GetCapability(project.Npm)
	require.NoError(t, err)
	assert.Equal(t, []string{"npm"}, npm.ClientBinaries, "clientBinaries defaults to the package manager's own name")
	assert.Equal(t, []string{"--version"}, npm.VersionCmd, "versionCmd defaults to --version")
	assert.Empty(t, npm.Aliases)
	assert.Equal(t, RoleInstall, npm.Role)
	assert.True(t, npm.RequiresClient)
}

// The three facts downstream copies got wrong, pinned here.
func TestGetCapabilities_KnownSpecialCases(t *testing.T) {
	t.Run("maven and gradle need no client on PATH", func(t *testing.T) {
		// configureMaven goes through NewSettingsXmlManager and configureGradle through
		// WriteInitScript, so a wrapper-only project must not be gated on a PATH lookup.
		for _, packageManager := range []project.ProjectType{project.Maven, project.Gradle} {
			capability, err := GetCapability(packageManager)
			require.NoError(t, err)
			assert.False(t, capability.RequiresClient,
				"%s setup writes its configuration directly", capability.PackageManager)
		}
	})

	t.Run("maven is reachable as mvn", func(t *testing.T) {
		maven, err := GetCapability(project.Maven)
		require.NoError(t, err)
		assert.Equal(t, []string{"mvn"}, maven.Aliases)
		assert.Equal(t, []string{"mvn"}, maven.ClientBinaries)
	})

	t.Run("pip prefers pip over pip3", func(t *testing.T) {
		// Matches getExecutable, which tries pip first and falls back to pip3.
		pip, err := GetCapability(project.Pip)
		require.NoError(t, err)
		assert.Equal(t, []string{"pip", "pip3"}, pip.ClientBinaries)
	})

	t.Run("twine is publish-only", func(t *testing.T) {
		twine, err := GetCapability(project.Twine)
		require.NoError(t, err)
		assert.Equal(t, RolePublish, twine.Role)
		assert.Equal(t, "pypi", twine.RepoPackageType, "publish-only does not mean a different repo type")
	})

	t.Run("clients that reject --version", func(t *testing.T) {
		for packageManager, expected := range map[project.ProjectType][]string{
			project.Nuget:  {"help"},
			project.Dotnet: {"help"},
			project.Go:     {"version"},
			project.Helm:   {"version", "--short"},
		} {
			capability, err := GetCapability(packageManager)
			require.NoError(t, err)
			assert.Equal(t, expected, capability.VersionCmd, capability.PackageManager)
		}
	})

	t.Run("helm requires OCI support", func(t *testing.T) {
		// configureHelm runs `helm registry login`; OCI reached GA in Helm 3.8.0.
		helm, err := GetCapability(project.Helm)
		require.NoError(t, err)
		assert.Equal(t, "3.8.0", helm.MinVersion)
	})
}

// A shared config file is a concurrency constraint, so the grouping has to be
// exactly the set of package managers that write the same file — no more.
func TestGetCapabilities_ConfigGroupsMatchSharedFiles(t *testing.T) {
	grouped := make(map[string][]string)
	for _, capability := range GetCapabilities() {
		if capability.ConfigGroup != "" {
			grouped[capability.ConfigGroup] = append(grouped[capability.ConfigGroup], capability.PackageManager)
		}
	}

	assert.Equal(t, map[string][]string{
		ConfigGroupPipConf:     {"pip", "pipenv"},
		ConfigGroupNugetConfig: {"nuget", "dotnet"},
	}, grouped)

	// npm, pnpm and Yarn each write their own file, so grouping them would
	// needlessly serialize three independent setups.
	for _, packageManager := range []project.ProjectType{project.Npm, project.Pnpm, project.Yarn} {
		capability, err := GetCapability(packageManager)
		require.NoError(t, err)
		assert.Empty(t, capability.ConfigGroup, "%s writes its own file", capability.PackageManager)
	}
	// Docker and Podman keep separate credential stores.
	for _, packageManager := range []project.ProjectType{project.Docker, project.Podman} {
		capability, err := GetCapability(packageManager)
		require.NoError(t, err)
		assert.Empty(t, capability.ConfigGroup, "%s has its own credential store", capability.PackageManager)
	}

	// Every group must have more than one member, or it is not a constraint.
	for group, packageManagers := range grouped {
		assert.Greater(t, len(packageManagers), 1, "config group %q has a single member", group)
	}
}

func TestGetCapability_UnsupportedPackageManager(t *testing.T) {
	_, err := GetCapability(project.Cocoapods)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported package manager")
}

func TestMarshalCapabilities(t *testing.T) {
	encoded, err := MarshalCapabilities()
	require.NoError(t, err)

	var decoded []Capability
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	assert.Equal(t, GetCapabilities(), decoded, "the JSON form must round-trip")

	// Optional fields must stay absent rather than emitting empty values that a
	// consumer would have to distinguish from a real one.
	var raw []map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded), &raw))
	for _, entry := range raw {
		if entry["packageManager"] == "npm" {
			assert.NotContains(t, entry, "aliases")
			assert.NotContains(t, entry, "minVersion")
			assert.NotContains(t, entry, "configGroup")
			assert.Contains(t, entry, "requiresClient", "non-optional fields must always be present")
		}
	}
}

// Callers group by repo type to decide which package managers a governed
// repository covers, so the mapping has to survive being inverted.
func TestGetCapabilities_GroupByRepoPackageType(t *testing.T) {
	byRepoType := make(map[string][]string)
	for _, capability := range GetCapabilities() {
		byRepoType[capability.RepoPackageType] = append(byRepoType[capability.RepoPackageType], capability.PackageManager)
	}

	assert.ElementsMatch(t, []string{"pip", "pipenv", "poetry", "twine", "uv"}, byRepoType["pypi"])
	assert.ElementsMatch(t, []string{"npm", "pnpm", "yarn"}, byRepoType["npm"])
	assert.ElementsMatch(t, []string{"docker", "podman"}, byRepoType["docker"])
	assert.ElementsMatch(t, []string{"nuget", "dotnet"}, byRepoType["nuget"])

	// Gradle is its own Artifactory package type here, NOT part of "maven".
	// Pinned deliberately: downstream copies of this table disagree — the Fly client
	// registers gradle under its maven repo name, and the agent hooks map the maven
	// package type to [maven, gradle]. Anything resolving a gradle repository from
	// the "maven" type is contradicting this mapping, and `jf setup gradle` will not
	// offer a maven-type repository when it prompts.
	assert.Equal(t, []string{"maven"}, byRepoType["maven"])
	assert.Equal(t, []string{"gradle"}, byRepoType["gradle"])
}
