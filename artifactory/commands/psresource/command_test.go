package psresource

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/setup"
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

// TestRedactPSResourceArgsColonForm guards against a real bug: PowerShell's colon-attached flag
// form (-ApiKey:secret, one token) was never matched by the space-separated-only redaction logic,
// so the real secret leaked into debug logs verbatim whenever a user passed a credential this way.
func TestRedactPSResourceArgsColonForm(t *testing.T) {
	for _, flag := range []string{"-ApiKey", "-Password", "-Credential", "-Token"} {
		redacted := redactPSResourceArgs([]string{"-Name", "Foo", flag + ":sup3rs3cr3t"})
		assert.NotContains(t, redacted, flag+":sup3rs3cr3t", "colon-form %s must be redacted", flag)
		assert.Contains(t, redacted, flag+":***")
	}
	// A colon inside an ordinary value (not a sensitive flag) must be left alone.
	redacted := redactPSResourceArgs([]string{"-Repository", "myrepo:with:colons"})
	assert.Equal(t, []string{"-Repository", "myrepo:with:colons"}, redacted)
}

// ── PowerShell script construction ───────────────────────────────────────────

func TestPsresourceCommandLineArg(t *testing.T) {
	assert.Equal(t, "-Name", psresourceCommandLineArg("-Name"))
	assert.Equal(t, "'Foo'", psresourceCommandLineArg("Foo"))
	assert.Equal(t, "'it''s escaped'", psresourceCommandLineArg("it's escaped"))
	assert.Equal(t, "-Repository:myrepo", psresourceCommandLineArg("-Repository:myrepo"), "a colon-attached flag+value must still pass through unquoted")
}

// TestPsresourceCommandLineArgRejectsInjectionLikeDashedValues guards against a real bug: any
// argument starting with "-" was treated as a trusted, pre-safe flag name and spliced into the
// -Command script completely unquoted, with no validation that it was actually just a flag - a
// single crafted argument (e.g. threaded through from an external/templated source) could smuggle
// PowerShell statement separators past QuotePSLiteral entirely.
func TestPsresourceCommandLineArgRejectsInjectionLikeDashedValues(t *testing.T) {
	malicious := "-Name; Remove-Item -Recurse -Force C:\\Temp; -Repository"
	rendered := psresourceCommandLineArg(malicious)
	assert.NotEqual(t, malicious, rendered, "must not be spliced in raw just because it starts with '-'")
	assert.True(t, strings.HasPrefix(rendered, "'") && strings.HasSuffix(rendered, "'"), "must be quoted as a literal instead: got %q", rendered)

	for _, benign := range []string{"-Name", "-Version", "-Repository", "-Trusted", "-Repository:my-repo", "-ApiKey:token", "-Trusted:$true", "-Trusted:$false"} {
		assert.Equal(t, benign, psresourceCommandLineArg(benign), "a genuine flag token must still pass through unquoted")
	}

	// A colon-attached value must not be able to smuggle a subexpression, an extra parameter or a
	// quote past the flag fast-path. Each of these matched the original pattern and was spliced in
	// completely unquoted, even though the pattern's own contract claimed to exclude them.
	for _, hostile := range []string{
		"-Name:$(Get-Process)",
		"-Name:$(Remove-Item -Recurse X)",
		"-Name:foo -Force",
		"-Name:foo\tbar",
		`-Name:'quoted'`,
		`-Name:"quoted"`,
		"-Name:$env:USERNAME",
	} {
		rendered := psresourceCommandLineArg(hostile)
		assert.NotEqual(t, hostile, rendered, "%q must not be spliced in unquoted", hostile)
		assert.True(t, strings.HasPrefix(rendered, "'") && strings.HasSuffix(rendered, "'"),
			"%q must be quoted as a literal instead: got %q", hostile, rendered)
	}
}

