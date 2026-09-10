package choco

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	buildutils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChocoCommandNonWindowsFailsClearly(t *testing.T) {
	originalChecker := chocoPlatformChecker
	chocoPlatformChecker = func() bool { return false }
	t.Cleanup(func() { chocoPlatformChecker = originalChecker })

	err := NewChocoFlexPackCommand().SetSubCommand("list").Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Windows")
	assert.Contains(t, err.Error(), runtime.GOOS)
}

func TestChocoCommandPassesNativeArguments(t *testing.T) {
	originalChecker := chocoPlatformChecker
	originalRunner := chocoNativeRunner
	chocoPlatformChecker = func() bool { return true }
	var received []string
	chocoNativeRunner = func(args []string) error {
		received = append([]string(nil), args...)
		return nil
	}
	t.Cleanup(func() {
		chocoPlatformChecker = originalChecker
		chocoNativeRunner = originalRunner
	})

	err := NewChocoFlexPackCommand().
		SetSubCommand("list").
		SetArgs([]string{"--source", "jfrt-acme.jfrog.io-choco-virtual", "--limit-output"}).
		SetWorkingDirectory(t.TempDir()).
		Run()
	require.NoError(t, err)
	assert.Equal(t, []string{"list", "--source", "jfrt-acme.jfrog.io-choco-virtual", "--limit-output"}, received)
}

func TestChocoCommandNativeFailureIsWrapped(t *testing.T) {
	originalChecker := chocoPlatformChecker
	originalRunner := chocoNativeRunner
	chocoPlatformChecker = func() bool { return true }
	chocoNativeRunner = func([]string) error { return errors.New("native failure") }
	t.Cleanup(func() {
		chocoPlatformChecker = originalChecker
		chocoNativeRunner = originalRunner
	})

	err := NewChocoFlexPackCommand().SetSubCommand("install").SetWorkingDirectory(t.TempDir()).Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "choco install failed")
	assert.Contains(t, err.Error(), "native failure")
}

func TestRequestedPackages(t *testing.T) {
	packages := requestedPackages([]string{"git", "--version", "2.43.0", "--yes", "7zip.install"})
	assert.Equal(t, []string{"git", "7zip.install"}, packages)
}

func TestRepoFromSource(t *testing.T) {
	serverDetails := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
	assert.Equal(t, "choco-local", repoFromSource("jfrt-acme.jfrog.io-choco-local", serverDetails))
	assert.Equal(t, "choco-local", repoFromSource("https://acme.jfrog.io/artifactory/api/nuget/choco-local", serverDetails))
	assert.Equal(t, "choco-virtual", repoFromSource("https://acme.jfrog.io/artifactory/api/nuget/v3/choco-virtual/index.json", serverDetails))
	assert.Empty(t, repoFromSource("unrelated", serverDetails))
}

func TestHasNativeAuthOverride(t *testing.T) {
	assert.True(t, hasNativeAuthOverride([]string{"-s=jfrt-acme.jfrog.io-choco-local"}))
	assert.True(t, hasNativeAuthOverride([]string{"--api-key", "key"}))
	assert.False(t, hasNativeAuthOverride([]string{"package", "--yes"}))
}

func TestValidateExplicitPushSource(t *testing.T) {
	serverDetails := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
	require.NoError(t, validateExplicitPushSource([]string{"-s=jfrt-acme.jfrog.io-choco-local"}, "choco-local", serverDetails))

	err := validateExplicitPushSource([]string{"--source=https://acme.jfrog.io/artifactory/api/nuget/choco-local"}, "choco-virtual", serverDetails)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matching source and --repo")
}

