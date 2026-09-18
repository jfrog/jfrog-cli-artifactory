package psresource

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	buildutils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── argument parsing helpers ─────────────────────────────────────────────────

func TestArgValue(t *testing.T) {
	assert.Equal(t, "myrepo", argValue([]string{"-Name", "Foo", "-Repository", "myrepo"}, "-Repository"))
	assert.Equal(t, "myrepo", argValue([]string{"-Repository:myrepo"}, "-Repository"))
	assert.Equal(t, "myrepo", argValue([]string{"-REPOSITORY", "myrepo"}, "-repository"))
	assert.Equal(t, "", argValue([]string{"-Name", "Foo"}, "-Repository"))
	assert.Equal(t, "", argValue([]string{"-Repository"}, "-Repository"))
}

func TestArgValues(t *testing.T) {
	assert.Equal(t, []string{"Foo", "Bar", "Baz"}, argValues([]string{"-Name", "Foo,Bar,Baz"}, "-Name"))
	assert.Equal(t, []string{"Foo"}, argValues([]string{"-Name:Foo"}, "-Name"))
	assert.Nil(t, argValues([]string{"-Repository", "myrepo"}, "-Name"))
}

func TestSplitCommaList(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, splitCommaList("a, b"))
	assert.Equal(t, []string{"a"}, splitCommaList("a"))
	assert.Empty(t, splitCommaList(""))
}

// ── credential-override / redaction ──────────────────────────────────────────

func TestHasNativeCredentialOverride(t *testing.T) {
	assert.True(t, hasNativeCredentialOverride([]string{"-Name", "Foo", "-Credential", "$cred"}))
	assert.True(t, hasNativeCredentialOverride([]string{"-ApiKey", "abc"}))
	assert.True(t, hasNativeCredentialOverride([]string{"-Credential:$cred"}))
	assert.False(t, hasNativeCredentialOverride([]string{"-Name", "Foo", "-Repository", "myrepo"}))
}

func TestRedactPSResourceArgs(t *testing.T) {
	redacted := redactPSResourceArgs([]string{"-Name", "Foo", "-ApiKey", "sup3rs3cr3t", "-Repository", "myrepo"})
	assert.Equal(t, []string{"-Name", "Foo", "-ApiKey", "***", "-Repository", "myrepo"}, redacted)
	assert.NotContains(t, redacted, "sup3rs3cr3t")

	// The original slice must not be mutated - redactPSResourceArgs is used for logging only, the
	// real args (with the real secret) still have to reach the native command.
	original := []string{"-ApiKey", "sup3rs3cr3t"}
	_ = redactPSResourceArgs(original)
	assert.Equal(t, "sup3rs3cr3t", original[1])
}

// ── PowerShell script construction ───────────────────────────────────────────

func TestPsresourceCommandLineArg(t *testing.T) {
	assert.Equal(t, "-Name", psresourceCommandLineArg("-Name"))
	assert.Equal(t, "'Foo'", psresourceCommandLineArg("Foo"))
	assert.Equal(t, "'it''s escaped'", psresourceCommandLineArg("it's escaped"))
}

func TestPsresourceQuoteLiteral(t *testing.T) {
	assert.Equal(t, "'C:\\Modules\\Foo'", psresourceQuoteLiteral(`C:\Modules\Foo`))
	assert.Equal(t, "'it''s'", psresourceQuoteLiteral("it's"))
}

func TestCmdletInvocationRedactsUserSuppliedSecrets(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Path", "./MyModule", "-ApiKey", "sup3rs3cr3t"})
	invocation := command.debugScript()
	assert.NotContains(t, invocation, "sup3rs3cr3t")
	assert.Contains(t, invocation, "***")
	assert.Contains(t, invocation, SubCommandPublish)
}

func TestBuildScriptNeverEmbedsResolvedCredentialValue(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandInstall).
		SetArgs([]string{"-Name", "Foo", "-Repository", "myrepo"})
	script := command.buildScript(true)
	assert.Contains(t, script, "$env:JFROG_PSRESOURCE_TOKEN")
	assert.Contains(t, script, "$env:JFROG_PSRESOURCE_USER")
	assert.Contains(t, script, "-Credential $cred")
	assert.NotContains(t, script, "sup3rs3cr3t")

	withoutCredential := command.buildScript(false)
	assert.NotContains(t, withoutCredential, "-Credential")
	assert.NotContains(t, withoutCredential, "$env:")
}

// ── credential resolution ────────────────────────────────────────────────────

