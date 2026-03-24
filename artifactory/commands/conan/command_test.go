package conan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConanCommand(t *testing.T) {
	cmd := NewConanCommand()

	assert.NotNil(t, cmd)
	assert.Empty(t, cmd.commandName)
	assert.Nil(t, cmd.args)
	assert.Nil(t, cmd.serverDetails)
	assert.Nil(t, cmd.buildConfiguration)
}

func TestConanCommand_SetCommandName(t *testing.T) {
	cmd := NewConanCommand()

	result := cmd.SetCommandName("install")

	assert.Equal(t, "install", cmd.commandName)
	assert.Same(t, cmd, result, "SetCommandName should return same instance for chaining")
}

func TestConanCommand_SetArgs(t *testing.T) {
	cmd := NewConanCommand()
	args := []string{".", "--build=missing"}

	result := cmd.SetArgs(args)

	assert.Equal(t, args, cmd.args)
	assert.Same(t, cmd, result, "SetArgs should return same instance for chaining")
}

func TestConanCommand_SetServerDetails(t *testing.T) {
	cmd := NewConanCommand()
	serverDetails := &config.ServerDetails{
		ServerId: "test-server",
	}

	result := cmd.SetServerDetails(serverDetails)

	assert.Equal(t, serverDetails, cmd.serverDetails)
	assert.Same(t, cmd, result, "SetServerDetails should return same instance for chaining")
}

func TestConanCommand_CommandName(t *testing.T) {
	cmd := NewConanCommand()

	result := cmd.CommandName()

	assert.Equal(t, "rt_conan", result)
}

func TestConanCommand_ServerDetails(t *testing.T) {
	cmd := NewConanCommand()
	serverDetails := &config.ServerDetails{
		ServerId: "test-server",
	}
	cmd.serverDetails = serverDetails

	result, err := cmd.ServerDetails()

	assert.NoError(t, err)
	assert.Equal(t, serverDetails, result)
}

func TestConanCommand_GetCmd(t *testing.T) {
	cmd := NewConanCommand()
	cmd.commandName = "install"
	cmd.args = []string{".", "--build=missing"}

	execCmd := cmd.GetCmd()

	assert.NotNil(t, execCmd)
	assert.Equal(t, "conan", execCmd.Path[len(execCmd.Path)-5:]) // ends with "conan"
	assert.Contains(t, execCmd.Args, "install")
	assert.Contains(t, execCmd.Args, ".")
	assert.Contains(t, execCmd.Args, "--build=missing")
}

func TestConanCommand_GetEnv(t *testing.T) {
	cmd := NewConanCommand()

	env := cmd.GetEnv()

	assert.NotNil(t, env)
	assert.Empty(t, env)
}

func TestConanCommand_GetStdWriter(t *testing.T) {
	cmd := NewConanCommand()

	writer := cmd.GetStdWriter()

	assert.Nil(t, writer)
}

func TestConanCommand_GetErrWriter(t *testing.T) {
	cmd := NewConanCommand()

	writer := cmd.GetErrWriter()

	assert.Nil(t, writer)
}

func TestConanCommand_ChainedSetters(t *testing.T) {
	serverDetails := &config.ServerDetails{ServerId: "test"}

	cmd := NewConanCommand().
		SetCommandName("upload").
		SetArgs([]string{"pkg/1.0", "-r", "remote"}).
		SetServerDetails(serverDetails)

	assert.Equal(t, "upload", cmd.commandName)
	assert.Equal(t, []string{"pkg/1.0", "-r", "remote"}, cmd.args)
	assert.Equal(t, serverDetails, cmd.serverDetails)
}

func TestExtractRecipePathFromArgs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "conan-recipe-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	subDir := filepath.Join(tempDir, "recipes", "mylib")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	tests := []struct {
		name     string
		cwd      string
		args     []string
		expected string
	}{
		{
			name:     "dot path resolves to cwd",
			cwd:      subDir,
			args:     []string{".", "--build=missing"},
			expected: subDir,
		},
		{
			name:     "relative subdir path",
			cwd:      tempDir,
			args:     []string{filepath.Join("recipes", "mylib"), "--build=missing"},
			expected: subDir,
		},
		{
			name:     "absolute path",
			cwd:      tempDir,
			args:     []string{subDir, "--build=missing"},
			expected: subDir,
		},
		{
			name:     "no path arg returns empty",
			cwd:      tempDir,
			args:     []string{"--build=missing", "-r", "my-remote"},
			expected: "",
		},
		{
			name:     "skips flags",
			cwd:      tempDir,
			args:     []string{"--build=missing", subDir},
			expected: subDir,
		},
		{
			name:     "skips conan reference with @",
			cwd:      tempDir,
			args:     []string{"pkg/1.0@user/channel", "-r", "remote"},
			expected: "",
		},
		{
			name:     "nonexistent path returns empty",
			cwd:      tempDir,
			args:     []string{"/nonexistent/path"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRecipePathFromArgs(tt.cwd, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractReferenceOverridesFromArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedName    string
		expectedVersion string
		expectedUser    string
		expectedChannel string
	}{
		{
			name:            "all overrides equals form",
			args:            []string{".", "--name=mylib", "--version=1.0.0", "--user=myuser", "--channel=stable", "--build=missing"},
			expectedName:    "mylib",
			expectedVersion: "1.0.0",
			expectedUser:    "myuser",
			expectedChannel: "stable",
		},
		{
			name:            "all overrides space form",
			args:            []string{".", "--name", "mylib", "--version", "1.0.0", "--user", "myuser", "--channel", "stable"},
			expectedName:    "mylib",
			expectedVersion: "1.0.0",
			expectedUser:    "myuser",
			expectedChannel: "stable",
		},
		{
			name:            "only name and version",
			args:            []string{"--name=mylib", "--version=1.0.0"},
			expectedName:    "mylib",
			expectedVersion: "1.0.0",
			expectedUser:    "",
			expectedChannel: "",
		},
		{
			name:            "only user and channel",
			args:            []string{".", "--user=admin", "--channel=testing"},
			expectedName:    "",
			expectedVersion: "",
			expectedUser:    "admin",
			expectedChannel: "testing",
		},
		{
			name:            "only name",
			args:            []string{"--name=mylib", "--build=missing"},
			expectedName:    "mylib",
			expectedVersion: "",
			expectedUser:    "",
			expectedChannel: "",
		},
		{
			name:            "only version",
			args:            []string{"--version=2.0"},
			expectedName:    "",
			expectedVersion: "2.0",
			expectedUser:    "",
			expectedChannel: "",
		},
		{
			name:            "no overrides",
			args:            []string{".", "--build=missing"},
			expectedName:    "",
			expectedVersion: "",
			expectedUser:    "",
			expectedChannel: "",
		},
		{
			name:            "empty args",
			args:            []string{},
			expectedName:    "",
			expectedVersion: "",
			expectedUser:    "",
			expectedChannel: "",
		},
		{
			name:            "mixed with requires",
			args:            []string{"--requires", "zlib/1.2.11", "--name=mypkg", "--version=3.0", "--user=ci", "--channel=dev"},
			expectedName:    "mypkg",
			expectedVersion: "3.0",
			expectedUser:    "ci",
			expectedChannel: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := extractReferenceOverridesFromArgs(tt.args)
			assert.Equal(t, tt.expectedName, o.name)
			assert.Equal(t, tt.expectedVersion, o.version)
			assert.Equal(t, tt.expectedUser, o.user)
			assert.Equal(t, tt.expectedChannel, o.channel)
		})
	}
}
