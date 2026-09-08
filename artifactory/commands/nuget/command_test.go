package nuget

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCmdPreservesNativeArguments(t *testing.T) {
	tests := []struct {
		name          string
		toolchainType dotnetutils.ToolchainType
		subCommand    string
		args          []string
		expected      []string
	}{
		{
			name:          "dotnet user config and source",
			toolchainType: dotnetutils.DotnetCore,
			subCommand:    "nuget push",
			args:          []string{"Package.1.0.0.nupkg", "--configfile", "user.config", "--source", "native-source", "--api-key", "native-key"},
			expected:      []string{"dotnet", "nuget", "push", "Package.1.0.0.nupkg", "--configfile", "user.config", "--source", "native-source", "--api-key", "native-key"},
		},
		{
			name:          "nuget user config and source",
			toolchainType: dotnetutils.Nuget,
			subCommand:    "push",
			args:          []string{"Package.1.0.0.nupkg", "-ConfigFile", "user.config", "-Source", "native-source", "-ApiKey", "native-key"},
			expected:      []string{"nuget", "push", "Package.1.0.0.nupkg", "-ConfigFile", "user.config", "-Source", "native-source", "-ApiKey", "native-key"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := NewNuGetFlexPackCommand().
				SetToolchainType(test.toolchainType).
				SetSubCommand(test.subCommand).
				SetArgs(test.args).
				buildCmd()
			if !reflect.DeepEqual(command.Args, test.expected) {
				t.Fatalf("command arguments: got %v, want %v", command.Args, test.expected)
			}
		})
	}
}

func TestRequiresServerDetails(t *testing.T) {
	tests := []struct {
		name        string
		subCommand  string
		repoResolve string
		repoDeploy  string
		expected    bool
	}{
		{name: "transparent passthrough", subCommand: "--info", expected: false},
		{name: "anonymous push", subCommand: "push", expected: false},
		{name: "push property stamping", subCommand: "push", repoDeploy: "nuget-local", expected: true},
		{name: "dotnet push property stamping", subCommand: "nuget push", repoDeploy: "nuget-local", expected: true},
		{name: "anonymous restore", subCommand: "restore", expected: false},
		{name: "repository restore", subCommand: "restore", repoResolve: "nuget-virtual", expected: true},
		{name: "local pack", subCommand: "pack", repoDeploy: "nuget-local", expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := NewNuGetFlexPackCommand().
				SetSubCommand(test.subCommand).
				SetRepoResolve(test.repoResolve).
				SetRepoDeploy(test.repoDeploy)
			if actual := command.RequiresServerDetails(); actual != test.expected {
				t.Fatalf("RequiresServerDetails() = %t, want %t", actual, test.expected)
			}
		})
	}
}

func TestArtifactPatternsUseExactPaths(t *testing.T) {
	artifacts := []entities.Artifact{
		{
			Name:                   "Package.1.0.0.nupkg",
			Path:                   "Package/1.0.0/Package.1.0.0.nupkg",
			OriginalDeploymentRepo: "nuget-local",
		},
		{
			Name:                   "Package.1.0.0.snupkg",
			Path:                   "/Package/1.0.0/Package.1.0.0.snupkg",
			OriginalDeploymentRepo: "symbols-local",
		},
		{Name: "missing-repository", Path: "ignored.nupkg"},
		{Name: "missing-path", OriginalDeploymentRepo: "nuget-local"},
	}

	expected := []string{
		"nuget-local/Package/1.0.0/Package.1.0.0.nupkg",
		"symbols-local/Package/1.0.0/Package.1.0.0.snupkg",
	}
	actual := artifactPatterns(artifacts)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("artifactPatterns() = %v, want %v", actual, expected)
	}
}