func TestRepoFromConfiguredSources(t *testing.T) {
	originalRunner := chocoSourceListRunner
	chocoSourceListRunner = func() (string, error) {
		return "jfrt-acme.jfrog.io-choco-local|https://acme.jfrog.io/artifactory/api/nuget/choco-local|false|||0\n" +
			"jfrt-acme.jfrog.io-choco-virtual|https://acme.jfrog.io/artifactory/api/nuget/choco-virtual|false|||1\n", nil
	}
	t.Cleanup(func() { chocoSourceListRunner = originalRunner })

	repo := repoFromConfiguredSources(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"})
	assert.Equal(t, "choco-virtual", repo)
}

func TestRepoFromConfiguredSourcesAvoidsAmbiguity(t *testing.T) {
	originalRunner := chocoSourceListRunner
	chocoSourceListRunner = func() (string, error) {
		return "source-one|https://acme.jfrog.io/artifactory/api/nuget/one|false|||1\n" +
			"source-two|https://acme.jfrog.io/artifactory/api/nuget/two|false|||1\n", nil
	}
	t.Cleanup(func() { chocoSourceListRunner = originalRunner })

	repo := repoFromConfiguredSources(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"})
	assert.Empty(t, repo)
}

func TestRepoFromConfiguredSourcesSkipsDisabledSource(t *testing.T) {
	originalRunner := chocoSourceListRunner
	chocoSourceListRunner = func() (string, error) {
		return "disabled|https://acme.jfrog.io/artifactory/api/nuget/disabled|true|||1\n" +
			"enabled|https://acme.jfrog.io/artifactory/api/nuget/enabled|false|||1\n", nil
	}
	t.Cleanup(func() { chocoSourceListRunner = originalRunner })

	repo := repoFromConfiguredSources(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"})
	assert.Equal(t, "enabled", repo)
}

func TestArtifactPatterns(t *testing.T) {
	patterns := artifactPatterns([]entities.Artifact{
		{OriginalDeploymentRepo: "choco-local", Path: "tool.1.0.0.nupkg"},
		{OriginalDeploymentRepo: "", Path: "ignored.nupkg"},
	})
	assert.Equal(t, []string{"choco-local/tool.1.0.0.nupkg"}, patterns)
}

func TestSetChocoCommandPropertyRedactsAPIKey(t *testing.T) {
	modules := []entities.Module{{}}
	setChocoCommandProperty(modules, "push", []string{"tool.1.0.0.nupkg", "--api-key=secret", "-k", "another-secret"})

	properties, ok := modules[0].Properties.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "push tool.1.0.0.nupkg --api-key=*** -k ***", properties[entities.BuildInfoEnvPrefix+"CHOCO_COMMAND"])
}

func TestBuildConfigurationDoesNotCollectWhenNativeOnly(t *testing.T) {
	configuration := buildutils.NewBuildConfiguration("", "", "", "")
	collects, err := configuration.IsCollectBuildInfo()
	require.NoError(t, err)
	assert.False(t, collects)
}

func TestChocoCommandConfiguration(t *testing.T) {
	serverDetails := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
	buildConfiguration := buildutils.NewBuildConfiguration("cli-choco-build", "7", "tool", "proj")
	args := []string{"tool.nuspec"}

	command := NewChocoFlexPackCommand().
		SetSubCommand("PACK").
		SetArgs(args).
		SetServerDetails(serverDetails).
		SetRepoResolve("choco-remote").
		SetRepoDeploy("choco-local").
		SetBuildConfiguration(buildConfiguration).
		SetWorkingDirectory(filepath.Join("some", "package", "dir"))

	assert.Equal(t, "rt_choco_flexpack", command.CommandName())
	assert.Equal(t, "pack", command.subCommand, "sub command should be normalized to lower case")
	assert.Equal(t, "choco-remote", command.repoResolve)
	assert.Equal(t, "choco-local", command.repoDeploy)
	assert.Equal(t, filepath.Join("some", "package", "dir"), command.workingDirectory)
	assert.Same(t, buildConfiguration, command.buildConfiguration)

	resolvedServerDetails, err := command.ServerDetails()
	require.NoError(t, err)
	assert.Same(t, serverDetails, resolvedServerDetails)

	// The command must not alias the caller's slice.
	args[0] = "mutated.nuspec"
	assert.Equal(t, []string{"tool.nuspec"}, command.args)
}

func TestResolveLocalDeployRepoShortCircuits(t *testing.T) {
	// Both of these branches return before a services manager is created, so they
	// are the only ones reachable without a live Artifactory.
	testCases := []struct {
		name          string
		repoName      string
		serverDetails *config.ServerDetails
		expected      string
	}{
		{
			name:          "no repo to resolve",
			repoName:      "",
			serverDetails: &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"},
			expected:      "",
		},
		{
			name:          "no server details",
			repoName:      "choco-local",
			serverDetails: nil,
			expected:      "choco-local",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resolved, err := NewChocoFlexPackCommand().
				SetServerDetails(testCase.serverDetails).
				resolveLocalDeployRepo(testCase.repoName)
			require.NoError(t, err)
			assert.Equal(t, testCase.expected, resolved)
		})
	}
}

func TestChocoCommandDefaultsWorkingDirectory(t *testing.T) {
	originalChecker := chocoPlatformChecker
	originalRunner := chocoNativeRunner
	chocoPlatformChecker = func() bool { return true }
	chocoNativeRunner = func([]string) error { return nil }
	t.Cleanup(func() {
		chocoPlatformChecker = originalChecker
		chocoNativeRunner = originalRunner
	})

	command := NewChocoFlexPackCommand().SetSubCommand("list")
	require.NoError(t, command.Run())

	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, workingDirectory, command.workingDirectory)
}

