package python

import (
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPypiRepoUrlWithCredentials(t *testing.T) {
	testCases := []struct {
		name        string
		curationCmd bool
	}{
		{
			name:        "test curation command true",
			curationCmd: true,
		},
		{
			name:        "test curation command false",
			curationCmd: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			url, _, _, err := GetPypiRepoUrlWithCredentials(&config.ServerDetails{}, "test", testCase.curationCmd)
			require.NoError(t, err)
			assert.Equal(t, testCase.curationCmd, strings.Contains(url.Path, coreutils.CurationPassThroughApi))
		})
	}
}

func TestGetExecutable(t *testing.T) {
	testCases := []struct {
		name         string
		buildTool    project.ProjectType
		expectedName string
	}{
		{
			name:         "npm build tool uses direct name",
			buildTool:    project.Npm,
			expectedName: "npm",
		},
		{
			name:         "pipenv build tool uses direct name",
			buildTool:    project.Pipenv,
			expectedName: "pipenv",
		},
		{
			name:      "pip build tool uses detection",
			buildTool: project.Pip,
			// expectedName will be validated separately since it depends on system
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			execName, err := getExecutable(testCase.buildTool)
			assert.NoError(t, err)

			if testCase.buildTool == project.Pip {
				// For pip, should return either "pip" or "pip3"
				assert.True(t, execName == "pip" || execName == "pip3",
					"Expected 'pip' or 'pip3', got: %s", execName)
			} else {
				// For other tools, should return exact tool name
				assert.Equal(t, testCase.expectedName, execName)
			}
		})
	}
}
