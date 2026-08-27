package nuget

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/build-info-go/entities"
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

func TestPushSinglePackage(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		skipDuplicate bool
		wantErr       bool
		errContains   string
	}{
		{name: "201 created", statusCode: http.StatusCreated},
		{name: "200 ok", statusCode: http.StatusOK},
		{name: "204 no content", statusCode: http.StatusNoContent},
		{name: "409 skip duplicate false", statusCode: http.StatusConflict, skipDuplicate: false, wantErr: true, errContains: "409"},
		{name: "409 skip duplicate true", statusCode: http.StatusConflict, skipDuplicate: true},
		{name: "500 server error", statusCode: http.StatusInternalServerError, responseBody: "internal error", wantErr: true, errContains: "500"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				if tc.responseBody != "" {
					_, _ = fmt.Fprint(w, tc.responseBody)
				}
			}))
			defer srv.Close()

			// create a minimal temp .nupkg file
			tmpPkg := filepath.Join(t.TempDir(), "test.1.0.0.nupkg")
			require.NoError(t, os.WriteFile(tmpPkg, []byte("fake nupkg content"), 0o600))

			err := pushSinglePackage(srv.Client(), srv.URL+"/", tmpPkg, "user", "pass", tc.skipDuplicate)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errContains), "expected %q in error: %v", tc.errContains, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("file not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()
		err := pushSinglePackage(srv.Client(), srv.URL+"/", "/nonexistent/path/pkg.nupkg", "user", "pass", false)
		require.Error(t, err)
	})
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

func TestBuildPushURLs(t *testing.T) {
	tests := []struct {
		name        string
		rtURL       string
		repo        string
		wantNupkg   string
		wantSnupkg  string
	}{
		{
			name:       "simple repo",
			rtURL:      "https://example.jfrog.io/artifactory",
			repo:       "nuget-local",
			wantNupkg:  "https://example.jfrog.io/artifactory/api/nuget/v2/nuget-local/",
			wantSnupkg: "https://example.jfrog.io/artifactory/api/nuget/v2/nuget-local/symbolpackage",
		},
		{
			name:       "repo with slash",
			rtURL:      "https://example.jfrog.io/artifactory",
			repo:       "org/nuget-local",
			wantNupkg:  "https://example.jfrog.io/artifactory/api/nuget/v2/org%2Fnuget-local/",
			wantSnupkg: "https://example.jfrog.io/artifactory/api/nuget/v2/org%2Fnuget-local/symbolpackage",
		},
		{
			name:       "repo with spaces",
			rtURL:      "https://example.jfrog.io/artifactory",
			repo:       "my repo",
			wantNupkg:  "https://example.jfrog.io/artifactory/api/nuget/v2/my%20repo/",
			wantSnupkg: "https://example.jfrog.io/artifactory/api/nuget/v2/my%20repo/symbolpackage",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotNupkg, gotSnupkg := buildPushURLs(tc.rtURL, tc.repo)
			assert.Equal(t, tc.wantNupkg, gotNupkg)
			assert.Equal(t, tc.wantSnupkg, gotSnupkg)
		})
	}
}