func TestChocoPackCollectsBuildInfoLocally(t *testing.T) {
	testCases := []struct {
		name              string
		module            string
		buildNumber       string
		omitBuildName     bool
		expectedError     string
		expectedModuleIDs []string
	}{
		{
			name:              "a module per packed package",
			buildNumber:       "1",
			expectedModuleIDs: []string{"cli-choco-tool:1.0.0", "cli-choco-tool-extras:2.1.0"},
		},
		{
			name:              "a single module when overridden",
			module:            "choco-module",
			buildNumber:       "2",
			expectedModuleIDs: []string{"choco-module"},
		},
		{
			// Build-info is collected only when both flags are given; half of the pair is a
			// mistake worth reporting rather than silently ignoring.
			name:          "a build name without a build number is rejected",
			buildNumber:   "",
			expectedError: "cannot be provided separately",
		},
		{
			name:              "no collection without either build flag",
			omitBuildName:     true,
			buildNumber:       "",
			expectedModuleIDs: nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Keep the build configuration hermetic - it otherwise falls back to env vars.
			t.Setenv(coreutils.BuildName, "")
			t.Setenv(coreutils.BuildNumber, "")

			workingDirectory := t.TempDir()
			packedPackages := []string{"cli-choco-tool.1.0.0.nupkg", "cli-choco-tool-extras.2.1.0.nupkg"}
			withFakeChoco(t, func([]string) error {
				// Emulate 'choco pack' dropping the packages in the working directory.
				for _, packedPackage := range packedPackages {
					if err := os.WriteFile(filepath.Join(workingDirectory, packedPackage), []byte(packedPackage), 0600); err != nil {
						return err
					}
				}
				return nil
			})

			buildName := uniqueChocoBuildName(t)
			if testCase.omitBuildName {
				buildName = ""
			}
			localBuild := requireLocalBuild(t, buildName, testCase.buildNumber)

			err := NewChocoFlexPackCommand().
				SetSubCommand("pack").
				SetArgs([]string{"cli-choco-tool.nuspec"}).
				SetRepoDeploy("choco-local").
				SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, testCase.buildNumber, testCase.module, "")).
				SetWorkingDirectory(workingDirectory).
				Run()
			if testCase.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), testCase.expectedError)
				return
			}
			require.NoError(t, err)

			if testCase.buildNumber == "" {
				// Without a build number there is nothing to read back.
				assert.Nil(t, localBuild)
				return
			}

			collected, err := localBuild.ToBuildInfo()
			require.NoError(t, err)
			moduleIDs := make([]string, 0, len(collected.Modules))
			for _, module := range collected.Modules {
				moduleIDs = append(moduleIDs, module.Id)
				assert.Equal(t, entities.Nuget, module.Type)
				assert.Equal(t, "pack cli-choco-tool.nuspec", moduleChocoCommand(t, module))
				for _, artifact := range module.Artifacts {
					assert.Equal(t, "choco-local", artifact.OriginalDeploymentRepo)
					assert.NotEmpty(t, artifact.Sha1, "packed artifacts should carry checksums")
					assert.Equal(t, artifact.Name, artifact.Path, "Chocolatey packages are deployed flat")
				}
			}
			assert.ElementsMatch(t, testCase.expectedModuleIDs, moduleIDs)
		})
	}
}

// withFakeChoco replaces the Chocolatey platform check and native runner for the
// duration of the test, so the command can be driven without a choco binary.
func withFakeChoco(t *testing.T, runner func(args []string) error) {
	t.Helper()
	originalChecker := chocoPlatformChecker
	originalRunner := chocoNativeRunner
	chocoPlatformChecker = func() bool { return true }
	chocoNativeRunner = runner
	t.Cleanup(func() {
		chocoPlatformChecker = originalChecker
		chocoNativeRunner = originalRunner
	})
}

