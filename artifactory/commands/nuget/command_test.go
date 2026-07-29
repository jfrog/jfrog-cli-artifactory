package nuget

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/entities"
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
				buildCmd("")
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