func TestHasNativeAuthOverride(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		// nuget.exe style (single dash)
		{name: "nuget -Source", args: []string{"-Source", "https://host/"}, expected: true},
		{name: "nuget -s", args: []string{"-s", "https://host/"}, expected: true},
		{name: "nuget -ApiKey", args: []string{"-ApiKey", "key"}, expected: true},
		{name: "nuget -SymbolApiKey", args: []string{"-SymbolApiKey", "key"}, expected: true},
		// dotnet CLI style (double dash)
		{name: "dotnet --source space-separated", args: []string{"--source", "https://host/"}, expected: true},
		{name: "dotnet --source inline-equals", args: []string{"--source=https://host/"}, expected: true},
		{name: "dotnet --api-key space-separated", args: []string{"--api-key", "token"}, expected: true},
		{name: "dotnet --api-key inline-equals", args: []string{"--api-key=mytoken"}, expected: true},
		{name: "dotnet -k short", args: []string{"-k", "token"}, expected: true},
		{name: "dotnet -k inline-equals", args: []string{"-k=mytoken"}, expected: true},
		{name: "dotnet --symbol-api-key", args: []string{"--symbol-api-key", "key"}, expected: true},
		{name: "dotnet --symbol-api-key inline-equals", args: []string{"--symbol-api-key=key"}, expected: true},
		// case insensitivity
		{name: "mixed case -APIKEY", args: []string{"-APIKEY", "key"}, expected: true},
		{name: "mixed case --Source", args: []string{"--Source", "https://host/"}, expected: true},
		// dotnet --symbol-source (Gap 2 fix)
		{name: "dotnet --symbol-source", args: []string{"--symbol-source", "https://symbols/"}, expected: true},
		{name: "dotnet --symbol-source inline-equals", args: []string{"--symbol-source=https://symbols/"}, expected: true},
		{name: "dotnet -ss", args: []string{"-ss", "https://symbols/"}, expected: true},
		// no override
		{name: "no flags", args: []string{"Package.1.0.0.nupkg"}, expected: false},
		{name: "unrelated flags", args: []string{"Package.1.0.0.nupkg", "--skip-duplicate", "--timeout", "60"}, expected: false},
		{name: "empty args", args: []string{}, expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := hasNativeAuthOverride(test.args); actual != test.expected {
				t.Fatalf("hasNativeAuthOverride(%v) = %t, want %t", test.args, actual, test.expected)
			}
		})
	}
}

func TestRestoreTarget(t *testing.T) {
	workingDir := t.TempDir()
	projectDir := filepath.Join(workingDir, "src")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{name: "project after option values", args: []string{"--source", "native-source", "src/App.csproj", "--verbosity", "minimal"}, expected: "src/App.csproj"},
		{name: "solution", args: []string{"solution/App.sln"}, expected: "solution/App.sln"},
		{name: "directory", args: []string{"src"}, expected: "src"},
		{name: "option value is not target", args: []string{"--packages", "src"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := restoreTarget(workingDir, test.args); actual != test.expected {
				t.Fatalf("restoreTarget() = %q, want %q", actual, test.expected)
			}
		})
	}
}

func TestSearchWithRetry(t *testing.T) {
	t.Run("succeeds on third attempt", func(t *testing.T) {
		attempt := 0
		n, err := searchWithRetry(5, 0, []string{"repo/pkg"}, func() (int, error) {
			attempt++
			if attempt < 3 {
				return 0, nil
			}
			return 2, nil
		})
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, 3, attempt)
	})

	t.Run("exhausts all attempts", func(t *testing.T) {
		calls := 0
		_, err := searchWithRetry(3, 0, []string{"repo/pkg"}, func() (int, error) {
			calls++
			return 0, nil
		})
		require.Error(t, err)
		assert.Equal(t, 3, calls)
		assert.True(t, strings.Contains(err.Error(), "3 attempts"), "expected attempt count in error: %v", err)
	})

	t.Run("propagates search error", func(t *testing.T) {
		_, err := searchWithRetry(3, 0, []string{"repo/pkg"}, func() (int, error) {
			return 0, errors.New("search failed")
		})
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "search failed"))
	})
}

func TestIsPushCommand(t *testing.T) {
	tests := []struct {
		sub      string
		wantPush bool
	}{
		{"push", true},
		{"nuget push", true}, // dotnet CLI two-word form
		{"restore", false},
		{"pack", false},
		{"build", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.sub, func(t *testing.T) {
			assert.Equal(t, tc.wantPush, isPushCommand(tc.sub))
		})
	}
}