// uniqueChocoBuildName keeps parallel and repeated runs from sharing a local build cache entry.
func uniqueChocoBuildName(t *testing.T) string {
	t.Helper()
	return "cli-choco-unit-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// requireLocalBuild opens the local build cache entry the command writes to and
// removes it when the test ends. It returns nil when no build number is given,
// since the command does not collect build info in that case.
func requireLocalBuild(t *testing.T, buildName, buildNumber string) *build.Build {
	t.Helper()
	if buildNumber == "" {
		return nil
	}
	localBuild, err := buildutils.CreateBuildInfoService().GetOrCreateBuildWithProject(buildName, buildNumber, "")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, localBuild.Clean()) })
	return localBuild
}

// moduleChocoCommand reads the recorded Chocolatey command line off a module.
// Module properties are typed as interface{}, so a build info that went through
// JSON carries a generic map rather than the map[string]string set in place.
func moduleChocoCommand(t *testing.T, module entities.Module) string {
	t.Helper()
	const propertyKey = entities.BuildInfoEnvPrefix + "CHOCO_COMMAND"
	switch properties := module.Properties.(type) {
	case map[string]string:
		return properties[propertyKey]
	case map[string]interface{}:
		value, ok := properties[propertyKey].(string)
		require.True(t, ok, "%s should be recorded as a string", propertyKey)
		return value
	default:
		t.Fatalf("unexpected module properties type %T", module.Properties)
		return ""
	}
}

func TestChocoPushCollectsBuildInfoLocally(t *testing.T) {
	t.Setenv(coreutils.BuildName, "")
	t.Setenv(coreutils.BuildNumber, "")

	workingDirectory := t.TempDir()
	packedPackage := filepath.Join(workingDirectory, "cli-choco-tool.1.0.0.nupkg")
	require.NoError(t, os.WriteFile(packedPackage, []byte("nupkg"), 0600))

	var received []string
	withFakeChoco(t, func(args []string) error {
		received = append([]string(nil), args...)
		return nil
	})

	buildName := uniqueChocoBuildName(t)
	localBuild := requireLocalBuild(t, buildName, "3")

	// Without server details the push runs purely natively, so no Artifactory call is made.
	err := NewChocoFlexPackCommand().
		SetSubCommand("push").
		SetArgs([]string{packedPackage, "--api-key=secret-token"}).
		SetRepoDeploy("choco-local").
		SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, "3", "", "")).
		SetWorkingDirectory(workingDirectory).
		Run()
	require.NoError(t, err)
	assert.Equal(t, []string{"push", packedPackage, "--api-key=secret-token"}, received)

	collected, err := localBuild.ToBuildInfo()
	require.NoError(t, err)
	require.Len(t, collected.Modules, 1)
	module := collected.Modules[0]
	assert.Equal(t, "cli-choco-tool:1.0.0", module.Id)
	assert.Equal(t, entities.Nuget, module.Type)
	require.Len(t, module.Artifacts, 1)
	assert.Equal(t, "cli-choco-tool.1.0.0.nupkg", module.Artifacts[0].Name)
	assert.Equal(t, "choco-local", module.Artifacts[0].OriginalDeploymentRepo)
	recordedCommand := moduleChocoCommand(t, module)
	assert.Contains(t, recordedCommand, "push")
	assert.NotContains(t, recordedCommand, "secret-token", "the api key must be redacted from the recorded command")
}

func TestStampBuildPropertiesSkipsWithoutTarget(t *testing.T) {
	artifact := entities.Artifact{Name: "tool.1.0.0.nupkg", Path: "tool.1.0.0.nupkg", OriginalDeploymentRepo: "choco-local"}
	// Each of these cases returns before a services manager is created.
	testCases := []struct {
		name      string
		command   *ChocoFlexPackCommand
		artifacts []entities.Artifact
	}{
		{
			name:      "no server details",
			command:   NewChocoFlexPackCommand().SetRepoDeploy("choco-local"),
			artifacts: []entities.Artifact{artifact},
		},
		{
			name:      "no deploy repo",
			command:   NewChocoFlexPackCommand().SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}),
			artifacts: []entities.Artifact{artifact},
		},
		{
			name:      "no artifacts",
			command:   NewChocoFlexPackCommand().SetRepoDeploy("choco-local").SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}),
			artifacts: nil,
		},
		{
			name:      "artifacts without a deployment path",
			command:   NewChocoFlexPackCommand().SetRepoDeploy("choco-local").SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}),
			artifacts: []entities.Artifact{{Name: "tool.1.0.0.nupkg"}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.NoError(t, testCase.command.stampBuildProperties(testCase.artifacts, "cli-choco-build", "1"))
		})
	}
}

