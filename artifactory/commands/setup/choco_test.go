package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChocoSourceDetailsUsesV2URL(t *testing.T) {
	serverDetails := &config.ServerDetails{
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
		User:           "john",
		Password:       "secret",
	}

	sourceURL, apiKey, err := chocoSourceDetails(serverDetails, "choco-virtual")
	require.NoError(t, err)
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/nuget/choco-virtual", sourceURL)
	assert.NotContains(t, sourceURL, "/v3/")
	assert.NotContains(t, sourceURL, "index.json")
	assert.Equal(t, "john:secret", apiKey)
}

func TestChocoSourceDetailsValidatesInput(t *testing.T) {
	_, _, err := chocoSourceDetails(nil, "choco-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details")

	_, _, err = chocoSourceDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository")

	_, _, err = chocoSourceDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}, "choco-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials")
}

func TestChocoSourceName(t *testing.T) {
	sourceName, err := chocoSourceName(
		&config.ServerDetails{ArtifactoryUrl: "https://Acme.JFrog.io/artifactory/"},
		"Team Repo/Release",
	)
	require.NoError(t, err)
	assert.Equal(t, "jfrt-acme.jfrog.io-team-repo-release", sourceName)
}

func TestChocoSourceNameValidatesInput(t *testing.T) {
	_, err := chocoSourceName(nil, "choco-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server details")

	_, err = chocoSourceName(&config.ServerDetails{ArtifactoryUrl: "://invalid"}, "choco-virtual")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Artifactory URL")
}

