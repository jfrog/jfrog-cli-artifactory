package commands

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func TestSetupCmd(t *testing.T) {
	tests := []struct {
		name        string
		ideName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "VSCode setup",
			ideName:     "vscode",
			expectError: true,
			errorMsg:    "--repo-key flag is required",
		},
		{
			name:        "JetBrains setup",
			ideName:     "jetbrains",
			expectError: true,
			errorMsg:    "--repo-key flag is required",
		},
		{
			name:        "Unsupported IDE",
			ideName:     "eclipse",
			expectError: true,
			errorMsg:    "unsupported IDE: eclipse",
		},
		{
			name:        "Empty IDE name",
			ideName:     "",
			expectError: true,
			errorMsg:    "unsupported IDE: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &components.Context{
				PrintCommandHelp: func(commandName string) error {
					return nil
				},
			}
			err := SetupCmd(ctx, tt.ideName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSetupFlags(t *testing.T) {
	flags := GetSetupFlags()

	assert.NotEmpty(t, flags)

	hasRepoKeyFlag := false
	for _, flag := range flags {
		if flag.GetName() == "repo-key" {
			hasRepoKeyFlag = true
			break
		}
	}
	assert.True(t, hasRepoKeyFlag, "repo-key flag should be present")
}
