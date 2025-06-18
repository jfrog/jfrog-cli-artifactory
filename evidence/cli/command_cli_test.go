package cli

import (
	"flag"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreUtils "github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
	"go.uber.org/mock/gomock"
	"os"
	"testing"
)

func TestCreateEvidence_Context(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	assert.NoError(t, os.Setenv(coreUtils.SigningKey, "PGP"), "Failed to set env: "+coreUtils.SigningKey)
	assert.NoError(t, os.Setenv(coreUtils.BuildName, buildName), "Failed to set env: JFROG_CLI_BUILD_NAME")
	defer os.Unsetenv(coreUtils.SigningKey)
	defer os.Unsetenv(coreUtils.BuildName)

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
		{
			name: "ValidContext - Build With BuildNumber As Env Var",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(buildNumber, buildNumber),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "InvalidContext - Build",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(buildName, buildName),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "ValidContext - Package",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageVersion, packageVersion),
				setDefaultValue(packageRepoName, packageRepoName),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "ValidContext With Key As Env Var- Package",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageVersion, packageVersion),
				setDefaultValue(packageRepoName, packageRepoName),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "InvalidContext - Missing package version",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageRepoName, packageRepoName),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Missing package repository key",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageVersion, packageVersion),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Unsupported Basic Auth",
			flags: []components.Flag{
				setDefaultValue(predicate, predicate),
				setDefaultValue(predicateType, "InToto"),
				setDefaultValue(key, "PGP"),
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue("url", "url"),
				setDefaultValue("user", "testUser"),
				setDefaultValue("password", "testPassword"),
			},
			expectErr: true,
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

func TestVerifyEvidence_Context(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	assert.NoError(t, os.Setenv(coreUtils.SigningKey, "PGP"), "Failed to set env: "+coreUtils.SigningKey)
	assert.NoError(t, os.Setenv(coreUtils.BuildName, buildName), "Failed to set env: JFROG_CLI_BUILD_NAME")
	defer os.Unsetenv(coreUtils.SigningKey)
	defer os.Unsetenv(coreUtils.BuildName)

	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "verify",
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
				setDefaultValue(publicKeys, "PGP"),
				setDefaultValue(format, "json"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Subject Duplication",
			flags: []components.Flag{
				setDefaultValue(publicKeys, "PGP"),
				setDefaultValue(subjectRepoPath, subjectRepoPath),
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue(releaseBundleVersion, releaseBundleVersion),
			},
			expectErr: true,
		},
		{
			name: "ValidContext - ReleaseBundle",
			flags: []components.Flag{
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue(releaseBundleVersion, releaseBundleVersion),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "ValidContext - RepoPath",
			flags: []components.Flag{
				setDefaultValue(subjectRepoPath, subjectRepoPath),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "ValidContext - Build",
			flags: []components.Flag{
				setDefaultValue(publicKeys, "PGP"),
				setDefaultValue(format, "full"),
				setDefaultValue(buildName, buildName),
				setDefaultValue(buildNumber, buildNumber),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "InvalidContext - Build",
			flags: []components.Flag{
				setDefaultValue(buildName, buildName),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "ValidContext - Package",
			flags: []components.Flag{
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageVersion, packageVersion),
				setDefaultValue(packageRepoName, packageRepoName),
				setDefaultValue("url", "url"),
			},
			expectErr: false,
		},
		{
			name: "InvalidContext - Missing package version",
			flags: []components.Flag{
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageRepoName, packageRepoName),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Missing package repository key",
			flags: []components.Flag{
				setDefaultValue(packageName, packageName),
				setDefaultValue(packageVersion, packageVersion),
				setDefaultValue("url", "url"),
			},
			expectErr: true,
		},
		{
			name: "InvalidContext - Unsupported Basic Auth",
			flags: []components.Flag{
				setDefaultValue(releaseBundle, releaseBundle),
				setDefaultValue("url", "url"),
				setDefaultValue("user", "testUser"),
				setDefaultValue("password", "testPassword"),
			},
			expectErr: true,
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

			err := verifyEvidences(context)
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
