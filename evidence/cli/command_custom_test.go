package cli

import (
	"flag"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func TestEvidenceCustomCommand_CreateEvidence_SigstoreBundle(t *testing.T) {
	tests := []struct {
		name          string
		flags         []components.Flag
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid_SigstoreBundle_Without_SubjectSha256",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
			},
			expectError: false,
		},
		{
			name: "Invalid_SigstoreBundle_With_SubjectSha256",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
				setDefaultValue(subjectSha256, "abcd1234567890"),
			},
			expectError:   true,
			errorContains: "The parameter --subject-sha256 cannot be used with --sigstore-bundle",
		},
		{
			name: "Valid_No_SigstoreBundle_With_SubjectSha256",
			flags: []components.Flag{
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
				setDefaultValue(subjectSha256, "abcd1234567890"),
				setDefaultValue(predicate, "/path/to/predicate.json"),
				setDefaultValue(predicateType, "test-type"),
				setDefaultValue(key, "/path/to/key.pem"),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := cli.NewApp()
			app.Commands = []cli.Command{{Name: "create"}}
			set := flag.NewFlagSet("test", 0)
			cliCtx := cli.NewContext(app, set, nil)

			ctx, err := components.ConvertContext(cliCtx, tt.flags...)
			assert.NoError(t, err)

			var cmdError error
			mockExec := func(cmd commands.Command) error {
				// Mock successful execution
				return nil
			}

			cmd := NewEvidenceCustomCommand(ctx, mockExec)
			serverDetails := &config.ServerDetails{}

			err = cmd.CreateEvidence(ctx, serverDetails)
			if cmdError != nil {
				err = cmdError
			}

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
