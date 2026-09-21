package setup

import (
	"errors"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPSResourceSourceName(t *testing.T) {
	sourceName, err := psresourceSourceName(
		&config.ServerDetails{ArtifactoryUrl: "https://Acme.JFrog.io/artifactory/"},
		"Team Repo/Release",
	)
	require.NoError(t, err)
	assert.Equal(t, "jfrt-acme.jfrog.io-team-repo-release", sourceName)
}

func TestPSResourceSourceNameValidatesInput(t *testing.T) {
	_, err := psresourceSourceName(nil, "psresource-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details")

	_, err = psresourceSourceName(&config.ServerDetails{ArtifactoryUrl: "://invalid"}, "psresource-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Artifactory URL")

	// A repo name that sanitizes to empty (see TestSanitizePSResourceSourceComponent) must be
	// caught at this integration point too, not just at the sanitizer's own unit level - otherwise
	// a malformed source name like "jfrt--" could reach Register-PSResourceRepository verbatim.
	_, err = psresourceSourceName(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}, "///")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository name")
}

func TestSanitizePSResourceSourceComponent(t *testing.T) {
	assert.Equal(t, "acme.jfrog.io", sanitizePSResourceSourceComponent("Acme.JFrog.io"))
	assert.Equal(t, "team-repo-release", sanitizePSResourceSourceComponent("Team Repo/Release"))
	assert.Equal(t, "", sanitizePSResourceSourceComponent("///"))
}

func TestPSResourcePSStringLiteral(t *testing.T) {
	assert.Equal(t, "'plain'", QuotePSLiteral("plain"))
	assert.Equal(t, "'it''s escaped'", QuotePSLiteral("it's escaped"))
}

func TestValidatePSResourcePlatform(t *testing.T) {
	stubPSResourcePlatformChecker(t, true)
	require.NoError(t, ValidatePSResourcePlatform())

	stubPSResourcePlatformChecker(t, false)
	err := ValidatePSResourcePlatform()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pwsh")
	assert.Contains(t, err.Error(), "Microsoft.PowerShell.PSResourceGet")
}

func TestPSResourcePlatformCheckerUsesShellAndModuleCheck(t *testing.T) {
	stubPSResourceShellResolver(t, "pwsh", true)

	var capturedArgs []string
	restoreRunner := stubPSResourceCommandRunnerCapturing(t, &capturedArgs, nil)
	defer restoreRunner()
	assert.True(t, psresourcePlatformChecker())
	require.Len(t, capturedArgs, 4)
	assert.Equal(t, "pwsh", capturedArgs[0])
	assert.Contains(t, capturedArgs[3], "Microsoft.PowerShell.PSResourceGet")
}

func TestPSResourcePlatformCheckerFailsWhenNoShellFound(t *testing.T) {
	stubPSResourceShellResolver(t, "", false)
	assert.False(t, psresourcePlatformChecker())
}

func TestPSResourcePlatformCheckerFailsWhenModuleMissing(t *testing.T) {
	stubPSResourceShellResolver(t, "pwsh", true)
	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(string, ...string) error { return errors.New("exit status 1") }
	t.Cleanup(func() { psresourceCommandRunner = originalRunner })
	assert.False(t, psresourcePlatformChecker())
}

func TestPSResourceShell(t *testing.T) {
	stubPSResourceShellResolver(t, "pwsh", true)
	shell, err := PSResourceShell()
	require.NoError(t, err)
	assert.Equal(t, "pwsh", shell)

	stubPSResourceShellResolver(t, "", false)
	_, err = PSResourceShell()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pwsh")
}

// psresourcePlatformChecker is never gated on runtime.GOOS: as long as a shell is found and the
// module check succeeds, the platform is usable regardless of which OS this test runs on.
func TestPSResourcePlatformCheckerNotGatedOnOS(t *testing.T) {
	stubPSResourceShellResolver(t, "pwsh", true)
	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(string, ...string) error { return nil }
	t.Cleanup(func() { psresourceCommandRunner = originalRunner })
	assert.True(t, psresourcePlatformChecker())
}