func TestNormalizeChocoRepositoryType(t *testing.T) {
	for input, expected := range map[string]string{
		"virtual":  services.VirtualRepositoryRepoType,
		"local":    services.LocalRepositoryRepoType,
		"remote":   services.RemoteRepositoryRepoType,
		" REMOTE ": services.RemoteRepositoryRepoType,
	} {
		t.Run(input, func(t *testing.T) {
			actual, err := normalizeChocoRepositoryType(input)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}

	_, err := normalizeChocoRepositoryType("federated")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "virtual, local, or remote")
}

func TestConfigureChocoCreatesVirtualSource(t *testing.T) {
	calls := stubChocoCommandRunner(t)
	stubChocoPlatformChecker(t, true)
	stubChocoRepoClassResolver(t, services.VirtualRepositoryRepoType, nil)

	configureChocoForTest(t, "choco-virtual", "secret")

	const sourceURL = "https://acme.jfrog.io/artifactory/api/nuget/choco-virtual"
	assert.Equal(t, [][]string{
		{"choco", "source", "remove", "-n=jfrt-acme.jfrog.io-choco-virtual"},
		{"choco", "source", "add", "-n=jfrt-acme.jfrog.io-choco-virtual", "-s=" + sourceURL, "--priority=1"},
		{"choco", "apikey", "add", "-s=" + sourceURL, "-k=john:secret"},
	}, *calls)
}

func TestConfigureChocoCreatesLocalSourceWithoutPriority(t *testing.T) {
	calls := stubChocoCommandRunner(t)
	stubChocoPlatformChecker(t, true)
	stubChocoRepoClassResolver(t, services.LocalRepositoryRepoType, nil)

	configureChocoForTest(t, "choco-local", "secret")

	assert.Equal(t, []string{
		"choco", "source", "add", "-n=jfrt-acme.jfrog.io-choco-local",
		"-s=https://acme.jfrog.io/artifactory/api/nuget/choco-local",
	}, (*calls)[1])
}

func TestConfigureChocoCreatesRemoteSourceWithPriority(t *testing.T) {
	calls := stubChocoCommandRunner(t)
	stubChocoPlatformChecker(t, true)
	stubChocoRepoClassResolver(t, services.RemoteRepositoryRepoType, nil)

	configureChocoForTest(t, "choco-remote", "secret")

	assert.Equal(t, []string{
		"choco", "source", "add", "-n=jfrt-acme.jfrog.io-choco-remote",
		"-s=https://acme.jfrog.io/artifactory/api/nuget/choco-remote", "--priority=1",
	}, (*calls)[1])
}

func TestConfigureChocoDoesNotLeakSecrets(t *testing.T) {
	stubChocoPlatformChecker(t, true)
	stubChocoRepoClassResolver(t, services.VirtualRepositoryRepoType, nil)
	originalRunner := chocoCommandRunner
	chocoCommandRunner = func(string, ...string) error { return errors.New("boom") }
	t.Cleanup(func() { chocoCommandRunner = originalRunner })

	err := newChocoSetupCommand("choco-virtual", "sup3rs3cr3t").configureChoco()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sup3rs3cr3t")
}

func TestConfigureChocoToleratesMissingSource(t *testing.T) {
	stubChocoPlatformChecker(t, true)
	stubChocoRepoClassResolver(t, services.VirtualRepositoryRepoType, nil)
	originalRunner := chocoCommandRunner
	chocoCommandRunner = func(_ string, args ...string) error {
		if len(args) > 1 && args[0] == "source" && args[1] == "remove" {
			return exitWithStatus(t, 2)
		}
		return nil
	}
	t.Cleanup(func() { chocoCommandRunner = originalRunner })

	configureChocoForTest(t, "choco-virtual", "secret")
}

func TestConfigureChocoNonWindowsFailsClearly(t *testing.T) {
	stubChocoPlatformChecker(t, false)

	err := ValidateChocoPlatform()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Windows")
	assert.Contains(t, err.Error(), runtime.GOOS)
}

func TestConfigScopeNoteChocoIsMachineWide(t *testing.T) {
	note := configScopeNote(project.Choco)
	assert.Contains(t, note, "every user on this machine")
	assert.NotContains(t, note, "for this user")
}

func TestChocoIsSupportedBySetup(t *testing.T) {
	assert.True(t, IsSupportedPackageManager(project.Choco))
	assert.Contains(t, GetSupportedPackageManagersList(), "choco")

	packageType, err := GetRepositoryPackageType(project.Choco)
	require.NoError(t, err)
	assert.Equal(t, repository.Nuget, packageType)
}

func configureChocoForTest(t *testing.T, repoName, password string) {
	t.Helper()
	require.NoError(t, newChocoSetupCommand(repoName, password).configureChoco())
}

func newChocoSetupCommand(repoName, password string) *SetupCommand {
	return &SetupCommand{
		packageManager: project.Choco,
		repoName:       repoName,
		serverDetails: &config.ServerDetails{
			ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
			User:           "john",
			Password:       password,
		},
	}
}

func stubChocoCommandRunner(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	originalRunner := chocoCommandRunner
	chocoCommandRunner = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() { chocoCommandRunner = originalRunner })
	return &calls
}

func stubChocoPlatformChecker(t *testing.T, isWindows bool) {
	t.Helper()
	originalChecker := chocoPlatformChecker
	chocoPlatformChecker = func() bool { return isWindows }
	t.Cleanup(func() { chocoPlatformChecker = originalChecker })
}

func stubChocoRepoClassResolver(t *testing.T, repoClass string, err error) {
	t.Helper()
	originalResolver := chocoRepoClassResolver
	chocoRepoClassResolver = func(*config.ServerDetails, string) (string, error) {
		return repoClass, err
	}
	t.Cleanup(func() { chocoRepoClassResolver = originalResolver })
}

func exitWithStatus(t *testing.T, status int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestChocoExitWithStatus")
	cmd.Env = append(os.Environ(), "GO_WANT_CHOCO_EXIT_STATUS=1", fmt.Sprintf("CHOCO_EXIT_STATUS=%d", status))
	return cmd.Run()
}

func TestChocoExitWithStatus(t *testing.T) {
	if os.Getenv("GO_WANT_CHOCO_EXIT_STATUS") != "1" {
		return
	}
	status, err := strconv.Atoi(os.Getenv("CHOCO_EXIT_STATUS"))
	if err != nil {
		os.Exit(1)
	}
	os.Exit(status)
}