func TestSearchWithRetry(t *testing.T) {
	patterns := []string{"choco-local/tool.1.0.0.nupkg"}

	t.Run("returns on the first successful search", func(t *testing.T) {
		attempts := 0
		count, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			return 2, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.Equal(t, 1, attempts)
	})

	t.Run("gives an empty result exactly one re-attempt", func(t *testing.T) {
		attempts := 0
		count, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			if attempts < 2 {
				return 0, nil
			}
			return 1, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 2, attempts, "an empty result must not consume the transient-failure retries")
	})

	t.Run("fails with the sentinel when the recorded paths stay empty", func(t *testing.T) {
		attempts := 0
		_, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			return 0, nil
		})
		require.ErrorIs(t, err, errChocoArtifactsNotFound)
		assert.Contains(t, err.Error(), "choco-local/tool.1.0.0.nupkg")
		assert.Contains(t, err.Error(), "no build-info was saved")
		// The message must name both causes an empty search cannot tell apart.
		assert.Contains(t, err.Error(), "the repository or path recorded in the build-info is wrong")
		assert.Contains(t, err.Error(), "no read permission")
		assert.Equal(t, 2, attempts, "an empty result gets one re-attempt and then fails hard")
	})

	t.Run("retries transient failures and succeeds", func(t *testing.T) {
		attempts := 0
		count, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			if attempts < 3 {
				return 0, httpError(http.StatusServiceUnavailable)
			}
			return 1, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 3, attempts)
	})

	t.Run("returns the last transient failure once the retries are exhausted", func(t *testing.T) {
		attempts := 0
		_, err := searchWithRetry(2, 0, 0, patterns, func() (int, error) {
			attempts++
			return 0, httpError(http.StatusBadGateway)
		})
		require.Error(t, err)
		assert.NotErrorIs(t, err, errChocoArtifactsNotFound)
		assert.Equal(t, 3, attempts, "MaxRetries=2 allows one initial attempt plus two retries")
	})

	t.Run("fails fast on a non-retryable status", func(t *testing.T) {
		attempts := 0
		_, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			return 0, httpError(http.StatusBadRequest)
		})
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})

	t.Run("fails fast on an unclassified error", func(t *testing.T) {
		attempts := 0
		_, err := searchWithRetry(3, 0, 0, patterns, func() (int, error) {
			attempts++
			return 0, errors.New("malformed AQL response")
		})
		assert.EqualError(t, err, "malformed AQL response")
		assert.Equal(t, 1, attempts)
	})
}

func TestIsRetryableError(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "nil", err: nil, retryable: false},
		{name: "internal server error", err: httpError(http.StatusInternalServerError), retryable: true},
		{name: "bad gateway", err: httpError(http.StatusBadGateway), retryable: true},
		{name: "too many requests", err: httpError(http.StatusTooManyRequests), retryable: true},
		{name: "forbidden may be gateway throttling", err: httpError(http.StatusForbidden), retryable: true},
		{name: "unauthorized", err: httpError(http.StatusUnauthorized), retryable: false},
		{name: "not found", err: httpError(http.StatusNotFound), retryable: false},
		{name: "wrapped retryable status", err: fmt.Errorf("search: %w", httpError(http.StatusServiceUnavailable)), retryable: true},
		{name: "connection reset", err: fmt.Errorf("read tcp: %w", syscall.ECONNRESET), retryable: true},
		{name: "connection refused", err: syscall.ECONNREFUSED, retryable: true},
		{name: "broken pipe", err: syscall.EPIPE, retryable: true},
		{name: "unexpected eof", err: io.ErrUnexpectedEOF, retryable: true},
		{name: "timeout", err: &url.Error{Op: "Post", URL: "https://acme.jfrog.io", Err: timeoutError{}}, retryable: true},
		{name: "plain error", err: errors.New("boom"), retryable: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.retryable, isRetryableError(testCase.err))
		})
	}
}

func TestIsForbiddenError(t *testing.T) {
	assert.True(t, isForbiddenError(fmt.Errorf("set props: %w", httpError(http.StatusForbidden))))
	assert.False(t, isForbiddenError(httpError(http.StatusInternalServerError)))
	assert.False(t, isForbiddenError(errors.New("boom")))
	assert.False(t, isForbiddenError(nil))
}

