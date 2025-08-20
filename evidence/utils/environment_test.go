package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvVariable_Exists(t *testing.T) {
	testKey := "TEST_ENV_VAR"
	testValue := "test_value"

	assert.NoError(t, os.Setenv(testKey, testValue))
	defer func() {
		assert.NoError(t, os.Unsetenv(testKey))
	}()

	result, err := GetEnvVariable(testKey)
	assert.NoError(t, err)
	assert.Equal(t, testValue, result)
}

func TestGetEnvVariable_NotExists(t *testing.T) {
	testKey := "NON_EXISTENT_ENV_VAR"

	assert.NoError(t, os.Unsetenv(testKey))

	result, err := GetEnvVariable(testKey)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), testKey)
	assert.Contains(t, err.Error(), "field wasn't provided")
}

func TestIsRunningUnderGitHubAction_True(t *testing.T) {
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "true"))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
	}()

	result := IsRunningUnderGitHubAction()
	assert.True(t, result)
}

func TestIsRunningUnderGitHubAction_False(t *testing.T) {
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "false"))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
	}()

	result := IsRunningUnderGitHubAction()
	assert.False(t, result)
}

func TestIsRunningUnderGitHubAction_NotSet(t *testing.T) {
	assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))

	result := IsRunningUnderGitHubAction()
	assert.False(t, result)
}

func TestIsRunningUnderGitHubAction_EmptyString(t *testing.T) {
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", ""))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
	}()

	result := IsRunningUnderGitHubAction()
	assert.False(t, result)
}

func TestIsRunningUnderGitHubAction_OtherValue(t *testing.T) {
	assert.NoError(t, os.Setenv("GITHUB_ACTIONS", "something_else"))
	defer func() {
		assert.NoError(t, os.Unsetenv("GITHUB_ACTIONS"))
	}()

	result := IsRunningUnderGitHubAction()
	assert.False(t, result)
}