func TestResolveCredentialEnvInjectsWhenRepoAndCredentialsPresent(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/", User: "john", Password: "secret"})

	env, inject, err := command.resolveCredentialEnv("myrepo")
	require.NoError(t, err)
	assert.True(t, inject)
	assert.Contains(t, env, "JFROG_PSRESOURCE_USER=john")
	assert.Contains(t, env, "JFROG_PSRESOURCE_TOKEN=secret")
}

func TestResolveCredentialEnvSkipsWhenNoRepo(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/", User: "john", Password: "secret"})
	env, inject, err := command.resolveCredentialEnv("")
	require.NoError(t, err)
	assert.False(t, inject)
	assert.Nil(t, env)
}

func TestResolveCredentialEnvSkipsWhenUserSuppliedOwnCredential(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/", User: "john", Password: "secret"}).
		SetArgs([]string{"-Credential", "$myCred"})
	env, inject, err := command.resolveCredentialEnv("myrepo")
	require.NoError(t, err)
	assert.False(t, inject)
	assert.Nil(t, env)
}

func TestResolveCredentialEnvAnonymousIsNotAnError(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"})
	env, inject, err := command.resolveCredentialEnv("myrepo")
	require.NoError(t, err)
	assert.False(t, inject)
	assert.Nil(t, env)
}

func TestCredentialInjectionRepo(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetRepoDeploy("deployRepo").SetRepoResolve("resolveRepo")

	command.SetSubCommand(SubCommandPublish)
	assert.Equal(t, "deployRepo", command.credentialInjectionRepo())

	command.SetSubCommand(SubCommandInstall)
	assert.Equal(t, "resolveRepo", command.credentialInjectionRepo())

	command.SetSubCommand(SubCommandSave)
	assert.Equal(t, "resolveRepo", command.credentialInjectionRepo())

	command.SetSubCommand(SubCommandUpdate)
	assert.Equal(t, "resolveRepo", command.credentialInjectionRepo())

	command.SetSubCommand("Find-PSResource")
	assert.Equal(t, "", command.credentialInjectionRepo())
}

// ── Run() orchestration ───────────────────────────────────────────────────────

func TestRunFailsWhenPlatformUnavailable(t *testing.T) {
	stubValidatePlatform(t, assertErr("no pwsh"))
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall)
	err := command.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pwsh")
}

func TestRunRejectsHalfSpecifiedBuildParams(t *testing.T) {
	stubValidatePlatform(t, nil)
	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetBuildName("my-build")
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall).SetBuildConfiguration(buildConfig)
	err := command.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build-name and build-number")
}

func TestRunPassthroughWithoutBuildInfo(t *testing.T) {
	stubValidatePlatform(t, nil)
	stubResolveShell(t, "pwsh", nil)
	var capturedShell, capturedScript string
	restore := stubNativeRunner(t, func(shell, script string, env []string, workingDirectory string) error {
		capturedShell = shell
		capturedScript = script
		return nil
	})
	defer restore()

	command := NewPSResourceFlexPackCommand().
		SetSubCommand("Find-PSResource").
		SetArgs([]string{"-Name", "Foo"})
	require.NoError(t, command.Run())
	assert.Equal(t, "pwsh", capturedShell)
	assert.Contains(t, capturedScript, "Find-PSResource")
	assert.Contains(t, capturedScript, "'Foo'")
}

func TestRunSurfacesNativeCommandFailure(t *testing.T) {
	stubValidatePlatform(t, nil)
	stubResolveShell(t, "pwsh", nil)
	restore := stubNativeRunner(t, func(string, string, []string, string) error {
		return assertErr("boom")
	})
	defer restore()

	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall)
	err := command.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), SubCommandInstall)
	assert.Contains(t, err.Error(), "boom")
}

// ── collectDependencies / collectPublishArtifacts (with a real HEAD server) ─

func TestCollectDependenciesCollectsResolvedPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method, "checksum lookups must use HEAD, never GET")
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.Header().Set("X-Checksum-Sha1", "sha1value")
		w.Header().Set("X-Checksum-Md5", "md5value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stubValidatePlatform(t, nil)
	stubResolveShell(t, "pwsh", nil)
	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":"1.2.3"}`, nil
	})
	defer restoreQuery()

	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetBuildName("my-build")
	buildConfig.SetBuildNumber("1")
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandInstall).
		SetArgs([]string{"-Name", "Foo", "-Repository", "myrepo"}).
		SetRepoResolve("myrepo").
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetBuildConfiguration(buildConfig).
		SetWorkingDirectory(t.TempDir())

	err := command.collectDependencies("my-build", "1")
	require.NoError(t, err)
}

func TestCollectDependenciesFailsWithoutName(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall).SetRepoResolve("myrepo")
	err := command.collectDependencies("b", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-Name")
}