func TestSetPropsWithRetry(t *testing.T) {
	t.Run("retries a transient failure", func(t *testing.T) {
		attempts := 0
		err := setPropsWithRetry(3, 0, func() error {
			attempts++
			if attempts < 2 {
				return httpError(http.StatusTooManyRequests)
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})

	t.Run("keeps a persistent forbidden failure", func(t *testing.T) {
		attempts := 0
		err := setPropsWithRetry(2, 0, func() error {
			attempts++
			return httpError(http.StatusForbidden)
		})
		require.Error(t, err)
		assert.True(t, isForbiddenError(err))
		assert.Equal(t, 3, attempts)
	})

	t.Run("fails fast on a non-retryable status", func(t *testing.T) {
		attempts := 0
		err := setPropsWithRetry(3, 0, func() error {
			attempts++
			return httpError(http.StatusNotFound)
		})
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func TestPersistBuildInfoAfterStamping(t *testing.T) {
	artifacts := []entities.Artifact{{Name: "tool.1.0.0.nupkg", Path: "tool.1.0.0.nupkg", OriginalDeploymentRepo: "choco-local"}}

	requireSavedArtifact := func(t *testing.T, localBuild *build.Build) {
		t.Helper()
		collected, err := localBuild.ToBuildInfo()
		require.NoError(t, err)
		require.Len(t, collected.Modules, 1)
		require.Len(t, collected.Modules[0].Artifacts, 1)
		assert.Equal(t, "tool.1.0.0.nupkg", collected.Modules[0].Artifacts[0].Name)
	}

	t.Run("saves the build info when stamping succeeds", func(t *testing.T) {
		buildName := uniqueChocoBuildName(t)
		localBuild := requireLocalBuild(t, buildName, "1")
		command := NewChocoFlexPackCommand().SetSubCommand("push").
			SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, "1", "", ""))
		require.NoError(t, command.persistBuildInfoAfterStamping(nil, buildName, "1", artifacts))
		requireSavedArtifact(t, localBuild)
	})

	t.Run("does not save the build info when the recorded paths are wrong", func(t *testing.T) {
		buildName := uniqueChocoBuildName(t)
		localBuild := requireLocalBuild(t, buildName, "1")
		command := NewChocoFlexPackCommand().SetSubCommand("push").
			SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, "1", "", ""))
		stampErr := fmt.Errorf("%w: choco-local/tool.1.0.0.nupkg", errChocoArtifactsNotFound)
		err := command.persistBuildInfoAfterStamping(stampErr, buildName, "1", artifacts)
		require.ErrorIs(t, err, errChocoArtifactsNotFound)
		collected, buildInfoErr := localBuild.ToBuildInfo()
		require.NoError(t, buildInfoErr)
		assert.Empty(t, collected.Modules, "a build-info with unverifiable paths must not be persisted")
	})

	t.Run("saves the build info and still fails on a stamping failure", func(t *testing.T) {
		buildName := uniqueChocoBuildName(t)
		localBuild := requireLocalBuild(t, buildName, "1")
		command := NewChocoFlexPackCommand().SetSubCommand("push").
			SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, "1", "", ""))
		stampErr := fmt.Errorf("stamp build properties: %w", httpError(http.StatusForbidden))
		err := command.persistBuildInfoAfterStamping(stampErr, buildName, "1", artifacts)
		require.ErrorIs(t, err, stampErr)
		requireSavedArtifact(t, localBuild)
	})
}

// timeoutError is a net.Error whose Timeout reports true.
type timeoutError struct{}

func (timeoutError) Error() string { return "i/o timeout" }
func (timeoutError) Timeout() bool { return true }

//nolint:staticcheck // net.Error requires Temporary even though it is deprecated.
func (timeoutError) Temporary() bool { return false }

// httpError builds the error type Artifactory responses are wrapped in, so the classification
// helpers can be exercised without a live server.
func httpError(statusCode int) error {
	return &errorutils.HttpResponseError{StatusCode: statusCode, Status: fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))}
}