// TestShouldPushViaNativeClient pins which pushes the dotnet CLI performs itself. FlexPack's
// contract is that the native tool does the work and jf only observes it, so a dotnet push
// with a JFrog source to build must not be taken over by the Artifactory upload bypass.
func TestShouldPushViaNativeClient(t *testing.T) {
	server := &config.ServerDetails{Url: "https://acme.jfrog.io/"}

	newCmd := func(toolchain dotnetutils.ToolchainType, sub string, srv *config.ServerDetails, repo string, args []string) *NuGetFlexPackCommand {
		return &NuGetFlexPackCommand{
			toolchainType: toolchain,
			subCommand:    sub,
			serverDetails: srv,
			repoDeploy:    repo,
			args:          args,
		}
	}

	// Both toolchains upload through their own client. Each authenticates against Artifactory
	// from the temp nuget.config's <packageSourceCredentials> and finds its target via
	// defaultPushSource, so neither needs jf to upload on its behalf.
	t.Run("dotnet nuget push is handled natively", func(t *testing.T) {
		cmd := newCmd(dotnetutils.DotnetCore, "nuget push", server, "nuget-local", []string{"pkg.nupkg"})
		assert.True(t, cmd.shouldPushViaNativeClient())
	})

	t.Run("nuget.exe push is handled natively", func(t *testing.T) {
		cmd := newCmd(dotnetutils.Nuget, "push", server, "nuget-local", []string{"pkg.nupkg"})
		assert.True(t, cmd.shouldPushViaNativeClient())
	})

	// A user-supplied source/api-key is an explicit choice and must win untouched.
	t.Run("user auth override is respected", func(t *testing.T) {
		for _, override := range [][]string{
			{"pkg.nupkg", "--source", "mine"},
			{"pkg.nupkg", "--api-key", "abc"},
			{"pkg.nupkg", "-s", "mine"},
			{"pkg.nupkg", "--source=mine"},
		} {
			cmd := newCmd(dotnetutils.DotnetCore, "nuget push", server, "nuget-local", override)
			assert.False(t, cmd.shouldPushViaNativeClient(), "args: %v", override)
		}
	})

	t.Run("non-push subcommands are unaffected", func(t *testing.T) {
		for _, sub := range []string{"restore", "pack", "build", "publish"} {
			cmd := newCmd(dotnetutils.DotnetCore, sub, server, "nuget-local", []string{"App.csproj"})
			assert.False(t, cmd.shouldPushViaNativeClient(), "subcommand: %s", sub)
		}
	})

	// Without a server or deploy repo there is no source to inject, so the native tool runs
	// on its own configuration and jf collects build-info only.
	t.Run("missing server or repo falls through", func(t *testing.T) {
		assert.False(t, newCmd(dotnetutils.DotnetCore, "nuget push", nil, "nuget-local", []string{"pkg.nupkg"}).shouldPushViaNativeClient())
		assert.False(t, newCmd(dotnetutils.DotnetCore, "nuget push", server, "", []string{"pkg.nupkg"}).shouldPushViaNativeClient())
	})
}

// TestCredentialEnvEntry pins the NuGet environment-variable credential format. Both
// nuget.exe and the dotnet CLI read NuGetPackageSourceCredentials_<source> in the
// "Username=<u>;Password=<p>" form, keyed by the source name declared in the config file.
func TestCredentialEnvEntry(t *testing.T) {
	t.Run("format matches what NuGet expects", func(t *testing.T) {
		assert.Equal(t,
			"NuGetPackageSourceCredentials_JFrog=Username=admin;Password=token123",
			credentialEnvEntry("JFrog", "admin", "token123"))
	})

	// The key must carry the source name, since NuGet matches the variable to the source
	// declared in the config; a mismatch silently yields an unauthenticated request (401).
	t.Run("key is keyed by source name", func(t *testing.T) {
		got := credentialEnvEntry("MyFeed", "u", "p")
		assert.True(t, strings.HasPrefix(got, "NuGetPackageSourceCredentials_MyFeed="), got)
	})

	t.Run("access token as password is carried verbatim", func(t *testing.T) {
		token := "eyJ2ZXIiOiIyIiwidHlwIjoiSldUIn0.abc-DEF_123" //#nosec G101 -- not a credential, a shaped literal asserting the token survives unaltered
		got := credentialEnvEntry("JFrog", "bhanu", token)
		assert.Contains(t, got, "Password="+token)
	})
}