func TestCollectDependenciesFailsWithoutRepoResolve(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall).SetArgs([]string{"-Name", "Foo"})
	err := command.collectDependencies("b", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--repo-resolve")
}

func TestCollectPublishArtifactsWithExplicitNameVersion(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stubResolveShell(t, "pwsh", nil)
	// Property stamping drives a real Artifactory search-and-SetProps round trip that this
	// lightweight HEAD-only fake server cannot serve; it is exercised on its own in
	// TestStampBuildPropertiesNoopsWithoutServerDetails and covered end-to-end outside this suite.
	// This test's job is the checksum-fetch/build-info-assembly path collectPublishArtifacts owns.
	defer stubStampBuildProperties(t, nil)()
	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetBuildName("my-build")
	buildConfig.SetBuildNumber("1")
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Name", "Foo", "-Version", "1.2.3", "-Repository", "myrepo"}).
		SetRepoDeploy("myrepo").
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetBuildConfiguration(buildConfig).
		SetWorkingDirectory(t.TempDir())

	err := command.collectPublishArtifacts("my-build", "1")
	require.NoError(t, err)
	assert.Contains(t, requestedPath, "foo/1.2.3/Foo.1.2.3.nupkg")
}

func TestCollectPublishArtifactsNotFoundIsAClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	stubResolveShell(t, "pwsh", nil)
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Name", "Foo", "-Version", "1.2.3", "-Repository", "myrepo"}).
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(t.TempDir())

	err := command.collectPublishArtifacts("my-build", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was not found in Artifactory")
}

func TestCollectPublishArtifactsResolvesNameVersionFromManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "MyModule.psd1")
	require.NoError(t, os.WriteFile(manifestPath, []byte("@{ ModuleVersion = '9.9.9' }"), 0600))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stubResolveShell(t, "pwsh", nil)
	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return "9.9.9\n", nil
	})
	defer restoreQuery()
	defer stubStampBuildProperties(t, nil)()

	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Path", dir, "-Repository", "myrepo"}).
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(dir)

	err := command.collectPublishArtifacts("my-build", "1")
	require.NoError(t, err)
}

// ── installed-version resolution ─────────────────────────────────────────────

func TestResolveInstalledVersionSingleObject(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":"1.0.0"}`, nil
	})
	defer restore()
	version, err := resolveInstalledVersion("pwsh", "Foo")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", version)
}

func TestResolveInstalledVersionArrayPicksLatest(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `[{"Name":"Foo","Version":"1.0.0"},{"Name":"Foo","Version":"2.0.0"}]`, nil
	})
	defer restore()
	version, err := resolveInstalledVersion("pwsh", "Foo")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", version)
}

func TestResolveInstalledVersionEmptyOutputIsAnError(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return "", nil
	})
	defer restore()
	_, err := resolveInstalledVersion("pwsh", "Foo")
	require.Error(t, err)
}

func TestLatestInstalledVersion(t *testing.T) {
	list := []installedPSResource{{Version: "1.0.0"}, {Version: "1.10.0"}, {Version: "1.9.0"}}
	// Best-effort plain string comparison, documented as not SemVer-correct.
	assert.Equal(t, "1.9.0", latestInstalledVersion(list))
}

// ── build-info persistence / module id ───────────────────────────────────────

func TestModuleIDDefaultsWhenNoBuildConfiguration(t *testing.T) {
	command := NewPSResourceFlexPackCommand()
	assert.Equal(t, "psresource-project", command.moduleID())
}

func TestModuleIDUsesBuildConfigurationModule(t *testing.T) {
	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetModule("my-module")
	command := NewPSResourceFlexPackCommand().SetBuildConfiguration(buildConfig)
	assert.Equal(t, "my-module", command.moduleID())
}

func TestArtifactPatterns(t *testing.T) {
	patterns := artifactPatterns([]entities.Artifact{
		{OriginalDeploymentRepo: "myrepo", Path: "/foo/1.0.0/Foo.1.0.0.nupkg"},
		{OriginalDeploymentRepo: "", Path: "ignored"},
	})
	assert.Equal(t, []string{"myrepo/foo/1.0.0/Foo.1.0.0.nupkg"}, patterns)
}

func TestPersistArtifactBuildInfoReturnsNotFoundWithoutSaving(t *testing.T) {
	command := NewPSResourceFlexPackCommand()
	err := command.persistArtifactBuildInfo(errPSResourcePackageNotFound, "b", "1", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errPSResourcePackageNotFound)
}

// ── test doubles ──────────────────────────────────────────────────────────────

