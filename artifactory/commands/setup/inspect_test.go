package setup

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bidotnet "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/golang"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/gradle"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/python"
	commandsutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/commands/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/maven"
	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const statusTestArtifactoryURL = "https://acme.jfrog.io/artifactory/"

func statusTestServer() *config.ServerDetails {
	return &config.ServerDetails{Url: "https://acme.jfrog.io/", ArtifactoryUrl: statusTestArtifactoryURL}
}

// isolateStatusEnv points every location status reads at a fresh temp directory, clears
// every override variable, and runs the test from a directory with no project config.
func isolateStatusEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, env := range []string{"HOME", "USERPROFILE"} {
		t.Setenv(env, home)
	}
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, env := range []string{
		"NPM_CONFIG_USERCONFIG", "npm_config_userconfig", "NPM_CONFIG_REGISTRY", "npm_config_registry",
		"PIP_CONFIG_FILE", "PIP_INDEX_URL", "VIRTUAL_ENV",
		"UV_CONFIG_FILE", "UV_DEFAULT_INDEX", "UV_INDEX_URL", "UV_INDEX", "UV_EXTRA_INDEX_URL", "UV_NO_CONFIG",
		"GOENV", "GOPROXY", gradle.UserHomeEnv,
		"DOCKER_CONFIG", "REGISTRY_AUTH_FILE", "XDG_RUNTIME_DIR", "HELM_REGISTRY_CONFIG", "HELM_CONFIG_HOME",
	} {
		t.Setenv(env, "")
	}
	t.Chdir(t.TempDir())
	stubTools(t, nil, nil)
	return home
}

