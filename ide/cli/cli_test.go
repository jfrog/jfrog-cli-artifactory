package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func newMockContext(args ...string) *components.Context {
	ctx := &components.Context{}
	ctx.Arguments = args
	return ctx
}

func TestSetupCmd(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedError error
	}{
		{
			name:          "Missing argument",
			args:          []string{},
			expectedError: errors.New("error: Missing mandatory argument 'IDE_NAME'. Please specify ide name. Supported IDEs are 'vscode' or 'jetbrains'"),
		},
		{
			name:          "Invalid IDE name",
			args:          []string{"eclipse"},
			expectedError: errors.New("error: Invalid IDE name 'eclipse'. Supported IDEs are 'vscode' or 'jetbrains'"),
		},
		{
			name:          "Valid IDE vscode",
			args:          []string{"vscode"},
			expectedError: nil,
		},
		{
			name:          "Valid IDE jetbrains",
			args:          []string{"jetbrains"},
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newMockContext(tt.args...)
			err := setupCmd(ctx)
			if tt.expectedError == nil {
				assert.NoError(t, err, "Expected no error for %s", tt.name)
			} else if assert.Error(t, err, "Expected an error for %s", tt.name) {
				assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()),
					"Expected error :\n%s\nbut got:\n%s", tt.expectedError, err.Error())
			}
		})
	}
}