func TestPsresourceQuoteLiteral(t *testing.T) {
	assert.Equal(t, "'C:\\Modules\\Foo'", setup.QuotePSLiteral(`C:\Modules\Foo`))
	assert.Equal(t, "'it''s'", setup.QuotePSLiteral("it's"))
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

// TestBuildScriptPreservesUserSuppliedCredentialValue guards against a real bug: buildScript and
// debugScript shared the same redaction call, so a user-supplied -ApiKey/-Password/-Credential/-Token
// literal was replaced with "***" in the script actually executed by pwsh - not just in the log
// line - silently turning every such publish/install into an authentication failure.
func TestBuildScriptPreservesUserSuppliedCredentialValue(t *testing.T) {
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Path", "./MyModule", "-ApiKey", "sup3rs3cr3t"})

	// This command supplied its own credential, so Run() would resolve injectCredential=false and
	// call buildScript(false) - the exact path that must preserve the real value.
	script := command.buildScript(false)
	assert.Contains(t, script, "sup3rs3cr3t", "the real script sent to pwsh must never redact a user-supplied credential")
	assert.NotContains(t, script, "***")

	// The debug-log rendering of the same command must still redact it.
	assert.NotContains(t, command.debugScript(), "sup3rs3cr3t")
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
	assert.Equal(t, "deployRepo", command.injectionRepo())

	command.SetSubCommand(SubCommandInstall)
	assert.Equal(t, "resolveRepo", command.injectionRepo())

	command.SetSubCommand(SubCommandSave)
	assert.Equal(t, "resolveRepo", command.injectionRepo())

	command.SetSubCommand(SubCommandUpdate)
	assert.Equal(t, "resolveRepo", command.injectionRepo())

	command.SetSubCommand("Find-PSResource")
	assert.Equal(t, "", command.injectionRepo())
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
	stubResolveShell(t)
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
	stubResolveShell(t)
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
	stubResolveShell(t)
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

	err := command.collectDependencies("my-build", "1", "pwsh")
	require.NoError(t, err)
}

func TestCollectDependenciesFailsWithoutName(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall).SetRepoResolve("myrepo")
	err := command.collectDependencies("b", "1", "pwsh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-Name")
}

// TestCollectDependenciesRecognizesPositionalName guards against a real bug: -Name is
// Install-/Save-/Update-PSResource's first positional parameter ("Install-PSResource Foo -Repository
// myrepo" is valid, common PSResourceGet usage), but collectDependencies only ever looked for an
// explicit "-Name" flag, so a positional invocation - which had already installed the package
// successfully - was reported as a build-info collection failure.
func TestCollectDependenciesRecognizesPositionalName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":"1.2.3"}`, nil
	})
	defer restoreQuery()

	buildConfig := &buildutils.BuildConfiguration{}
	buildConfig.SetBuildName("my-build")
	buildConfig.SetBuildNumber("1")
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandInstall).
		SetArgs([]string{"Foo", "-Repository", "myrepo"}). // positional Name, no "-Name" flag
		SetRepoResolve("myrepo").
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetBuildConfiguration(buildConfig).
		SetWorkingDirectory(t.TempDir())

	err := command.collectDependencies("my-build", "1", "pwsh")
	require.NoError(t, err)
}

func TestCollectDependenciesFailsWithoutRepoResolve(t *testing.T) {
	command := NewPSResourceFlexPackCommand().SetSubCommand(SubCommandInstall).SetArgs([]string{"-Name", "Foo"})
	err := command.collectDependencies("b", "1", "pwsh")
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

	stubResolveShell(t)
	// Property stamping drives a real Artifactory search-and-SetProps round trip that this
	// lightweight HEAD-only fake server cannot serve; it is exercised on its own in
	// TestStampBuildPropertiesNoopsWithoutServerDetails and covered end-to-end outside this suite.
	// This test's job is the checksum-fetch/build-info-assembly path collectPublishArtifacts owns.
	defer stubStampBuildProperties(t)()
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

	err := command.collectPublishArtifacts("my-build", "1", "pwsh")
	require.NoError(t, err)
	assert.Contains(t, requestedPath, "foo/1.2.3/Foo.1.2.3.nupkg")
}

func TestCollectPublishArtifactsNotFoundIsAClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	stubResolveShell(t)
	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Name", "Foo", "-Version", "1.2.3", "-Repository", "myrepo"}).
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(t.TempDir())

	err := command.collectPublishArtifacts("my-build", "1", "pwsh")
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

	stubResolveShell(t)
	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return "9.9.9\n", nil
	})
	defer restoreQuery()
	defer stubStampBuildProperties(t)()

	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{"-Path", dir, "-Repository", "myrepo"}).
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(dir)

	err := command.collectPublishArtifacts("my-build", "1", "pwsh")
	require.NoError(t, err)
}