// stubTools replaces binary lookup and execution: only the binaries listed are found,
// and each command line returns its canned output.
func stubTools(t *testing.T, binaries []string, outputs map[string]string) {
	t.Helper()
	originalLookPath, originalRunTool := lookPath, runTool
	t.Cleanup(func() { lookPath, runTool = originalLookPath, originalRunTool })
	lookPath = func(name string) (string, error) {
		for _, binary := range binaries {
			if binary == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
	runTool = func(name string, args ...string) ([]byte, error) {
		if output, ok := outputs[strings.Join(append([]string{name}, args...), " ")]; ok {
			return []byte(output), nil
		}
		return nil, exec.ErrNotFound
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	// #nosec G703 -- test helper; path is always under t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	// #nosec G703 -- test helper; path is always under t.TempDir()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func requireStatus(t *testing.T, packageManager project.ProjectType) (PackageManagerStatus, storedCredentials) {
	t.Helper()
	result, err := inspect(packageManager, statusTestServer())
	require.NoError(t, err)
	return result.status, result.credentials
}

func TestInspectors_CoverEverySupportedPackageManager(t *testing.T) {
	assert.Len(t, inspectors, len(packageManagerToRepositoryPackageType)-len(unsupportedInspection))
	for packageManager := range packageManagerToRepositoryPackageType {
		_, inspected := inspectors[packageManager]
		assert.True(t, inspected != containsProjectType(unsupportedInspection, packageManager),
			"%s must be in exactly one of inspectors and unsupportedInspection", packageManager)
	}
}

func containsProjectType(types []project.ProjectType, target project.ProjectType) bool {
	for _, candidate := range types {
		if candidate == target {
			return true
		}
	}
	return false
}

func TestMatchServerURL(t *testing.T) {
	server := statusTestServer()
	tests := []struct {
		name    string
		url     string
		matched bool
		rest    string
	}{
		{"same URL", "https://acme.jfrog.io/artifactory", true, ""},
		{"repository path", "https://acme.jfrog.io/artifactory/api/npm/npm-virtual/", true, "api/npm/npm-virtual"},
		{"scheme ignored", "http://acme.jfrog.io/artifactory/api/npm/r", true, "api/npm/r"},
		{"default port ignored", "https://acme.jfrog.io:443/artifactory/r", true, "r"},
		{"host case ignored", "https://ACME.jfrog.io/Artifactory/r", true, "r"},
		{"credentials ignored", "https://u:p@acme.jfrog.io/artifactory/r", true, "r"},
		{"other host", "https://other.jfrog.io/artifactory/r", false, ""},
		{"other instance on the same host", "https://acme.jfrog.io/artifactory2/r", false, ""},
		{"non-default port", "https://acme.jfrog.io:8443/artifactory/r", false, ""},
		{"not a URL", "direct", false, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched, rest := matchServerURL(test.url, server)
			assert.Equal(t, test.matched, matched)
			assert.Equal(t, test.rest, rest)
		})
	}
}

func TestMatchServerURL_TwoInstancesOnOneHost(t *testing.T) {
	first := &config.ServerDetails{ArtifactoryUrl: "https://shared.example.com/first/artifactory/"}
	second := &config.ServerDetails{ArtifactoryUrl: "https://shared.example.com/second/artifactory/"}
	configured := "https://shared.example.com/second/artifactory/api/npm/npm/"
	matched, _ := matchServerURL(configured, first)
	assert.False(t, matched)
	matched, rest := matchServerURL(configured, second)
	assert.True(t, matched)
	assert.Equal(t, "api/npm/npm", rest)
}

func TestRedactURL(t *testing.T) {
	assert.Equal(t, "https://***:***@acme.jfrog.io/artifactory", redactURL("https://user:secret@acme.jfrog.io/artifactory"))
	assert.Equal(t, "https://acme.jfrog.io/artifactory", redactURL("https://acme.jfrog.io/artifactory"))
	assert.Equal(t, "https://***:***@acme.jfrog.io/%zz", redactURL("https://user:secret@acme.jfrog.io/%zz"), "an unparsable URL is still redacted")
}

func TestIsPublicDefault_IgnoresDefaultPort(t *testing.T) {
	assert.True(t, isPublicDefault(project.Npm, "https://registry.npmjs.org:443/"))
	assert.False(t, isPublicDefault(project.Npm, "https://registry.npmjs.org:8443/"))
}

func TestCredentialsFromURL_RequiresAPassword(t *testing.T) {
	_, ok := credentialsFromURL("https://user@acme.jfrog.io/artifactory")
	assert.False(t, ok)
	creds, ok := credentialsFromURL("https://user:secret@acme.jfrog.io/artifactory")
	assert.True(t, ok)
	assert.Equal(t, storedCredentials{user: "user", password: "secret"}, creds)
}

func TestInspect_RequiresArtifactoryURL(t *testing.T) {
	isolateStatusEnv(t)
	_, err := inspect(project.Npm, &config.ServerDetails{})
	assert.ErrorContains(t, err, "no Artifactory URL")
}

func TestInspect_Unsupported(t *testing.T) {
	isolateStatusEnv(t)
	status, _ := requireStatus(t, project.Yarn)
	assert.Equal(t, StateUnsupported, status.State)
	assert.Nil(t, status.BinaryFound)
	assert.Equal(t, StatusSchemaVersion, status.SchemaVersion)
}

func TestInspectNpm(t *testing.T) {
	token := testCredential()
	tests := []struct {
		name        string
		npmrc       string
		state       ConfigState
		repoKey     string
		host        string
		credentials CredentialsState
	}{
		{name: "missing file", state: StateNotConfigured, credentials: CredentialsNotApplicable},
		{name: "public default", npmrc: "registry=https://registry.npmjs.org/\n", state: StateNotConfigured, host: "registry.npmjs.org", credentials: CredentialsNotApplicable},
		{
			name:  "configured with a token",
			npmrc: "registry=https://acme.jfrog.io/artifactory/api/npm/npm-virtual/\n//acme.jfrog.io/artifactory/api/npm/npm-virtual/:_authToken=" + token + "\n",
			state: StateConfigured, repoKey: "npm-virtual", host: "acme.jfrog.io", credentials: CredentialsPresent,
		},
		{
			name:  "configured anonymously",
			npmrc: "registry=https://acme.jfrog.io/artifactory/api/npm/npm-virtual/\n//other.io/:_authToken=x\n",
			state: StateConfigured, repoKey: "npm-virtual", host: "acme.jfrog.io", credentials: CredentialsAbsent,
		},
		{name: "other host", npmrc: "registry=https://npm.example.com/\n", state: StateOtherHost, host: "npm.example.com", credentials: CredentialsNotApplicable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := isolateStatusEnv(t)
			if test.npmrc != "" {
				writeTestFile(t, filepath.Join(home, ".npmrc"), test.npmrc)
			}
			status, _ := requireStatus(t, project.Npm)
			assert.Equal(t, test.state, status.State)
			assert.Equal(t, test.repoKey, status.RepoKey)
			assert.Equal(t, test.host, status.Host)
			assert.Equal(t, test.credentials, status.Credentials)
			assert.Equal(t, filepath.Join(home, ".npmrc"), status.Location)
			require.NotNil(t, status.BinaryFound)
			assert.False(t, *status.BinaryFound)
		})
	}
}

func TestInspectNpm_CredentialsForms(t *testing.T) {
	home := isolateStatusEnv(t)
	basic := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	t.Setenv("NPM_TEST_TOKEN", "from-env")
	writeTestFile(t, filepath.Join(home, ".npmrc"), strings.Join([]string{
		"registry=https://acme.jfrog.io/artifactory/api/npm/npm-virtual/",
		"//acme.jfrog.io/artifactory/:_auth=" + basic,
		"//acme.jfrog.io/artifactory/api/npm/npm-virtual/:_authToken=${NPM_TEST_TOKEN}",
	}, "\n"))
	_, creds := requireStatus(t, project.Npm)
	assert.Equal(t, "from-env", creds.token, "the longest matching prefix wins and ${VAR} is expanded")
}

func TestInspectNpm_CredentialsParsing(t *testing.T) {
	registry := "registry=https://acme.jfrog.io/artifactory/api/npm/npm-virtual/"
	scope := "//acme.jfrog.io/artifactory/api/npm/npm-virtual/:"
	basic := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	tests := []struct {
		name     string
		lines    []string
		expected storedCredentials
		present  bool
	}{
		{"username and base64 password", []string{scope + "username=user", scope + "_password=" + base64.StdEncoding.EncodeToString([]byte("pass"))},
			storedCredentials{user: "user", password: "pass"}, true},
		{"_auth on a parent prefix", []string{"//acme.jfrog.io/artifactory/:_auth=" + basic}, storedCredentials{basicAuth: basic}, true},
		{"a longer prefix without auth does not hide a shorter one", []string{"//acme.jfrog.io/:_authToken=tok", scope + "always-auth=true"},
			storedCredentials{token: "tok"}, true},
		{"inline comment", []string{scope + "_authToken=tok ; comment"}, storedCredentials{token: "tok"}, true},
		{"quoted value keeps its semicolon", []string{scope + `_authToken="to;k"`}, storedCredentials{token: "to;k"}, true},
		{"${VAR?} of an unset variable is empty", []string{scope + "_authToken=${NPM_TEST_UNSET?}"}, storedCredentials{}, false},
		// #nosec G101 -- False positive - an unexpanded variable reference, not a credential.
		{"${VAR} of an unset variable stays literal", []string{scope + "_authToken=${NPM_TEST_UNSET}"}, storedCredentials{token: "${NPM_TEST_UNSET}"}, true},
		{"top-level auth is not used", []string{"_authToken=tok"}, storedCredentials{}, false},
		{"environment variable in the key", []string{"//${NPM_TEST_HOST}/artifactory/api/npm/npm-virtual/:_authToken=tok"}, storedCredentials{token: "tok"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := isolateStatusEnv(t)
			t.Setenv("NPM_TEST_HOST", "acme.jfrog.io")
			writeTestFile(t, filepath.Join(home, ".npmrc"), strings.Join(append([]string{registry}, test.lines...), "\n")+"\n")
			status, creds := requireStatus(t, project.Npm)
			assert.Equal(t, test.expected, creds)
			if test.present {
				assert.Equal(t, CredentialsPresent, status.Credentials)
			} else {
				assert.Equal(t, CredentialsAbsent, status.Credentials)
			}
		})
	}
}

func TestInspectNpm_ProjectConfigIsTheLocalPrefixOnly(t *testing.T) {
	isolateStatusEnv(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".npmrc"), "registry=https://npm.example.com/\n")
	projectDir := filepath.Join(root, "app")
	writeTestFile(t, filepath.Join(projectDir, "package.json"), "{}")
	writeTestFile(t, filepath.Join(projectDir, ".npmrc"), "save-exact=true\n")
	t.Chdir(projectDir)
	status, _ := requireStatus(t, project.Npm)
	assert.Empty(t, status.OverriddenBy, "npm reads only the project's own .npmrc, and this one sets no registry")
}