func TestChocoCommandRunFailures(t *testing.T) {
	serverDetails := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}

	t.Run("pack fails when the working directory is unreadable", func(t *testing.T) {
		withFakeChoco(t, func([]string) error {
			t.Error("the native command must not run when the package snapshot fails")
			return nil
		})
		err := NewChocoFlexPackCommand().
			SetSubCommand("pack").
			SetWorkingDirectory(filepath.Join(t.TempDir(), "missing")).
			Run()
		assert.Error(t, err)
	})

	t.Run("push rejects a source that contradicts the repo", func(t *testing.T) {
		withFakeChoco(t, func([]string) error {
			t.Error("the native command must not run when the push source is rejected")
			return nil
		})
		err := NewChocoFlexPackCommand().
			SetSubCommand("push").
			SetArgs([]string{"tool.1.0.0.nupkg", "--source=https://acme.jfrog.io/artifactory/api/nuget/choco-local"}).
			SetRepoDeploy("choco-other").
			SetServerDetails(serverDetails).
			SetWorkingDirectory(t.TempDir()).
			Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "matching source and --repo")
	})

	t.Run("pack reports artifact collection failures", func(t *testing.T) {
		t.Setenv(coreutils.BuildName, "")
		t.Setenv(coreutils.BuildNumber, "")

		workingDirectory := t.TempDir()
		withFakeChoco(t, func([]string) error {
			// Losing the working directory between the snapshot and the collection is
			// what a partial 'choco pack' failure looks like from here.
			return os.RemoveAll(workingDirectory)
		})
		buildName := uniqueChocoBuildName(t)
		requireLocalBuild(t, buildName, "4")

		err := NewChocoFlexPackCommand().
			SetSubCommand("pack").
			SetArgs([]string{"cli-choco-tool.nuspec"}).
			SetBuildConfiguration(buildutils.NewBuildConfiguration(buildName, "4", "", "")).
			SetWorkingDirectory(workingDirectory).
			Run()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "collect packed Chocolatey artifacts")
	})

	t.Run("push without a build configuration skips collection", func(t *testing.T) {
		workingDirectory := t.TempDir()
		withFakeChoco(t, func([]string) error { return nil })
		assert.NoError(t, NewChocoFlexPackCommand().
			SetSubCommand("push").
			SetArgs([]string{"tool.1.0.0.nupkg"}).
			SetWorkingDirectory(workingDirectory).
			Run())
	})
}

func TestHasNativeResolveOverride(t *testing.T) {
	// Anything that already points the native command at a source, or authenticates one, must stop
	// --repo-resolve from adding a second one.
	assert.True(t, hasNativeResolveOverride([]string{"-s=jfrt-acme.jfrog.io-choco-local"}))
	assert.True(t, hasNativeResolveOverride([]string{"--source", "https://acme.jfrog.io/artifactory/api/nuget/choco-local"}))
	assert.True(t, hasNativeResolveOverride([]string{"-u", "admin"}))
	assert.True(t, hasNativeResolveOverride([]string{"--user=admin"}))
	assert.True(t, hasNativeResolveOverride([]string{"-p", "secret"}))
	assert.True(t, hasNativeResolveOverride([]string{"--password=secret"}))
	assert.False(t, hasNativeResolveOverride([]string{"package", "--yes"}))
	assert.False(t, hasNativeResolveOverride([]string{"--prerelease", "--package-parameters=/silent"}))
}

func TestChocoResolveRedirectsTheNativeSource(t *testing.T) {
	for _, subCommand := range []string{"install", "upgrade"} {
		t.Run(subCommand+" with credentials", func(t *testing.T) {
			var received []string
			withFakeChoco(t, func(args []string) error {
				received = append([]string(nil), args...)
				return nil
			})

			require.NoError(t, NewChocoFlexPackCommand().
				SetSubCommand(subCommand).
				SetArgs([]string{"tool", "--yes"}).
				SetRepoResolve("choco-virtual").
				SetServerDetails(&config.ServerDetails{
					ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
					User:           "admin",
					Password:       "secret",
				}).
				SetWorkingDirectory(t.TempDir()).
				Run())

			assert.Equal(t, []string{subCommand, "tool", "--yes",
				"-s=https://acme.jfrog.io/artifactory/api/nuget/choco-virtual",
				"-u=admin", "-p=secret"}, received)
		})
	}

	t.Run("anonymous resolution still redirects the source", func(t *testing.T) {
		var received []string
		withFakeChoco(t, func(args []string) error {
			received = append([]string(nil), args...)
			return nil
		})

		// A repository that allows anonymous reads is a legitimate setup, so missing credentials
		// must not fail the command - only leave out -u/-p.
		require.NoError(t, NewChocoFlexPackCommand().
			SetSubCommand("install").
			SetArgs([]string{"tool"}).
			SetRepoResolve("choco-virtual").
			SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}).
			SetWorkingDirectory(t.TempDir()).
			Run())

		assert.Equal(t, []string{"install", "tool",
			"-s=https://acme.jfrog.io/artifactory/api/nuget/choco-virtual"}, received)
	})

	t.Run("a native source wins", func(t *testing.T) {
		var received []string
		withFakeChoco(t, func(args []string) error {
			received = append([]string(nil), args...)
			return nil
		})

		require.NoError(t, NewChocoFlexPackCommand().
			SetSubCommand("install").
			SetArgs([]string{"tool", "-s=jfrt-acme.jfrog.io-choco-local"}).
			SetRepoResolve("choco-virtual").
			SetServerDetails(&config.ServerDetails{
				ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
				User:           "admin",
				Password:       "secret",
			}).
			SetWorkingDirectory(t.TempDir()).
			Run())

		assert.Equal(t, []string{"install", "tool", "-s=jfrt-acme.jfrog.io-choco-local"}, received)
	})
}

