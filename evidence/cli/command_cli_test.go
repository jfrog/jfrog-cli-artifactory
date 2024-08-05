package cli

import (
	"flag"
	"github.com/golang/mock/gomock"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
	"testing"
)

func TestCreateEvidence_Context(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "create",
		},
	}
	set := flag.NewFlagSet(predicate, 0)
	ctx := cli.NewContext(app, set, nil)

	tests := []struct {
		name      string
		flags     []components.Flag
		expectErr bool
	}{
		{
			name: "InvalidContext - Missing Subject",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, predicateType),
				setDefaultValue(key, key),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Missing Predicate",
			flags: []components.Flag{
				setDefaultValue("", ""),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Subject Duplication",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(subjectRepoPath, subjectRepoPath),
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue(releaseBundleVersion, releaseBundleVersion),
			},
			expectErr: true,
		},
		{
			name: "ValidContext - ReleaseBundle",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue(releaseBundleVersion, releaseBundleVersion),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "ValidContext - RepoPath",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(subjectRepoPath, subjectRepoPath),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "ValidContext - Build",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(buildName, buildName),
				setDefaultValue(buildNumber, buildNumber),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, err1 := components.ConvertContext(ctx, tt.flags...)
			if err1 != nil {
				return
			}

			execFunc = func(command commands.Command) error {
				return nil
			}
			// Replace execFunc with the mockExec function
			defer func() { execFunc = exec }() // Restore original execFunc after test

			err := createEvidence(context)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func setDefaultValue(flag string, defaultValue string) components.Flag {
	f := components.NewStringFlag(flag, flag)
	f.DefaultValue = defaultValue
	return f
}
