package conan

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
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