func TestRedactChocoArgs(t *testing.T) {
	assert.Equal(t,
		[]string{"install", "tool", "-p=***", "--password", "***", "--proxy-password=***",
			"--cp", "***", "--certpassword=***", "-k", "***", "--api-key=***", "--apikey", "***"},
		redactChocoArgs([]string{"install", "tool", "-p=secret", "--password", "secret",
			"--proxy-password=proxy", "--cp", "certificate", "--certpassword=certificate",
			"-k", "token", "--api-key=token", "--apikey", "token"}))

	// A username is not a secret, and nothing that merely starts with a redacted flag may be
	// swallowed: --params and --prerelease must survive untouched.
	assert.Equal(t,
		[]string{"install", "tool", "-u=admin", "--user", "admin", "--params=/silent",
			"--package-parameters", "/silent", "--prerelease"},
		redactChocoArgs([]string{"install", "tool", "-u=admin", "--user", "admin", "--params=/silent",
			"--package-parameters", "/silent", "--prerelease"}))
}

// "-v" is Chocolatey's global --verbose switch, not a short form of --version, so the package that
// follows it must still be recorded as a requested package.
func TestRequestedPackagesKeepsThePackageAfterVerbose(t *testing.T) {
	assert.Equal(t, []string{"tool"}, requestedPackages([]string{"-v", "tool"}))
	assert.Equal(t, []string{"tool"}, requestedPackages([]string{"--verbose", "tool"}))
	assert.Equal(t, []string{"tool"}, requestedPackages([]string{"--version", "1.0.0", "tool"}))
}

func TestPackNuspecPaths(t *testing.T) {
	workingDirectory := t.TempDir()
	named := filepath.Join(workingDirectory, "tool.nuspec")
	require.NoError(t, os.WriteFile(named, []byte("<package/>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workingDirectory, "other.nuspec"), []byte("<package/>"), 0o600))

	// A manifest named on the command line is the only one choco packs.
	assert.Equal(t, []string{named}, packNuspecPaths(workingDirectory, []string{"tool.nuspec", "--yes"}))
	assert.Equal(t, []string{named}, packNuspecPaths(workingDirectory, []string{named}))

	// With no path, choco packs every manifest in the directory.
	assert.ElementsMatch(t,
		[]string{named, filepath.Join(workingDirectory, "other.nuspec")},
		packNuspecPaths(workingDirectory, nil))

	assert.Empty(t, packNuspecPaths(t.TempDir(), nil))
	assert.Empty(t, packNuspecPaths(filepath.Join(t.TempDir(), "missing"), nil))
}

func TestWarnUnsubstitutedNuspecTokens(t *testing.T) {
	logged := &bytes.Buffer{}
	originalLogger := log.Logger
	log.SetLogger(log.NewLogger(log.WARN, logged))
	t.Cleanup(func() { log.Logger = originalLogger })

	workingDirectory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workingDirectory, "tool.nuspec"),
		[]byte(`<package><metadata><id>$id$</id><version>$version$</version></metadata></package>`), 0o600))
	warnUnsubstitutedNuspecTokens(workingDirectory, nil)
	warning := logged.String()
	assert.Contains(t, warning, "$id$")
	assert.Contains(t, warning, "$version$")
	assert.Contains(t, warning, "tool.nuspec")

	// A manifest with literal metadata is packed as written, so there is nothing to warn about.
	logged.Reset()
	literal := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(literal, "tool.nuspec"),
		[]byte(`<package><metadata><id>tool</id><version>1.0.0</version></metadata></package>`), 0o600))
	warnUnsubstitutedNuspecTokens(literal, nil)
	assert.Empty(t, logged.String())
}

func TestUniqueStrings(t *testing.T) {
	assert.Equal(t, []string{"$version$", "$id$"},
		uniqueStrings([]string{"$version$", "$id$", "$version$"}))
	assert.Empty(t, uniqueStrings(nil))
}