func TestInspectNpm_UserConfigOverrideAndOverrides(t *testing.T) {
	isolateStatusEnv(t)
	custom := filepath.Join(t.TempDir(), "custom.npmrc")
	writeTestFile(t, custom, "registry=https://acme.jfrog.io/artifactory/api/npm/npm/\n")
	t.Setenv("NPM_CONFIG_USERCONFIG", custom)
	t.Setenv("NPM_CONFIG_REGISTRY", "https://registry.npmjs.org/")
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "package.json"), "{}")
	writeTestFile(t, filepath.Join(projectDir, ".npmrc"), "registry=https://npm.example.com/\n")
	subDir := filepath.Join(projectDir, "packages", "app")
	require.NoError(t, os.MkdirAll(subDir, 0700))
	t.Chdir(subDir)
	stubTools(t, []string{"npm"}, nil)

	status, _ := requireStatus(t, project.Npm)
	assert.Equal(t, StateConfigured, status.State)
	assert.Equal(t, custom, status.Location)
	assert.True(t, *status.BinaryFound)
	require.Len(t, status.OverriddenBy, 2)
	assert.Equal(t, "NPM_CONFIG_REGISTRY environment variable", status.OverriddenBy[0].Source)
	assert.Equal(t, filepath.Join(projectDir, ".npmrc"), status.OverriddenBy[1].Path)
}

func TestInspectPnpm(t *testing.T) {
	t.Run("binary missing", func(t *testing.T) {
		isolateStatusEnv(t)
		status, _ := requireStatus(t, project.Pnpm)
		assert.Equal(t, StateNotConfigured, status.State)
		assert.False(t, *status.BinaryFound)
	})
	t.Run("configured in auth.ini", func(t *testing.T) {
		home := isolateStatusEnv(t)
		configDir := filepath.Join(home, "pnpm-config")
		writeTestFile(t, filepath.Join(configDir, "auth.ini"),
			"registry=https://acme.jfrog.io/artifactory/api/npm/npm-remote/\n//acme.jfrog.io/artifactory/api/npm/npm-remote/:_authToken=tok\n")
		stubTools(t, []string{"pnpm"}, map[string]string{"pnpm config get globalconfig": filepath.Join(configDir, "rc") + "\n"})
		status, creds := requireStatus(t, project.Pnpm)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "npm-remote", status.RepoKey)
		assert.Equal(t, CredentialsPresent, status.Credentials)
		assert.Equal(t, filepath.Join(configDir, "auth.ini"), status.Location)
		assert.Equal(t, "tok", creds.token)
	})
}

func TestInspectPip(t *testing.T) {
	t.Run("configured through PIP_CONFIG_FILE", func(t *testing.T) {
		isolateStatusEnv(t)
		pipConfig := filepath.Join(t.TempDir(), "pip.conf")
		writeTestFile(t, pipConfig, "[global]\nindex-url = https://user:secret@acme.jfrog.io/artifactory/api/pypi/pypi-virtual/simple\n")
		t.Setenv("PIP_CONFIG_FILE", pipConfig)
		t.Setenv("PIP_INDEX_URL", "https://pypi.org/simple")
		status, creds := requireStatus(t, project.Pip)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "pypi-virtual", status.RepoKey)
		assert.Equal(t, pipConfig, status.Location)
		assert.Equal(t, CredentialsPresent, status.Credentials)
		assert.Equal(t, storedCredentials{user: "user", password: "secret"}, creds)
		require.Len(t, status.OverriddenBy, 1)
	})
	t.Run("public default", func(t *testing.T) {
		isolateStatusEnv(t)
		pipConfig := filepath.Join(t.TempDir(), "pip.conf")
		writeTestFile(t, pipConfig, "[global]\nindex_url = https://pypi.org/simple\n")
		t.Setenv("PIP_CONFIG_FILE", pipConfig)
		status, _ := requireStatus(t, project.Pipenv)
		assert.Equal(t, StateNotConfigured, status.State)
	})
	t.Run("other host", func(t *testing.T) {
		isolateStatusEnv(t)
		pipConfig := filepath.Join(t.TempDir(), "pip.conf")
		writeTestFile(t, pipConfig, "[global]\nindex-url = https://pypi.example.com/simple\n")
		t.Setenv("PIP_CONFIG_FILE", pipConfig)
		status, _ := requireStatus(t, project.Pip)
		assert.Equal(t, StateOtherHost, status.State)
		assert.Equal(t, "pypi.example.com", status.Host)
	})
	t.Run("missing file", func(t *testing.T) {
		isolateStatusEnv(t)
		t.Setenv("PIP_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.conf"))
		status, _ := requireStatus(t, project.Pip)
		assert.Equal(t, StateNotConfigured, status.State)
	})
}

func TestInspectPip_Overrides(t *testing.T) {
	const configured = "[global]\nindex-url = https://acme.jfrog.io/artifactory/api/pypi/pypi-virtual/simple\n"
	venvConfigName := "pip.conf"
	if runtime.GOOS == "windows" {
		venvConfigName = "pip.ini"
	}
	// setup writes userConfig where pip reads the per-user file, and a virtual environment
	// whose config file selects another index for `pip install`.
	setup := func(t *testing.T, userConfig string) (pipConfig, venv string) {
		isolateStatusEnv(t)
		pipConfig, err := python.ResolvePipConfigPath()
		require.NoError(t, err)
		writeTestFile(t, pipConfig, userConfig)
		venv = t.TempDir()
		writeTestFile(t, filepath.Join(venv, venvConfigName), "[install]\nindex-url = https://pypi.example.com/simple\n")
		t.Setenv("VIRTUAL_ENV", venv)
		return pipConfig, venv
	}
	t.Run("PIP_CONFIG_FILE beats the virtual environment config", func(t *testing.T) {
		pipConfig, _ := setup(t, configured)
		t.Setenv("PIP_CONFIG_FILE", pipConfig)
		status, _ := requireStatus(t, project.Pip)
		assert.Equal(t, StateConfigured, status.State)
		assert.Empty(t, status.OverriddenBy)
	})
	t.Run("an [install] index-url in the virtual environment config", func(t *testing.T) {
		pipConfig, venv := setup(t, configured)
		status, _ := requireStatus(t, project.Pip)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, pipConfig, status.Location)
		require.Len(t, status.OverriddenBy, 1)
		assert.Equal(t, filepath.Join(venv, venvConfigName), status.OverriddenBy[0].Path)
	})
	t.Run("an [install] index-url in the same file", func(t *testing.T) {
		setup(t, configured+"[install]\nindex-url = https://pypi.example.com/simple\n")
		t.Setenv("VIRTUAL_ENV", "")
		status, _ := requireStatus(t, project.Pip)
		require.Len(t, status.OverriddenBy, 1)
		assert.Equal(t, "[install] index-url", status.OverriddenBy[0].Source)
	})
}

func TestInspectUV(t *testing.T) {
	isolateStatusEnv(t)
	uvConfig := filepath.Join(t.TempDir(), "uv.toml")
	writeTestFile(t, uvConfig, "[[index]]\nname = \"jfrog-pypi\"\nurl = \"https://acme.jfrog.io/artifactory/api/pypi/pypi-remote/simple\"\ndefault = true\n")
	t.Setenv("UV_CONFIG_FILE", uvConfig)
	status, _ := requireStatus(t, project.UV)
	assert.Equal(t, StateConfigured, status.State)
	assert.Equal(t, "pypi-remote", status.RepoKey)
	assert.Equal(t, uvConfig, status.Location)
	assert.Equal(t, CredentialsUnknown, status.Credentials)
}