func TestConfigurePSResourceRegistersSource(t *testing.T) {
	calls := stubPSResourceCommandRunner(t)
	stubPSResourcePlatformChecker(t, true)
	stubPSResourceShellResolver(t, "pwsh", true)
	stubPSResourceRepoClassResolver(t)

	require.NoError(t, newPSResourceSetupCommand("secret").configurePSResource())

	require.Len(t, *calls, 2)
	unregister := (*calls)[0]
	register := (*calls)[1]

	assert.Equal(t, "pwsh", unregister[0])
	assert.Contains(t, unregister[3], "Unregister-PSResourceRepository")
	assert.Contains(t, unregister[3], "jfrt-acme.jfrog.io-psresource-virtual")
	assert.Contains(t, unregister[3], "-ErrorAction SilentlyContinue")

	assert.Equal(t, "pwsh", register[0])
	assert.Contains(t, register[3], "Register-PSResourceRepository")
	assert.Contains(t, register[3], "jfrt-acme.jfrog.io-psresource-virtual")
	assert.Contains(t, register[3], "https://acme.jfrog.io/artifactory/api/nuget/v3/psresource-virtual/index.json")
	assert.Contains(t, register[3], "-Trusted")
}

// PSResourceGet's registration is deliberately credential-free: -Credential is injected per
// invocation by the psresource FlexPack command instead (see artifactory/commands/psresource).
func TestConfigurePSResourceDoesNotPersistCredentials(t *testing.T) {
	calls := stubPSResourceCommandRunner(t)
	stubPSResourcePlatformChecker(t, true)
	stubPSResourceShellResolver(t, "pwsh", true)
	stubPSResourceRepoClassResolver(t)

	require.NoError(t, newPSResourceSetupCommand("sup3rs3cr3t").configurePSResource())

	for _, call := range *calls {
		for _, arg := range call {
			assert.NotContains(t, arg, "sup3rs3cr3t")
			assert.NotContains(t, arg, "-CredentialInfo")
			assert.NotContains(t, arg, "SecretManagement")
		}
	}
}

func TestConfigurePSResourceToleratesUnregisterFailure(t *testing.T) {
	stubPSResourcePlatformChecker(t, true)
	stubPSResourceShellResolver(t, "pwsh", true)
	stubPSResourceRepoClassResolver(t)

	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(_ string, args ...string) error {
		if len(args) > 3 && args[2] == "-Command" && strings.Contains(args[3], "Unregister-PSResourceRepository") {
			return errors.New("boom")
		}
		return nil
	}
	t.Cleanup(func() { psresourceCommandRunner = originalRunner })

	require.NoError(t, newPSResourceSetupCommand("secret").configurePSResource())
}

// TestConfigurePSResourceSurfacesRegisterFailure guards against a real bug: a failing
// Register-PSResourceRepository call was replaced with a static, generic "install pwsh" message
// that discarded the actual error - even though ValidatePSResourcePlatform had already confirmed
// pwsh/the module were installed a few lines earlier - making the real cause undiagnosable.
func TestConfigurePSResourceSurfacesRegisterFailure(t *testing.T) {
	stubPSResourcePlatformChecker(t, true)
	stubPSResourceShellResolver(t, "pwsh", true)
	stubPSResourceRepoClassResolver(t)

	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(_ string, args ...string) error {
		if len(args) > 2 && args[1] == "-Command" && strings.Contains(args[2], "Register-PSResourceRepository") {
			return errors.New("a distinctive, real PowerShell failure message")
		}
		return nil
	}
	t.Cleanup(func() { psresourceCommandRunner = originalRunner })

	err := newPSResourceSetupCommand("secret").configurePSResource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a distinctive, real PowerShell failure message", "the real error must be surfaced, not replaced with a generic message")
}

