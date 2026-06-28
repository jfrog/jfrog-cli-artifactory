package container

import (
	"os"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerCommandBase_init_SetsWorkingDirectoryWhenRepoPreSet(t *testing.T) {
	expectedWD, err := os.Getwd()
	require.NoError(t, err)

	ccb := &ContainerCommandBase{}
	ccb.SetRepo("my-repo")
	ccb.SetBuildConfiguration(build.NewBuildConfiguration("build-name", "1", "", ""))

	err = ccb.init()
	require.NoError(t, err)
	assert.Equal(t, expectedWD, ccb.workingDirectory)
}

func TestContainerCommandBase_init_PreservesExistingWorkingDirectory(t *testing.T) {
	const presetWD = "/preset/working/dir"

	ccb := &ContainerCommandBase{}
	ccb.workingDirectory = presetWD
	ccb.SetRepo("my-repo")
	ccb.SetBuildConfiguration(build.NewBuildConfiguration("build-name", "1", "", ""))

	err := ccb.init()
	require.NoError(t, err)
	assert.Equal(t, presetWD, ccb.workingDirectory)
}