func TestInspectUV_StoredLogin(t *testing.T) {
	login := func(service, user, password string) string {
		return "[[credential]]\nservice = \"" + service + "\"\nusername = \"" + user + "\"\nscheme = \"basic\"\npassword = \"" + password + "\"\n\n"
	}
	tests := []struct {
		name        string
		indexURL    string
		store       string
		uvFound     bool
		credentials CredentialsState
		creds       storedCredentials
	}{
		{"host login, as jf setup writes", "", login("https://acme.jfrog.io", "admin", "secret"), true, CredentialsPresent, storedCredentials{user: "admin", password: "secret"}},
		{"longest service path wins", "", login("https://acme.jfrog.io", "admin", "host") + login("https://acme.jfrog.io/artifactory/api/pypi/pypi-remote", "admin", "repo"), true, CredentialsPresent, storedCredentials{user: "admin", password: "repo"}},
		{"another repository's path", "", login("https://acme.jfrog.io/artifactory/api/pypi/other", "admin", "secret"), true, CredentialsUnknown, storedCredentials{}},
		{"other host", "", login("https://other.jfrog.io", "admin", "secret"), true, CredentialsUnknown, storedCredentials{}},
		{"other scheme", "", login("http://acme.jfrog.io", "admin", "secret"), true, CredentialsUnknown, storedCredentials{}},
		{"non-basic scheme", "", "[[credential]]\nservice = \"https://acme.jfrog.io\"\nscheme = \"bearer\"\ntoken = \"t\"\n", true, CredentialsUnknown, storedCredentials{}},
		{"username in the index URL must match", "https://someone@acme.jfrog.io/artifactory/api/pypi/pypi-remote/simple", login("https://acme.jfrog.io", "admin", "secret"), true, CredentialsUnknown, storedCredentials{}},
		{"no store", "", "", true, CredentialsUnknown, storedCredentials{}},
		{"uv not found", "", login("https://acme.jfrog.io", "admin", "secret"), false, CredentialsUnknown, storedCredentials{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateStatusEnv(t)
			indexURL := test.indexURL
			if indexURL == "" {
				indexURL = "https://acme.jfrog.io/artifactory/api/pypi/pypi-remote/simple"
			}
			uvConfig := filepath.Join(t.TempDir(), "uv.toml")
			writeTestFile(t, uvConfig, "[[index]]\nname = \"jfrog-pypi\"\nurl = \""+indexURL+"\"\ndefault = true\n")
			t.Setenv("UV_CONFIG_FILE", uvConfig)
			credentialsDir := t.TempDir()
			if test.store != "" {
				writeTestFile(t, filepath.Join(credentialsDir, "credentials.toml"), test.store)
			}
			var binaries []string
			if test.uvFound {
				binaries = []string{"uv"}
			}
			stubTools(t, binaries, map[string]string{"uv auth dir": credentialsDir + "\n"})
			status, creds := requireStatus(t, project.UV)
			assert.Equal(t, StateConfigured, status.State)
			assert.Equal(t, test.credentials, status.Credentials)
			assert.Equal(t, test.creds, creds)
		})
	}
}

func TestUVOverrides(t *testing.T) {
	userConfig := filepath.Join(t.TempDir(), "uv.toml")
	withProject := func(t *testing.T, files map[string]string) string {
		isolateStatusEnv(t)
		projectDir := t.TempDir()
		for name, content := range files {
			writeTestFile(t, filepath.Join(projectDir, name), content)
		}
		subDir := filepath.Join(projectDir, "src")
		require.NoError(t, os.MkdirAll(subDir, 0700))
		t.Chdir(subDir)
		return projectDir
	}
	t.Run("project uv.toml with an index", func(t *testing.T) {
		projectDir := withProject(t, map[string]string{"uv.toml": "index-url = \"https://pypi.example.com/simple\"\n"})
		overrides := uvOverrides(userConfig)
		require.Len(t, overrides, 1)
		assert.Equal(t, filepath.Join(projectDir, "uv.toml"), overrides[0].Path)
	})
	t.Run("project uv.toml without an index", func(t *testing.T) {
		withProject(t, map[string]string{"uv.toml": "cache-dir = \"/tmp/uv\"\n"})
		assert.Empty(t, uvOverrides(userConfig))
	})
	t.Run("pyproject.toml [tool.uv] with an index", func(t *testing.T) {
		projectDir := withProject(t, map[string]string{"pyproject.toml": "[project]\nname = \"x\"\n\n[[tool.uv.index]]\nurl = \"https://pypi.example.com/simple\"\n"})
		overrides := uvOverrides(userConfig)
		require.Len(t, overrides, 1)
		assert.Equal(t, filepath.Join(projectDir, "pyproject.toml"), overrides[0].Path)
	})
	t.Run("UV_CONFIG_FILE skips project discovery", func(t *testing.T) {
		withProject(t, map[string]string{"uv.toml": "index-url = \"https://pypi.example.com/simple\"\n"})
		t.Setenv("UV_CONFIG_FILE", userConfig)
		assert.Empty(t, uvOverrides(userConfig))
	})
	t.Run("UV_NO_CONFIG ignores the user-level uv.toml", func(t *testing.T) {
		withProject(t, map[string]string{"uv.toml": "index-url = \"https://pypi.example.com/simple\"\n"})
		t.Setenv("UV_NO_CONFIG", "1")
		assert.Equal(t, []ConfigOverride{{Source: "UV_NO_CONFIG environment variable"}}, uvOverrides(userConfig))
	})
	t.Run("UV_NO_CONFIG with UV_CONFIG_FILE", func(t *testing.T) {
		isolateStatusEnv(t)
		t.Setenv("UV_NO_CONFIG", "1")
		t.Setenv("UV_CONFIG_FILE", userConfig)
		assert.Empty(t, uvOverrides(userConfig))
	})
	t.Run("UV_EXTRA_INDEX_URL", func(t *testing.T) {
		isolateStatusEnv(t)
		t.Setenv("UV_EXTRA_INDEX_URL", "https://pypi.example.com/simple")
		overrides := uvOverrides(userConfig)
		require.Len(t, overrides, 1)
		assert.Equal(t, "UV_EXTRA_INDEX_URL environment variable", overrides[0].Source)
	})
}