// TestCollectPublishArtifactsRecognizesPositionalPath guards against a real bug: -Path is
// Publish-PSResource's first positional parameter ("Publish-PSResource ./MyModule -Repository
// myrepo" is valid, common PSResourceGet usage), but resolvePublishNameVersion only ever looked for
// an explicit "-Path" flag, so a positional invocation fell back to scanning the working directory
// for any .psd1 - silently picking up the wrong manifest whenever one happened to be there.
func TestCollectPublishArtifactsRecognizesPositionalPath(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "MyModule.psd1")
	require.NoError(t, os.WriteFile(manifestPath, []byte("@{ ModuleVersion = '9.9.9' }"), 0600))

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stubResolveShell(t)
	restoreQuery := stubQueryRunner(t, func(shell, script string) (string, error) {
		return "9.9.9\n", nil
	})
	defer restoreQuery()
	defer stubStampBuildProperties(t)()

	command := NewPSResourceFlexPackCommand().
		SetSubCommand(SubCommandPublish).
		SetArgs([]string{dir, "-Repository", "myrepo"}). // positional Path, no "-Path" flag
		SetServerDetails(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}).
		SetWorkingDirectory(t.TempDir()) // deliberately NOT dir, so a working-directory fallback would find nothing/the wrong manifest

	err := command.collectPublishArtifacts("my-build", "1", "pwsh")
	require.NoError(t, err)
	assert.Contains(t, requestedPath, "mymodule/9.9.9/MyModule.9.9.9.nupkg")
}

// ── installed-version resolution ─────────────────────────────────────────────

// TestResolveInstalledVersionScopesToSearchPath guards against a real bug: Save-PSResource writes
// the package into its -Path target without ever installing it, but the version lookup ran a bare
// "Get-InstalledPSResource -Name <name>" against the installed-module set. That either returned
// nothing (failing a save that had actually succeeded) or reported an unrelated installed version,
// which then went into build-info and into the checksum lookup path.
func TestResolveInstalledVersionScopesToSearchPath(t *testing.T) {
	var capturedScript string
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		capturedScript = script
		return `{"Name":"Foo","Version":"3.1.4"}`, nil
	})
	defer restore()

	version, err := resolveInstalledVersion("pwsh", "Foo", "/tmp/saved modules")
	require.NoError(t, err)
	assert.Equal(t, "3.1.4", version)
	assert.Contains(t, capturedScript, "-Path ", "the query must be scoped to the save location")
	assert.Contains(t, capturedScript, "'/tmp/saved modules'", "the search path must be passed as a quoted PowerShell literal")
}

// TestResolveInstalledVersionOmitsPathWhenUnscoped is the counterpart: for Install-/Update-PSResource
// the installed-module set is the correct place to look, so no -Path may be added.
func TestResolveInstalledVersionOmitsPathWhenUnscoped(t *testing.T) {
	var capturedScript string
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		capturedScript = script
		return `{"Name":"Foo","Version":"1.0.0"}`, nil
	})
	defer restore()

	_, err := resolveInstalledVersion("pwsh", "Foo", "")
	require.NoError(t, err)
	assert.NotContains(t, capturedScript, "-Path")
}

// TestResolveAgainstWorkingDirectory pins the shell-agnostic absolute-path check that decides
// whether a user-supplied -Path is anchored at the working directory or left alone.
func TestResolveAgainstWorkingDirectory(t *testing.T) {
	assert.Equal(t, "/work/out", resolveAgainstWorkingDirectory("/work", "out"), "a relative path is anchored at the working directory")
	assert.Equal(t, "/abs/out", resolveAgainstWorkingDirectory("/work", "/abs/out"), "a POSIX absolute path is left alone")
	assert.Equal(t, `C:\out`, resolveAgainstWorkingDirectory("/work", `C:\out`), "a Windows drive-letter path is left alone")
	assert.Equal(t, "out", resolveAgainstWorkingDirectory("", "out"), "no working directory means nothing to anchor to")
}

func TestResolveInstalledVersionSingleObject(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":"1.0.0"}`, nil
	})
	defer restore()
	version, err := resolveInstalledVersion("pwsh", "Foo", "")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", version)
}

func TestResolveInstalledVersionArrayPicksLatest(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `[{"Name":"Foo","Version":"1.0.0"},{"Name":"Foo","Version":"2.0.0"}]`, nil
	})
	defer restore()
	version, err := resolveInstalledVersion("pwsh", "Foo", "")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", version)
}

func TestResolveInstalledVersionEmptyOutputIsAnError(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return "", nil
	})
	defer restore()
	_, err := resolveInstalledVersion("pwsh", "Foo", "")
	require.Error(t, err)
}