func TestConfigurePSResourceNonWindowsStillWorks(t *testing.T) {
	// Unlike Chocolatey, PSResourceGet through pwsh is cross-platform, so a non-Windows machine
	// with pwsh installed must succeed here.
	calls := stubPSResourceCommandRunner(t)
	stubPSResourcePlatformChecker(t, true)
	stubPSResourceShellResolver(t, "pwsh", true)
	stubPSResourceRepoClassResolver(t)

	require.NoError(t, newPSResourceSetupCommand("secret").configurePSResource())
	assert.Len(t, *calls, 2)
}

func TestConfigurePSResourceFailsClearlyWhenPlatformUnavailable(t *testing.T) {
	stubPSResourcePlatformChecker(t, false)

	err := newPSResourceSetupCommand("secret").configurePSResource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Microsoft.PowerShell.PSResourceGet")
}

func TestConfigurePSResourceRejectsNonNugetRepo(t *testing.T) {
	stubPSResourcePlatformChecker(t, true)
	originalResolver := psresourceRepoClassResolver
	psresourceRepoClassResolver = func(*config.ServerDetails, string) (string, error) {
		return "", errors.New(`repository "psresource-virtual" is package type "docker"; PSResourceGet requires a NuGet repository`)
	}
	t.Cleanup(func() { psresourceRepoClassResolver = originalResolver })

	err := newPSResourceSetupCommand("secret").configurePSResource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NuGet repository")
}

// TestPSResourceIsSupportedBySetup mirrors choco's own TestChocoIsSupportedBySetup: PSResource must
// be wired into both packageManagerConfigs and packageManagerToRepositoryPackageType (asserted to
// stay in sync by TestPackageManagerConfigs_CoversEverySupportedPackageManager), and its repository
// package type must be NuGet - Artifactory has no distinct PowerShell package type.
func TestPSResourceIsSupportedBySetup(t *testing.T) {
	assert.True(t, IsSupportedPackageManager(project.PSResource))
	assert.Contains(t, GetSupportedPackageManagersList(), "psresource")

	packageType, err := GetRepositoryPackageType(project.PSResource)
	require.NoError(t, err)
	assert.Equal(t, repository.Nuget, packageType)
}

// PSResourceGet has no environment-variable override for its configuration location, unlike e.g.
// pip's PIP_CONFIG_FILE, so configScopeNote must fall back to the plain user-level message.
func TestConfigScopeNotePSResourceIsUserLevel(t *testing.T) {
	note := configScopeNote(project.PSResource)
	assert.Contains(t, note, "for this user")
	assert.Contains(t, note, "PSResourceRepository.xml")
}

func newPSResourceSetupCommand(password string) *SetupCommand {
	return &SetupCommand{
		repoName: "psresource-virtual",
		serverDetails: &config.ServerDetails{
			ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
			User:           "john",
			Password:       password,
		},
	}
}

func stubPSResourceCommandRunner(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() { psresourceCommandRunner = originalRunner })
	return &calls
}

func stubPSResourceCommandRunnerCapturing(t *testing.T, captured *[]string, retErr error) func() {
	t.Helper()
	originalRunner := psresourceCommandRunner
	psresourceCommandRunner = func(name string, args ...string) error {
		*captured = append([]string{name}, args...)
		return retErr
	}
	return func() { psresourceCommandRunner = originalRunner }
}

func stubPSResourcePlatformChecker(t *testing.T, ok bool) {
	t.Helper()
	original := psresourcePlatformChecker
	psresourcePlatformChecker = func() bool { return ok }
	t.Cleanup(func() { psresourcePlatformChecker = original })
}

func stubPSResourceShellResolver(t *testing.T, shell string, found bool) {
	t.Helper()
	original := psresourceShellResolver
	psresourceShellResolver = func() (string, bool) { return shell, found }
	t.Cleanup(func() { psresourceShellResolver = original })
}

func stubPSResourceRepoClassResolver(t *testing.T) {
	t.Helper()
	original := psresourceRepoClassResolver
	psresourceRepoClassResolver = func(*config.ServerDetails, string) (string, error) {
		return services.VirtualRepositoryRepoType, nil
	}
	t.Cleanup(func() { psresourceRepoClassResolver = original })
}