func TestInspectGo(t *testing.T) {
	tests := []struct {
		name    string
		goProxy string
		state   ConfigState
		repoKey string
		host    string
	}{
		{"empty", "", StateNotConfigured, "", ""},
		{"public default", "https://proxy.golang.org,direct", StateNotConfigured, "", "proxy.golang.org"},
		{"configured", "https://u:tok@acme.jfrog.io/artifactory/api/go/go-virtual,direct", StateConfigured, "go-virtual", "acme.jfrog.io"},
		{"server after another proxy", "https://goproxy.example.com|https://acme.jfrog.io/artifactory/api/go/go-remote", StateOtherHost, "", "goproxy.example.com"},
		{"server after direct", "direct,https://acme.jfrog.io/artifactory/api/go/go-remote", StateNotConfigured, "", ""},
		{"server after the public default", "https://proxy.golang.org,https://acme.jfrog.io/artifactory/api/go/go-remote", StateNotConfigured, "", "proxy.golang.org"},
		{"other host", "https://goproxy.example.com,direct", StateOtherHost, "", "goproxy.example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateStatusEnv(t)
			goEnv := filepath.Join(t.TempDir(), "env")
			writeTestFile(t, goEnv, "GOPRIVATE=example.com\nGOPROXY="+test.goProxy+"\n")
			stubTools(t, []string{"go"}, map[string]string{"go env GOENV": goEnv + "\n"})
			status, _ := requireStatus(t, project.Go)
			assert.Equal(t, test.state, status.State)
			assert.Equal(t, test.repoKey, status.RepoKey)
			assert.Equal(t, test.host, status.Host)
			assert.Equal(t, goEnv, status.Location)
		})
	}
}

func TestInspectGo_WithoutGoBinary(t *testing.T) {
	isolateStatusEnv(t)
	goEnv := filepath.Join(t.TempDir(), "env")
	writeTestFile(t, goEnv, "GOPROXY=https://acme.jfrog.io/artifactory/api/go/go-virtual,direct\n")
	t.Setenv("GOENV", goEnv)
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	status, _ := requireStatus(t, project.Go)
	assert.Equal(t, StateConfigured, status.State)
	assert.False(t, *status.BinaryFound)
	assert.Equal(t, CredentialsAbsent, status.Credentials)
	assert.Equal(t, []ConfigOverride{{Source: "GOPROXY environment variable"}}, status.OverriddenBy)
}

func TestInspectGo_GoEnvOff(t *testing.T) {
	for name, binaries := range map[string][]string{"go found": {"go"}, "no go binary": nil} {
		t.Run(name, func(t *testing.T) {
			isolateStatusEnv(t)
			t.Setenv("GOENV", "off")
			stubTools(t, binaries, map[string]string{"go env GOENV": "\n"})
			status, _ := requireStatus(t, project.Go)
			assert.Equal(t, StateNotConfigured, status.State)
			assert.Equal(t, CredentialsNotApplicable, status.Credentials)
		})
	}
}

