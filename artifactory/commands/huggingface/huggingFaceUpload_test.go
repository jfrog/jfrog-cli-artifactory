package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

func TestNewHFUploadCmd(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	assert.NotNil(t, cmd)
	assert.IsType(t, &HuggingFaceUpload{}, cmd)
	assert.Empty(t, cmd.folderPath)
	assert.Empty(t, cmd.repoId)
	assert.Empty(t, cmd.revision)
	assert.Empty(t, cmd.repoType)
}

func TestHFUploadCmd_SetFolderPath(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	result := cmd.SetFolderPath("/path/to/folder")
	assert.Equal(t, cmd, result)
	assert.Equal(t, "/path/to/folder", cmd.folderPath)
}

func TestHFUploadCmd_SetRepoId(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	result := cmd.SetRepoId("test-repo")
	assert.Equal(t, cmd, result)
	assert.Equal(t, "test-repo", cmd.repoId)
}

func TestHFUploadCmd_SetRevision(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	result := cmd.SetRevision("main")
	assert.Equal(t, cmd, result)
	assert.Equal(t, "main", cmd.revision)
}

func TestHFUploadCmd_SetRepoType(t *testing.T) {
	testCases := []struct {
		testName string
		repoType string
		expected string
	}{
		{"set model type", "model", "model"},
		{"set dataset type", "dataset", "dataset"},
		{"set space type", "space", "space"},
		{"set empty type", "", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			cmd := NewHuggingFaceUpload()
			result := cmd.SetRepoType(tc.repoType)
			assert.Equal(t, cmd, result)
			assert.Equal(t, tc.expected, cmd.repoType)
		})
	}
}

func TestHFUploadCmd_CommandName(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	assert.Empty(t, cmd.CommandName())
	cmd.name = "test-command"
	assert.Equal(t, "test-command", cmd.CommandName())
}

func TestHFUploadCmd_ServerDetails(t *testing.T) {
	cmd := NewHuggingFaceUpload()
	serverDetails, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Nil(t, serverDetails)

	cmd.serverDetails = &config.ServerDetails{Url: "https://test.com"}
	serverDetails, err = cmd.ServerDetails()
	assert.NoError(t, err)
	assert.NotNil(t, serverDetails)
	assert.Equal(t, "https://test.com", serverDetails.Url)
}

func TestHFUploadCmd_Run_EmptyFolderPath(t *testing.T) {
	cmd := NewHuggingFaceUpload().SetRepoId("test-repo")
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "folder_path cannot be empty")
}

func TestHFUploadCmd_Run_EmptyRepoId(t *testing.T) {
	cmd := NewHuggingFaceUpload().SetFolderPath("/path/to/folder")
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repo_id cannot be empty")
}