// TestResolveInstalledVersionEmptyVersionFieldIsAnError closes a gap distinct from the empty-output
// case above: valid JSON for a single installed package whose Version field is itself empty must
// still be rejected, not silently recorded in build-info with a blank version.
func TestResolveInstalledVersionEmptyVersionFieldIsAnError(t *testing.T) {
	restore := stubQueryRunner(t, func(shell, script string) (string, error) {
		return `{"Name":"Foo","Version":""}`, nil
	})
	defer restore()
	_, err := resolveInstalledVersion("pwsh", "Foo", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no version")
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

func TestWrapChecksumErr(t *testing.T) {
	notFoundErr := wrapChecksumErr(errPSResourcePackageNotFound, "Foo 1.0.0", "myrepo", "foo/1.0.0/Foo.1.0.0.nupkg")
	assert.ErrorIs(t, notFoundErr, errPSResourcePackageNotFound)
	assert.Contains(t, notFoundErr.Error(), "myrepo/foo/1.0.0/Foo.1.0.0.nupkg")

	// A persistent/non-retryable transport or server error must be wrapped distinctly from the
	// not-found case (persistArtifactBuildInfo treats the two differently: not-found discards the
	// build-info, a genuine error still saves it locally unstamped) - this branch had no test.
	genuineErr := errors.New("connection reset by peer")
	wrapped := wrapChecksumErr(genuineErr, "Foo 1.0.0", "myrepo", "foo/1.0.0/Foo.1.0.0.nupkg")
	assert.ErrorIs(t, wrapped, genuineErr)
	assert.NotErrorIs(t, wrapped, errPSResourcePackageNotFound)
	assert.Contains(t, wrapped.Error(), "Foo 1.0.0")
}

// TestSearchOnceRetriesOnZeroCount guards against a real bug: a successful-but-empty search result
// (Artifactory's async search index lagging behind a just-completed, HEAD-confirmed publish) was
// never retried - only a genuine search error was - so a routine indexing delay turned into an
// immediate, unretried "not found" that discarded already-collected build-info.
func TestSearchOnceRetriesOnZeroCount(t *testing.T) {
	attempts := 0
	count, err := searchOnce(func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, nil // not found yet, no error - must still be retried
		}
		return 1, nil // the index has caught up
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 3, attempts, "must retry a zero-count/no-error result, not treat it as final on the first attempt")
}

// TestSearchOnceStillReportsNotFoundAfterExhaustingRetries confirms the fix does not change the
// outcome once every retry has genuinely found nothing: callers key off "err == nil && count == 0"
// to mean "still not found" (see stampBuildProperties), so that must keep holding even though the
// zero-count case is now retried before giving up.
func TestSearchOnceStillReportsNotFoundAfterExhaustingRetries(t *testing.T) {
	attempts := 0
	count, err := searchOnce(func() (int, error) {
		attempts++
		return 0, nil
	})
	require.NoError(t, err, "exhausting retries on nothing but zero-count results must still report as a plain (0, nil), not the executor's own timeout error")
	assert.Equal(t, 0, count)
	assert.Equal(t, psresourceHeadRetries+1, attempts)
}

// TestSearchOncePropagatesGenuineSearchError confirms a real, non-retryable search error is still
// reported distinctly from the plain not-found case (persistArtifactBuildInfo treats the two
// differently: not-found discards the build-info, a genuine error still saves it locally unstamped).
func TestSearchOncePropagatesGenuineSearchError(t *testing.T) {
	searchErr := errors.New("boom")
	count, err := searchOnce(func() (int, error) {
		return 0, searchErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, searchErr)
	assert.NotErrorIs(t, err, errPSResourcePackageNotFound)
	assert.Equal(t, 0, count)
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

// TestStampBuildPropertiesForbiddenHint closes a real test-coverage gap: the annotate-permission
// hint appended when SetProps returns 403 had no test at all, so a regression that dropped or
// garbled it would go undetected.
func TestStampBuildPropertiesForbiddenHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"repo":"myrepo","path":"foo/1.2.3","name":"Foo.1.2.3.nupkg"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/storage/"):
			w.WriteHeader(http.StatusForbidden)
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "annotate permission")
}

// TestStampBuildPropertiesRetriesSetPropsOnTransientFailure closes a real test-coverage gap: the
// reader.Reset()-then-retry sequence on a retryable SetProps failure had no test proving the
// content.ContentReader is actually re-consumable on the retried attempt rather than sending an
// exhausted reader.
func TestStampBuildPropertiesRetriesSetPropsOnTransientFailure(t *testing.T) {
	var putAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"repo":"myrepo","path":"foo/1.2.3","name":"Foo.1.2.3.nupkg"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/storage/"):
			putAttempts++
			if putAttempts < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
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
	require.NoError(t, err, "a retryable SetProps failure followed by success must not fail the command")
	assert.Equal(t, 2, putAttempts, "the retried SetProps call must actually re-send the reset reader")
}

// TestFetchArtifactChecksumRetriesThenSucceeds closes a real test-coverage gap: every existing test
// server answered on the first request, so the actual retry-then-succeed behavior of
// fetchArtifactChecksum (a transient HEAD failure followed by a successful one) was never verified.
func TestFetchArtifactChecksumRetriesThenSucceeds(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("X-Checksum-Sha256", "sha256value")
		w.Header().Set("X-Checksum-Sha1", "sha1value")
		w.Header().Set("X-Checksum-Md5", "md5value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpCtx, err := newPSResourceHTTPContext(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"})
	require.NoError(t, err)

	checksum, err := fetchArtifactChecksum(httpCtx, "myrepo", "foo/1.0.0/Foo.1.0.0.nupkg")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "must actually retry a transient HEAD failure")
	assert.Equal(t, "sha256value", checksum.Sha256, "the checksum from the eventually-successful attempt must be the one returned, not dropped")
}