func TestInspectMaven(t *testing.T) {
	settings := func(mirrorURL string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0">
  <servers><server><id>artifactory-mirror</id><username>admin</username><password>secret</password></server></servers>
  <mirrors>
    <mirror><id>corp</id><url>https://corp.example.com/maven</url><mirrorOf>central</mirrorOf></mirror>
    <mirror><id>artifactory-mirror</id><name>maven-virtual</name><url>` + mirrorURL + `</url><mirrorOf>external:*</mirrorOf></mirror>
  </mirrors>
</settings>`
	}
	t.Run("configured", func(t *testing.T) {
		home := isolateStatusEnv(t)
		writeTestFile(t, filepath.Join(home, ".m2", "settings.xml"), settings("https://acme.jfrog.io/artifactory/maven-virtual"))
		status, creds := requireStatus(t, project.Maven)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "maven-virtual", status.RepoKey)
		assert.Equal(t, CredentialsPresent, status.Credentials)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	t.Run("other host", func(t *testing.T) {
		home := isolateStatusEnv(t)
		writeTestFile(t, filepath.Join(home, ".m2", "settings.xml"), settings("https://other.jfrog.io/artifactory/maven-virtual"))
		status, _ := requireStatus(t, project.Maven)
		assert.Equal(t, StateOtherHost, status.State)
	})
	t.Run("non-UTF-8 declaration", func(t *testing.T) {
		home := isolateStatusEnv(t)
		latin1 := strings.Replace(settings("https://acme.jfrog.io/artifactory/maven-virtual"), `encoding="UTF-8"`, `encoding="ISO-8859-1"`, 1)
		writeTestFile(t, filepath.Join(home, ".m2", "settings.xml"), latin1)
		status, _ := requireStatus(t, project.Maven)
		assert.Equal(t, StateConfigured, status.State)
	})
	t.Run("missing file", func(t *testing.T) {
		isolateStatusEnv(t)
		status, _ := requireStatus(t, project.Maven)
		assert.Equal(t, StateNotConfigured, status.State)
	})
	t.Run("unparsable file", func(t *testing.T) {
		home := isolateStatusEnv(t)
		writeTestFile(t, filepath.Join(home, ".m2", "settings.xml"), "<settings><mirrors>")
		_, err := inspect(project.Maven, statusTestServer())
		assert.Error(t, err)
	})
}

func TestInspectGradle(t *testing.T) {
	isolateStatusEnv(t)
	gradleHome := t.TempDir()
	t.Setenv(gradle.UserHomeEnv, gradleHome)
	script, err := gradle.GenerateInitScript(gradle.InitScriptAuthConfig{
		ArtifactoryURL:         statusTestArtifactoryURL,
		GradleRepoName:         "gradle-virtual",
		ArtifactoryUsername:    "admin",
		ArtifactoryAccessToken: "tok",
	})
	require.NoError(t, err)
	require.NoError(t, gradle.WriteInitScript(script))

	status, creds := requireStatus(t, project.Gradle)
	assert.Equal(t, StateConfigured, status.State)
	assert.Equal(t, "gradle-virtual", status.RepoKey)
	assert.Equal(t, filepath.Join(gradleHome, "init.d", gradle.InitScriptName), status.Location)
	assert.Equal(t, storedCredentials{user: "admin", password: "tok"}, creds)
}

func TestInspectNuget(t *testing.T) {
	stubSourceState := func(t *testing.T, sourceURL string, enabled bool) {
		original := getNugetSourceURL
		t.Cleanup(func() { getNugetSourceURL = original })
		getNugetSourceURL = func(bidotnet.ToolchainType) (string, bool, error) { return sourceURL, enabled, nil }
	}
	stubSource := func(t *testing.T, sourceURL string) { stubSourceState(t, sourceURL, true) }
	// #nosec G101 -- False positive - fake credentials in a test fixture.
	credentialsConfig := `<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSourceCredentials>
    <JFrogCli>
      <add key="Username" value="admin" />
      <add key="ClearTextPassword" value="secret" />
    </JFrogCli>
  </packageSourceCredentials>
</configuration>`
	t.Run("binary missing", func(t *testing.T) {
		isolateStatusEnv(t)
		status, _ := requireStatus(t, project.Dotnet)
		assert.Equal(t, StateNotConfigured, status.State)
		assert.False(t, *status.BinaryFound)
	})
	t.Run("configured with clear-text credentials", func(t *testing.T) {
		home := isolateStatusEnv(t)
		stubTools(t, []string{"dotnet"}, nil)
		stubSource(t, "https://acme.jfrog.io/artifactory/api/nuget/v3/nuget-virtual/index.json")
		nugetConfig := filepath.Join(home, ".nuget", "NuGet", "NuGet.Config")
		if coreutils.IsWindows() {
			nugetConfig = filepath.Join(os.Getenv("APPDATA"), "NuGet", "NuGet.Config")
		}
		writeTestFile(t, nugetConfig, credentialsConfig)
		status, creds := requireStatus(t, project.Dotnet)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "nuget-virtual", status.RepoKey)
		assert.Equal(t, CredentialsPresent, status.Credentials)
		assert.Equal(t, nugetConfig, status.Location)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	t.Run("disabled source", func(t *testing.T) {
		isolateStatusEnv(t)
		stubTools(t, []string{"dotnet"}, nil)
		stubSourceState(t, "https://acme.jfrog.io/artifactory/api/nuget/v3/nuget-virtual/index.json", false)
		status, _ := requireStatus(t, project.Dotnet)
		assert.Equal(t, StateNotConfigured, status.State)
		assert.Empty(t, status.RepoKey)
	})
	t.Run("nuget ignores the dotnet user config", func(t *testing.T) {
		if coreutils.IsWindows() {
			t.Skip("both toolchains share %APPDATA%\\NuGet\\NuGet.Config on Windows")
		}
		home := isolateStatusEnv(t)
		stubTools(t, []string{"nuget"}, nil)
		stubSource(t, "https://acme.jfrog.io/artifactory/api/nuget/nuget-local")
		writeTestFile(t, filepath.Join(home, ".nuget", "NuGet", "NuGet.Config"), credentialsConfig)
		status, creds := requireStatus(t, project.Nuget)
		assert.Equal(t, filepath.Join(home, ".config", "NuGet", "NuGet.Config"), status.Location)
		assert.Equal(t, CredentialsAbsent, status.Credentials)
		assert.Equal(t, storedCredentials{}, creds)
	})
	t.Run("mono fallback only on linux", func(t *testing.T) {
		isolateStatusEnv(t)
		stubTools(t, []string{"mono", "nuget.exe"}, nil)
		stubSource(t, "https://acme.jfrog.io/artifactory/api/nuget/nuget-local")
		status, _ := requireStatus(t, project.Nuget)
		assert.Equal(t, runtime.GOOS == "linux", *status.BinaryFound)
	})
	t.Run("v2 source on another host", func(t *testing.T) {
		isolateStatusEnv(t)
		stubTools(t, []string{"nuget"}, nil)
		stubSource(t, "https://nuget.example.com/api/nuget/feed")
		status, _ := requireStatus(t, project.Nuget)
		assert.Equal(t, StateOtherHost, status.State)
	})
	t.Run("v2 source on the server", func(t *testing.T) {
		isolateStatusEnv(t)
		stubTools(t, []string{"nuget"}, nil)
		stubSource(t, "https://acme.jfrog.io/artifactory/api/nuget/nuget-local")
		status, _ := requireStatus(t, project.Nuget)
		assert.Equal(t, "nuget-local", status.RepoKey)
		assert.Equal(t, CredentialsAbsent, status.Credentials)
	})
}

// TestInspect_ReadsWhatSetupWrites builds each configuration with the same URL builders and
// writers `jf setup` uses, so a change to the URLs setup writes fails here rather than only in
// the jfrog-cli integration tests. Where setup hands the value to the package manager (npm,
// go, nuget), the test writes what that command stores.
func TestInspect_ReadsWhatSetupWrites(t *testing.T) {
	basicServer := statusTestServer()
	basicServer.User, basicServer.Password = "admin", "secret"

	t.Run("npm", func(t *testing.T) {
		home := isolateStatusEnv(t)
		tokenServer := statusTestServer()
		tokenServer.AccessToken = testCredential()
		repoURL := commandsutils.GetNpmRepositoryUrl("npm-virtual", tokenServer.ArtifactoryUrl) + "/"
		authKey, authValue := commandsutils.GetNpmAuthKeyValue(tokenServer, repoURL)
		writeTestFile(t, filepath.Join(home, ".npmrc"), "registry="+repoURL+"\n"+authKey+"="+authValue+"\n")
		status, creds := requireStatus(t, project.Npm)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "npm-virtual", status.RepoKey)
		assert.Equal(t, storedCredentials{token: tokenServer.AccessToken}, creds)
	})
	t.Run("pip", func(t *testing.T) {
		isolateStatusEnv(t)
		pipConfig := filepath.Join(t.TempDir(), "pip.conf")
		t.Setenv("PIP_CONFIG_FILE", pipConfig)
		repoURL, err := python.GetPypiRepoUrl(basicServer, "pypi-virtual", false)
		require.NoError(t, err)
		require.NoError(t, python.CreatePipConfigManually(pipConfig, repoURL))
		status, creds := requireStatus(t, project.Pip)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "pypi-virtual", status.RepoKey)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	t.Run("go", func(t *testing.T) {
		isolateStatusEnv(t)
		goProxy, err := golang.GetArtifactoryRemoteRepoUrl(basicServer, "go-virtual",
			golang.GoProxyUrlParams{Direct: true, FallbackOnlyIfNotFound: true})
		require.NoError(t, err)
		goEnv := filepath.Join(t.TempDir(), "env")
		writeTestFile(t, goEnv, "GOPROXY="+goProxy+"\n")
		t.Setenv("GOENV", goEnv)
		status, creds := requireStatus(t, project.Go)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "go-virtual", status.RepoKey)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	t.Run("maven", func(t *testing.T) {
		home := isolateStatusEnv(t)
		settingsXML, err := maven.NewSettingsXmlManagerWithPath(filepath.Join(home, ".m2", "settings.xml"))
		require.NoError(t, err)
		require.NoError(t, settingsXML.ConfigureArtifactoryRepository(basicServer.GetArtifactoryUrl(), "maven-virtual", "admin", "secret"))
		status, creds := requireStatus(t, project.Maven)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, "maven-virtual", status.RepoKey)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	for name, useNugetV2 := range map[string]bool{"nuget v3": false, "nuget v2": true} {
		t.Run(name, func(t *testing.T) {
			isolateStatusEnv(t)
			stubTools(t, []string{"dotnet"}, nil)
			sourceURL, _, _, err := dotnet.GetSourceDetails(basicServer, "nuget-virtual", useNugetV2)
			require.NoError(t, err)
			original := getNugetSourceURL
			t.Cleanup(func() { getNugetSourceURL = original })
			getNugetSourceURL = func(bidotnet.ToolchainType) (string, bool, error) { return sourceURL, true, nil }
			status, _ := requireStatus(t, project.Dotnet)
			assert.Equal(t, StateConfigured, status.State)
			assert.Equal(t, "nuget-virtual", status.RepoKey)
		})
	}
}

func TestInspectContainers(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	serverLogin := `{"https://acme.jfrog.io":"admin"}`
	otherLogin := `{"https://index.docker.io/v1/":"someone"}`
	tests := []struct {
		name        string
		authFile    string
		tools       map[string]string
		state       ConfigState
		credentials CredentialsState
		creds       storedCredentials
	}{
		{"missing file", "", nil, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"other registries only", `{"auths":{"https://index.docker.io/v1/":{"auth":"eA=="}}}`, nil, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"inline auth", `{"auths":{"acme.jfrog.io":{"auth":"` + basic + `"}}}`, nil, StateConfigured, CredentialsPresent, storedCredentials{user: "admin", password: "secret"}},
		{"credential store", `{"auths":{"https://acme.jfrog.io":{}},"credsStore":"desktop"}`, nil, StateConfigured, CredentialsPresent, storedCredentials{}},
		{"credential store without an auths entry", `{"auths":{},"credsStore":"osxkeychain"}`, map[string]string{"docker-credential-osxkeychain list": serverLogin}, StateConfigured, CredentialsPresent, storedCredentials{}},
		{"credential store without this server", `{"credsStore":"osxkeychain"}`, map[string]string{"docker-credential-osxkeychain list": otherLogin}, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"credential store that cannot run", `{"credsStore":"osxkeychain"}`, nil, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"credential store with a path is not run", `{"credsStore":"../evil"}`, map[string]string{"docker-credential-../evil list": serverLogin}, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"credential helper", `{"credHelpers":{"acme.jfrog.io":"ecr-login"}}`, map[string]string{"docker-credential-ecr-login list": serverLogin}, StateConfigured, CredentialsPresent, storedCredentials{}},
		{"credential helper without this server", `{"credHelpers":{"acme.jfrog.io":"ecr-login"}}`, map[string]string{"docker-credential-ecr-login list": otherLogin}, StateNotConfigured, CredentialsNotApplicable, storedCredentials{}},
		{"scheme and default port", `{"auths":{"https://acme.jfrog.io:443/v1/":{"auth":"` + basic + `"}}}`, nil, StateConfigured, CredentialsPresent, storedCredentials{user: "admin", password: "secret"}},
		{"duplicate keys prefer the one with a secret", `{"auths":{"https://acme.jfrog.io/v1/":{},"https://acme.jfrog.io":{"auth":"` + basic + `"}}}`, nil, StateConfigured, CredentialsPresent, storedCredentials{user: "admin", password: "secret"}},
		{"bare host wins", `{"auths":{"acme.jfrog.io":{},"https://acme.jfrog.io":{"auth":"` + basic + `"}}}`, nil, StateConfigured, CredentialsAbsent, storedCredentials{}},
	}
	for _, packageManager := range []project.ProjectType{project.Docker, project.Podman, project.Helm} {
		for _, test := range tests {
			t.Run(packageManager.String()+"/"+test.name, func(t *testing.T) {
				isolateStatusEnv(t)
				stubTools(t, nil, test.tools)
				authPath := filepath.Join(t.TempDir(), "auth.json")
				switch packageManager {
				case project.Docker:
					t.Setenv("DOCKER_CONFIG", filepath.Dir(authPath))
					authPath = filepath.Join(filepath.Dir(authPath), "config.json")
				case project.Podman:
					t.Setenv("REGISTRY_AUTH_FILE", authPath)
				case project.Helm:
					t.Setenv("HELM_REGISTRY_CONFIG", authPath)
				}
				if test.authFile != "" {
					writeTestFile(t, authPath, test.authFile)
				}
				status, creds := requireStatus(t, packageManager)
				assert.Equal(t, test.state, status.State)
				assert.Equal(t, test.credentials, status.Credentials)
				assert.Equal(t, test.creds, creds)
				assert.Equal(t, authPath, status.Location)
				assert.Empty(t, status.RepoKey)
			})
		}
	}
}

func TestPodmanAuthFiles(t *testing.T) {
	home := isolateStatusEnv(t)
	containersAuth := filepath.Join(home, ".config", "containers", "auth.json")
	dockerConfig := filepath.Join(home, ".docker", "config.json")

	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	files, err := podmanAuthFiles("linux")
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(runtimeDir, "containers", "auth.json"), containersAuth, dockerConfig}, files)

	files, err = podmanAuthFiles("darwin")
	require.NoError(t, err)
	assert.Equal(t, []string{containersAuth, dockerConfig}, files, "off Linux the primary file is the per-user one")

	custom := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("REGISTRY_AUTH_FILE", custom)
	files, err = podmanAuthFiles("linux")
	require.NoError(t, err)
	assert.Equal(t, []string{custom}, files, "REGISTRY_AUTH_FILE is the only file searched")
}

func TestInspectPodman_SearchesFallbackFiles(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("admin:secret"))
	setup := func(t *testing.T) (primary, dockerConfig string) {
		home := isolateStatusEnv(t)
		t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
		files, err := podmanAuthFiles(runtime.GOOS)
		require.NoError(t, err)
		return files[0], filepath.Join(home, ".docker", "config.json")
	}
	t.Run("login only in the Docker config", func(t *testing.T) {
		primary, dockerConfig := setup(t)
		writeTestFile(t, primary, `{"auths":{"quay.io":{"auth":"eA=="}}}`)
		writeTestFile(t, dockerConfig, `{"auths":{"acme.jfrog.io":{"auth":"`+basic+`"}}}`)
		status, creds := requireStatus(t, project.Podman)
		assert.Equal(t, StateConfigured, status.State)
		assert.Equal(t, dockerConfig, status.Location)
		assert.Equal(t, storedCredentials{user: "admin", password: "secret"}, creds)
	})
	t.Run("no login anywhere reports the primary file", func(t *testing.T) {
		primary, dockerConfig := setup(t)
		writeTestFile(t, dockerConfig, `{"auths":{"quay.io":{"auth":"eA=="}}}`)
		status, _ := requireStatus(t, project.Podman)
		assert.Equal(t, StateNotConfigured, status.State)
		assert.Equal(t, primary, status.Location)
	})
}

func TestHelmRegistryConfigFallback(t *testing.T) {
	home := isolateStatusEnv(t)
	helmHome := filepath.Join(home, "helm-home")
	t.Setenv("HELM_CONFIG_HOME", helmHome)
	path, err := helmRegistryConfig(false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(helmHome, "registry", "config.json"), path)

	t.Setenv("HELM_CONFIG_HOME", "")
	path, err = helmRegistryConfig(false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "helm", "registry", "config.json"), path, "XDG_CONFIG_HOME wins on every platform")
}

func TestProbeRepository(t *testing.T) {
	originalTimeout := deepProbeTimeout
	t.Cleanup(func() { deepProbeTimeout = originalTimeout })
	deepProbeTimeout = 200 * time.Millisecond

	tests := []struct {
		name     string
		status   int
		delay    time.Duration
		creds    storedCredentials
		state    CredentialsState
		expected DeepStatus
	}{
		{"ok with credentials", http.StatusOK, 0, storedCredentials{token: "tok"}, CredentialsPresent, DeepStatus{RepoReachable: true, AuthOk: ProbeTrue}},
		{"ok anonymously", http.StatusOK, 0, storedCredentials{}, CredentialsAbsent, DeepStatus{RepoReachable: true, AuthOk: ProbeUnknown}},
		{"rejected credentials", http.StatusUnauthorized, 0, storedCredentials{user: "u", password: "p"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeFalse, Error: "the stored credentials were rejected (HTTP 401)"}},
		{"forbidden", http.StatusForbidden, 0, storedCredentials{basicAuth: "dTpw"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeFalse, Error: "the stored credentials are not allowed to read repository repo (HTTP 403)"}},
		{"anonymous rejected", http.StatusUnauthorized, 0, storedCredentials{}, CredentialsAbsent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "the server requires credentials and none are stored (HTTP 401)"}},
		{"unreadable credentials rejected", http.StatusUnauthorized, 0, storedCredentials{}, CredentialsUnknown,
			DeepStatus{AuthOk: ProbeUnknown, Error: "the server requires credentials, and status cannot read the ones the package manager uses (HTTP 401)"}},
		{"redirect is not followed", http.StatusFound, 0, storedCredentials{token: "tok"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "unexpected response (HTTP 302)"}},
		{"unknown repository key", http.StatusBadRequest, 0, storedCredentials{token: "tok"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "repository repo was not found (HTTP 400)"}},
		{"missing repository", http.StatusNotFound, 0, storedCredentials{token: "tok"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "repository repo was not found (HTTP 404)"}},
		{"server error", http.StatusInternalServerError, 0, storedCredentials{token: "tok"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "unexpected response (HTTP 500)"}},
		{"timeout", http.StatusOK, time.Second, storedCredentials{token: "tok"}, CredentialsPresent,
			DeepStatus{AuthOk: ProbeUnknown, Error: "no response within 200ms"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			type request struct{ authorization, path string }
			// Handed over on a channel: after a timeout the handler is still running.
			requests := make(chan request, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- request{r.Header.Get("Authorization"), r.URL.Path}
				time.Sleep(test.delay)
				if test.status == http.StatusFound {
					w.Header().Set("Location", "/elsewhere")
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			result := probeRepository(&config.ServerDetails{ArtifactoryUrl: server.URL + "/artifactory/"}, "repo", test.creds, test.state)
			assert.Equal(t, test.expected, result)
			received := <-requests
			assert.Empty(t, requests, "the probe sends a single request")
			authorization := received.authorization
			assert.Equal(t, "/artifactory/api/repositories/repo", received.path)
			switch {
			case test.creds.token != "":
				assert.Equal(t, "Bearer tok", authorization)
			case test.creds.basicAuth != "":
				assert.Equal(t, "Basic dTpw", authorization)
			case test.creds.password != "":
				assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")), authorization)
			default:
				assert.Empty(t, authorization)
			}
		})
	}
}

func TestStatusTableRows_DeepError(t *testing.T) {
	status := PackageManagerStatus{PackageManager: "npm", State: StateConfigured,
		Deep: &DeepStatus{AuthOk: ProbeUnknown, Error: "no response within 5s"}}
	assert.Contains(t, statusTableRows(status), statusTableRow{"Deep check error", "no response within 5s"})
	status.Deep.Error = ""
	for _, row := range statusTableRows(status) {
		assert.NotEqual(t, "Deep check error", row.Field)
	}
}

func TestSetupStatusCommand_NoNetworkWithoutDeep(t *testing.T) {
	home := isolateStatusEnv(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	serverDetails := &config.ServerDetails{Url: server.URL + "/", ArtifactoryUrl: server.URL + "/artifactory/", AccessToken: "server-token"}
	token := testCredential()
	writeTestFile(t, filepath.Join(home, ".npmrc"), "registry="+server.URL+"/artifactory/api/npm/npm/\n"+
		"//"+strings.TrimPrefix(server.URL, "http://")+"/artifactory/api/npm/npm/:_authToken="+token+"\n")

	command := NewSetupStatusCommand(project.Npm).SetServerDetails(serverDetails).SetFormat(format.Json)
	require.NoError(t, command.Run())
	assert.Zero(t, requests.Load())
	assert.Nil(t, command.Result().Deep)
	usageServer, err := command.ServerDetails()
	require.NoError(t, err)
	assert.Nil(t, usageServer, "usage reporting would contact the server")

	require.NoError(t, command.SetDeep(true).Run())
	assert.Equal(t, int32(1), requests.Load())
	assert.Equal(t, &DeepStatus{RepoReachable: true, AuthOk: ProbeTrue}, command.Result().Deep)
	usageServer, err = command.ServerDetails()
	require.NoError(t, err)
	assert.Nil(t, usageServer, "deep mode sends only its probe")

	output, err := json.Marshal(command.Result())
	require.NoError(t, err)
	assert.NotContains(t, string(output), token)
	assert.NotContains(t, string(output), "server-token")
	for _, row := range statusTableRows(command.Result()) {
		assert.NotContains(t, row.Value, token)
	}
}

func TestSetupStatusCommand_JSONShape(t *testing.T) {
	status := PackageManagerStatus{SchemaVersion: 1, PackageManager: "npm", State: StateNotConfigured, Credentials: CredentialsNotApplicable, BinaryFound: boolPtr(true)}
	output, err := json.Marshal(status)
	require.NoError(t, err)
	assert.JSONEq(t, `{"schemaVersion":1,"packageManager":"npm","state":"not-configured","credentials":"not-applicable","binaryFound":true}`, string(output))
}