// TestTempConfigCarriesNoSecret is the regression guard for the change that moved credentials
// out of the temp nuget.config: the file must declare the source but never a password, so a
// crash that skips cleanup cannot leave a secret on disk.
func TestTempConfigCarriesNoSecret(t *testing.T) {
	cmd := NewNuGetFlexPackCommand().
		SetToolchainType(dotnetutils.DotnetCore).
		SetSubCommand("restore").
		SetArgs([]string{"App.csproj"}).
		SetServerDetails(&config.ServerDetails{
			ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
			User:           "admin",
			Password:       "sup3rs3cr3t",
		})

	cleanup, err := cmd.injectCredentialsViaTempConfig("nuget-virtual")
	require.NoError(t, err)
	defer cleanup()

	// Recover the temp config path from the flag jf appended.
	var cfgPath string
	for i, a := range cmd.args {
		if a == "--configfile" && i+1 < len(cmd.args) {
			cfgPath = cmd.args[i+1]
		}
	}
	require.NotEmpty(t, cfgPath, "expected --configfile to be appended")

	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	contents := string(body)

	assert.NotContains(t, contents, "sup3rs3cr3t", "password must not be written to disk")
	assert.NotContains(t, contents, "ClearTextPassword")
	assert.NotContains(t, contents, "packageSourceCredentials")
	assert.Contains(t, contents, "nuget-virtual", "source should still be declared")
	assert.Contains(t, contents, "defaultPushSource")

	// The secret travels in the environment instead.
	assert.Contains(t, cmd.credentialEnv, "Password=sup3rs3cr3t")

	// Cleanup must remove the file and clear the credential.
	cleanup()
	_, statErr := os.Stat(cfgPath)
	assert.True(t, os.IsNotExist(statErr), "temp config should be removed")
	assert.Empty(t, cmd.credentialEnv)
}

// TestInsertBeforeSeparator pins where jf's injected --configfile lands relative to a user's
// "--" separator. The dotnet CLI forwards everything after "--" to MSBuild, so an injected flag
// on the wrong side of it reaches MSBuild's parser and fails the restore with MSB1001.
// TestPackTargetDirs pins the directories a pack command can write packages to. Without --output
// each project writes to its own bin/<Configuration>, so packing a target below the working
// directory produced nothing under <workingDir>/bin and build-info was persisted with no modules
// while the command still reported success.
func TestPackTargetDirs(t *testing.T) {
	workingDir := t.TempDir()
	nested := filepath.Join(workingDir, "src", "Lib")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	for _, tc := range []struct {
		name     string
		args     []string
		expected []string
	}{
		{"no target", []string{"--configuration", "Release"}, nil},
		{"relative project", []string{filepath.Join("src", "Lib", "Lib.csproj")}, []string{nested}},
		{"solution", []string{filepath.Join("src", "Lib", "App.sln")}, []string{nested}},
		{"fsproj and vbproj", []string{"a.fsproj", "b.vbproj"}, []string{workingDir}},
		{"nuspec", []string{"Pkg.nuspec"}, []string{workingDir}},
		{"existing directory argument", []string{filepath.Join("src", "Lib")}, []string{nested}},
		// A flag value that happens to look like a path must not be treated as a target.
		{"flag value is not a target", []string{"--configuration", "Release", "x.csproj"}, []string{workingDir}},
		// A non-existent, non-project positional is not a directory to snapshot.
		{"unknown positional ignored", []string{"Release"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, packTargetDirs(workingDir, tc.args))
		})
	}
}

// TestPerformsRestore pins which sub-commands need the Artifactory source declared. pack and
// publish restore implicitly unless --no-restore is given, and both accept --configfile; omitting
// them meant --repo-resolve was accepted and then silently dropped.
func TestPerformsRestore(t *testing.T) {
	for _, sub := range []string{"restore", "install", "update", "build", "add", "pack", "publish"} {
		assert.True(t, performsRestore(sub), "%s restores and needs the source declared", sub)
	}
	for _, sub := range []string{"push", "nuget push", "locals", "list"} {
		assert.False(t, performsRestore(sub), "%s does not restore", sub)
	}
}