// TestFetchArtifactChecksumExhaustsRetriesOnPersistentFailure confirms the counterpart: a
// persistently failing HEAD (never a 404) is retried up to the configured limit and then surfaced
// as an error, not silently treated as success or as "not found".
func TestFetchArtifactChecksumExhaustsRetriesOnPersistentFailure(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	httpCtx, err := newPSResourceHTTPContext(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"})
	require.NoError(t, err)

	_, err = fetchArtifactChecksum(httpCtx, "myrepo", "foo/1.0.0/Foo.1.0.0.nupkg")
	require.Error(t, err)
	assert.NotErrorIs(t, err, errPSResourcePackageNotFound, "a persistent 503 is a real error, not a not-found")
	assert.Equal(t, psresourceHeadRetries+1, attempts)
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

func stubResolveShell(t *testing.T) {
	t.Helper()
	original := resolvePSResourceShell
	resolvePSResourceShell = func() (string, error) { return "pwsh", nil }
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

func stubStampBuildProperties(t *testing.T) func() {
	t.Helper()
	original := stampBuildPropertiesFn
	stampBuildPropertiesFn = func(*PSResourceFlexPackCommand, []entities.Artifact, string, string) error { return nil }
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

// TestIsRetryableErrorNetworkAndSyscallBranches closes a real test-coverage gap: the net.Error
// timeout branch and every syscall-errno branch (ECONNRESET/ECONNABORTED/ECONNREFUSED/EPIPE) had no
// test at all, so a regression silently dropping one of them from the retryable set would only
// surface as an intermittent, hard-to-diagnose production/CI failure.
func TestIsRetryableErrorNetworkAndSyscallBranches(t *testing.T) {
	assert.True(t, isRetryableError(timeoutError{}), "a timed-out net.Error must be retryable")
	assert.False(t, isRetryableError(nonTimeoutNetError{}), "a non-timeout net.Error must not be retryable")
	for _, syscallErr := range []error{syscall.ECONNRESET, syscall.ECONNABORTED, syscall.ECONNREFUSED, syscall.EPIPE} {
		assert.True(t, isRetryableError(syscallErr), "%v must be retryable", syscallErr)
		assert.True(t, isRetryableError(fmt.Errorf("wrapped: %w", syscallErr)), "a wrapped %v must still be retryable", syscallErr)
	}
	assert.True(t, isRetryableError(io.ErrUnexpectedEOF))
}

// timeoutError is a minimal net.Error whose Timeout() reports true.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// nonTimeoutNetError is a minimal net.Error whose Timeout() reports false.
type nonTimeoutNetError struct{}

func (nonTimeoutNetError) Error() string   { return "connection issue" }
func (nonTimeoutNetError) Timeout() bool   { return false }
func (nonTimeoutNetError) Temporary() bool { return false }

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
	_, err := resolveInstalledVersion("pwsh", "Foo", "")
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
	err := command.collectPublishArtifacts("b", "1", "pwsh")
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
	stubResolveShell(t)
	defer stubStampBuildProperties(t)()

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
