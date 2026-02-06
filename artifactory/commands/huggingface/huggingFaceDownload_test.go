package cli

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

func TestNewHFDownloadCmd(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	assert.NotNil(t, cmd)
	assert.IsType(t, &HuggingFaceDownload{}, cmd)
	assert.Empty(t, cmd.repoId)
	assert.Empty(t, cmd.revision)
	assert.Empty(t, cmd.repoType)
	assert.Zero(t, cmd.etagTimeout)
}

func TestHFDownloadCmd_SetRepoId(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	result := cmd.SetRepoId("test-repo")
	assert.Equal(t, cmd, result)
	assert.Equal(t, "test-repo", cmd.repoId)
}

func TestHFDownloadCmd_SetRevision(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	result := cmd.SetRevision("main")
	assert.Equal(t, cmd, result)
	assert.Equal(t, "main", cmd.revision)
}

func TestHFDownloadCmd_SetRepoType(t *testing.T) {
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
			cmd := NewHuggingFaceDownload()
			result := cmd.SetRepoType(tc.repoType)
			assert.Equal(t, cmd, result)
			assert.Equal(t, tc.expected, cmd.repoType)
		})
	}
}

func TestHFDownloadCmd_SetEtagTimeout(t *testing.T) {
	testCases := []struct {
		testName    string
		etagTimeout int
		expected    int
	}{
		{"set timeout to 86400", 86400, 86400},
		{"set timeout to 172800", 172800, 172800},
		{"set timeout to 0", 0, 0},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			cmd := NewHuggingFaceDownload()
			result := cmd.SetEtagTimeout(tc.etagTimeout)
			assert.Equal(t, cmd, result)
			assert.Equal(t, tc.expected, cmd.etagTimeout)
		})
	}
}

func TestHFDownloadCmd_CommandName(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	assert.Empty(t, cmd.CommandName())
	cmd.name = "test-command"
	assert.Equal(t, "test-command", cmd.CommandName())
}

func TestHFDownloadCmd_ServerDetails(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	serverDetails, err := cmd.ServerDetails()
	assert.NoError(t, err)
	assert.Nil(t, serverDetails)

	cmd.serverDetails = &config.ServerDetails{Url: "https://test.com"}
	serverDetails, err = cmd.ServerDetails()
	assert.NoError(t, err)
	assert.NotNil(t, serverDetails)
	assert.Equal(t, "https://test.com", serverDetails.Url)
}

func TestHFDownloadCmd_Run_EmptyRepoId(t *testing.T) {
	cmd := NewHuggingFaceDownload()
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repo_id cannot be empty")
}