// TestUserConfigFileDetection pins that a config file the user supplied is recognised in every
// spelling both clients accept, so jf steps aside instead of appending a second one. The dotnet
// CLI rejects a duplicate --configfile outright, and nuget.exe silently honours only the last,
// discarding the user's own sources and packageSourceCredentials.
func TestUserConfigFileDetection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		expected string
	}{
		{"none", []string{"restore", "App.sln", "--no-restore"}, ""},
		{"dotnet space form", []string{"restore", "--configfile", "corp.config"}, "corp.config"},
		{"dotnet inline form", []string{"restore", "--configfile=corp.config"}, "corp.config"},
		{"nuget space form", []string{"restore", "-ConfigFile", "corp.config"}, "corp.config"},
		{"nuget inline form", []string{"restore", "-ConfigFile=corp.config"}, "corp.config"},
		{"case insensitive", []string{"restore", "-CONFIGFILE", "corp.config"}, "corp.config"},
		// A trailing flag with no value is still the user asking for their own file; report the
		// flag rather than silently treating it as absent and injecting a second one.
		{"flag with no value", []string{"restore", "--configfile"}, "--configfile"},
		// Must not be confused with a different flag that merely starts the same way.
		{"similar flag is not a match", []string{"restore", "--configfile-ish", "x"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, userConfigFilePath(tc.args))
			assert.Equal(t, tc.expected != "", hasUserConfigFile(tc.args))
		})
	}
}

func TestInsertBeforeSeparator(t *testing.T) {
	t.Run("no separator appends at the end", func(t *testing.T) {
		got := insertBeforeSeparator([]string{"App.sln", "--verbosity", "quiet"}, "--configfile", "/tmp/x")
		assert.Equal(t, []string{"App.sln", "--verbosity", "quiet", "--configfile", "/tmp/x"}, got)
	})

	t.Run("separator receives the flag before it", func(t *testing.T) {
		got := insertBeforeSeparator([]string{"App.sln", "--", "--verbosity", "minimal"}, "--configfile", "/tmp/x")
		assert.Equal(t, []string{"App.sln", "--configfile", "/tmp/x", "--", "--verbosity", "minimal"}, got)
	})

	t.Run("only the first separator counts", func(t *testing.T) {
		got := insertBeforeSeparator([]string{"App.sln", "--", "a", "--", "b"}, "--configfile", "/tmp/x")
		assert.Equal(t, []string{"App.sln", "--configfile", "/tmp/x", "--", "a", "--", "b"}, got)
	})

	t.Run("leading separator still yields a valid command", func(t *testing.T) {
		got := insertBeforeSeparator([]string{"--", "--verbosity", "minimal"}, "--configfile", "/tmp/x")
		assert.Equal(t, []string{"--configfile", "/tmp/x", "--", "--verbosity", "minimal"}, got)
	})

	// A double-dashed flag is not a separator; only a bare "--" is.
	t.Run("double-dashed flags are not separators", func(t *testing.T) {
		got := insertBeforeSeparator([]string{"App.sln", "--no-restore"}, "--configfile", "/tmp/x")
		assert.Equal(t, []string{"App.sln", "--no-restore", "--configfile", "/tmp/x"}, got)
	})

	t.Run("empty args", func(t *testing.T) {
		assert.Equal(t, []string{"--configfile", "/tmp/x"},
			insertBeforeSeparator(nil, "--configfile", "/tmp/x"))
	})

	// The caller restores c.args from a saved copy, so the input slice must not be aliased in a
	// way that lets the insert leak back into it.
	t.Run("does not mutate the input slice", func(t *testing.T) {
		original := []string{"App.sln", "--", "--verbosity", "minimal"}
		snapshot := append([]string(nil), original...)
		_ = insertBeforeSeparator(original, "--configfile", "/tmp/x")
		assert.Equal(t, snapshot, original, "input slice must be left untouched")
	})
}