// TestStampBuildPropertiesEndToEnd exercises the real search-then-SetProps flow against a fake
// Artifactory that implements just enough of the AQL search and property-setting APIs to satisfy
// generic.SearchItems / servicesManager.SetProps.
func TestStampBuildPropertiesEndToEnd(t *testing.T) {
	var propsSet []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/artifactory/api/system/ping":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"repo":"myrepo","path":"foo/1.2.3","name":"Foo.1.2.3.nupkg"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/storage/"):
			propsSet = append(propsSet, r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer server.Close()

	command := NewPSResourceFlexPackCommand().
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(t.TempDir())

	artifacts := []entities.Artifact{{
		Name:                   "Foo.1.2.3.nupkg",
		OriginalDeploymentRepo: "myrepo",
		Path:                   "foo/1.2.3/Foo.1.2.3.nupkg",
	}}
	err := command.stampBuildProperties(artifacts, "my-build", "1")
	require.NoError(t, err)
	require.Len(t, propsSet, 1)
	assert.Contains(t, propsSet[0], "build.name")
	assert.Contains(t, propsSet[0], "build.number")
}

func assertErr(msg string) error { return errString(msg) }

type errString string

func (e errString) Error() string { return string(e) }

func stubValidatePlatform(t *testing.T, err error) {
	t.Helper()
	original := validatePSResourcePlatform
	validatePSResourcePlatform = func() error { return err }
	t.Cleanup(func() { validatePSResourcePlatform = original })
}

func stubResolveShell(t *testing.T, shell string, err error) {
	t.Helper()
	original := resolvePSResourceShell
	resolvePSResourceShell = func() (string, error) { return shell, err }
	t.Cleanup(func() { resolvePSResourceShell = original })
}

func stubNativeRunner(t *testing.T, fn func(shell, script string, env []string, workingDirectory string) error) func() {
	t.Helper()
	original := psresourceNativeRunner
	psresourceNativeRunner = fn
	return func() { psresourceNativeRunner = original }
}

func stubQueryRunner(t *testing.T, fn func(shell, script string) (string, error)) func() {
	t.Helper()
	original := psresourceQueryRunner
	psresourceQueryRunner = fn
	return func() { psresourceQueryRunner = original }
}

func stubStampBuildProperties(t *testing.T, err error) func() {
	t.Helper()
	original := stampBuildPropertiesFn
	stampBuildPropertiesFn = func(*PSResourceFlexPackCommand, []entities.Artifact, string, string) error { return err }
	return func() { stampBuildPropertiesFn = original }
}

// ── nil buildConfiguration safety ────────────────────────────────────────────

func TestPersistArtifactBuildInfoWithNilBuildConfiguration(t *testing.T) {
	// command.buildConfiguration is nil here on purpose: BuildConfiguration.GetProject/GetModule
	// dereference their receiver directly and panic on nil, so every call site in this package
	// must go through the nil-safe projectKey()/moduleID() accessors instead of calling those
	// methods on command.buildConfiguration directly.
	command := NewPSResourceFlexPackCommand()
	err := command.persistArtifactBuildInfo(nil, "my-build", "1", []entities.Artifact{{Name: "Foo.1.0.0.nupkg"}})
	require.NoError(t, err)
}

func TestStampBuildPropertiesNoopsWithoutServerDetails(t *testing.T) {
	command := NewPSResourceFlexPackCommand()
	require.NoError(t, command.stampBuildProperties([]entities.Artifact{{Name: "Foo"}}, "b", "1"))
}

func TestStampBuildPropertiesNoopsWithoutArtifacts(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetServerDetails(&config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"})
	require.NoError(t, command.stampBuildProperties(nil, "b", "1"))
}

func TestCommandNameAndServerDetails(t *testing.T) {
	serverDetails := &config.ServerDetails{ArtifactoryUrl: "https://acme.jfrog.io/artifactory/"}
	command := NewPSResourceFlexPackCommand().SetServerDetails(serverDetails)
	assert.Equal(t, "rt_psresource_flexpack", command.CommandName())
	got, err := command.ServerDetails()
	require.NoError(t, err)
	assert.Same(t, serverDetails, got)
}

func TestIsRetryableError(t *testing.T) {
	assert.False(t, isRetryableError(nil))
	assert.True(t, isRetryableError(&errorutils.HttpResponseError{StatusCode: http.StatusInternalServerError}))
	assert.True(t, isRetryableError(&errorutils.HttpResponseError{StatusCode: http.StatusTooManyRequests}))
	assert.False(t, isRetryableError(&errorutils.HttpResponseError{StatusCode: http.StatusNotFound}))
	assert.True(t, isRetryableError(io.EOF))
	assert.False(t, isRetryableError(errString("plain error")))
}

func TestIsForbiddenError(t *testing.T) {
	assert.True(t, isForbiddenError(&errorutils.HttpResponseError{StatusCode: http.StatusForbidden}))
	assert.False(t, isForbiddenError(&errorutils.HttpResponseError{StatusCode: http.StatusOK}))
	assert.False(t, isForbiddenError(errString("plain error")))
}

// ── manifest resolution edge cases ───────────────────────────────────────────

func TestResolvePSD1ManifestErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := resolvePSD1Manifest(filepath.Join(dir, "does-not-exist"))
	require.Error(t, err)

	wrongExt := filepath.Join(dir, "readme.txt")
	require.NoError(t, os.WriteFile(wrongExt, []byte("hi"), 0600))
	_, err = resolvePSD1Manifest(wrongExt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a module manifest")

	emptyDir := t.TempDir()
	_, err = resolvePSD1Manifest(emptyDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no module manifest")

	manifest := filepath.Join(dir, "Foo.psd1")
	require.NoError(t, os.WriteFile(manifest, []byte("@{}"), 0600))
	found, err := resolvePSD1Manifest(manifest)
	require.NoError(t, err)
	assert.Equal(t, manifest, found)

	found, err = resolvePSD1Manifest(dir)
	require.NoError(t, err)
	assert.Equal(t, manifest, found)
}

func TestPsd1ModuleVersionPropagatesQueryError(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) { return "", errString("boom") })
	defer restore()
	_, err := psd1ModuleVersion("pwsh", "Foo.psd1")
	require.Error(t, err)
}

func TestPsd1ModuleVersionEmptyIsAnError(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) { return "  \n", nil })
	defer restore()
	_, err := psd1ModuleVersion("pwsh", "Foo.psd1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ModuleVersion")
}

func TestResolveInstalledVersionParseErrorIsSurfaced(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) { return "not json", nil })
	defer restore()
	_, err := resolveInstalledVersion("pwsh", "Foo")
	require.Error(t, err)
}

func TestJoinWorkingDirectory(t *testing.T) {
	assert.Equal(t, "foo", joinWorkingDirectory("", "foo"))
	assert.Equal(t, "", joinWorkingDirectory("/work", ""))
	assert.Equal(t, "/work/foo", joinWorkingDirectory("/work", "foo"))
	assert.Equal(t, "/work/foo", joinWorkingDirectory("/work/", "foo"))
}

func TestFilepathExt(t *testing.T) {
	assert.Equal(t, ".psd1", filepathExt("Foo.psd1"))
	assert.Equal(t, "", filepathExt("noext"))
}

func TestResolvePublishNameVersionFailsWithoutNameOrManifest(t *testing.T) {
	dir := t.TempDir()
	command := NewPSResourceFlexPackCommand().SetWorkingDirectory(dir)
	_, _, err := command.resolvePublishNameVersion("pwsh")
	require.Error(t, err)
}

// ── remaining collectPublishArtifacts error paths ────────────────────────────

func TestCollectPublishArtifactsFailsWithoutRepo(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Name", "Foo", "-Version", "1.0.0"})
	err := command.collectPublishArtifacts("b", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-Repository")
}

// ── full Run() round trip with credential injection ──────────────────────────

func TestRunInjectsCredentialAndCollectsDependencyBuildInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.Header().Set("X-Checksum-Sha1", "sha1value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stubValidatePlatform(t, nil)
	stubResolveShell(t, "pwsh", nil)
	defer stubStampBuildProperties(t, nil)()

	var capturedScript string
	var capturedEnv []string
	restoreNative := stubNativeRunner(t, func(shell, script string, env []string, workingDirectory string) error {
		capturedScript = script
		capturedEnv = env
		return nil
	})
	defer restoreNative()
	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":"1.2.3"}`, nil
	})
	defer restoreQuery()

	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetBuildName("my-build")
	buildConfig.SetBuildNumber("1")
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandInstall).
		SetArgs([]string{"-Name", "Foo"}).
		SetRepoResolve("myrepo").
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/", User: "john", Password: "sup3rs3cr3t"}).
		SetBuildConfiguration(buildConfig).
		SetWorkingDirectory(t.TempDir())

	require.NoError(t, command.Run())

	assert.Contains(t, capturedScript, "$env:JFROG_PSRESOURCE_TOKEN")
	assert.NotContains(t, capturedScript, "sup3rs3cr3t")
	assert.Contains(t, capturedEnv, "JFROG_PSRESOURCE_USER=john")
	assert.Contains(t, capturedEnv, "JFROG_PSRESOURCE_TOKEN=sup3rs3cr3t")
}
