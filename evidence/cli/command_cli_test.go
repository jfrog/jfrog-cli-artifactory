package cli

import (
	"flag"
	"os"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreUtils "github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
	"go.uber.org/mock/gomock"
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

			err := verifyEvidence(context)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateEvidenceValidation_SigstoreBundle(t *testing.T) {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "create",
		},
	}
	ctx := cli.NewContext(app, &flag.FlagSet{}, nil)

	tests := []struct {
		name          string
		flags         []components.Flag
		expectError   bool
		errorContains string
	}{
		{
			name: "ValidContext_-_SigstoreBundle_Without_Predicate",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
			},
			expectError: false,
		},
		{
			name: "ValidContext_-_SigstoreBundle_Without_Any_Subject",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				// No subject fields provided - should still pass since subject is extracted from bundle
			},
			expectError: false,
		},
		{
			name: "InvalidContext_-_Missing_Predicate_Without_SigstoreBundle",
			flags: []components.Flag{
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
				setDefaultValue(key, "/path/to/key.pem"),
			},
			expectError:   true,
			errorContains: "'predicate' is a mandatory field",
		},
		{
			name: "InvalidContext_-_Missing_PredicateType_Without_SigstoreBundle",
			flags: []components.Flag{
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
				setDefaultValue(predicate, "/path/to/predicate.json"),
				setDefaultValue(key, "/path/to/key.pem"),
			},
			expectError:   true,
			errorContains: "'predicate-type' is a mandatory field",
		},
		{
			name: "InvalidContext_-_SigstoreBundle_With_Key",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(key, "/path/to/key.pem"),
			},
			expectError:   true,
			errorContains: "The following parameters cannot be used with --sigstore-bundle: --key",
		},
		{
			name: "InvalidContext_-_SigstoreBundle_With_KeyAlias",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(keyAlias, "my-key-alias"),
			},
			expectError:   true,
			errorContains: "The following parameters cannot be used with --sigstore-bundle: --key-alias",
		},
		{
			name: "InvalidContext_-_SigstoreBundle_With_Predicate",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(predicate, "/path/to/predicate.json"),
			},
			expectError:   true,
			errorContains: "The following parameters cannot be used with --sigstore-bundle: --predicate",
		},
		{
			name: "InvalidContext_-_SigstoreBundle_With_PredicateType",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(predicateType, "test-type"),
			},
			expectError:   true,
			errorContains: "The following parameters cannot be used with --sigstore-bundle: --predicate-type",
		},
		{
			name: "InvalidContext_-_SigstoreBundle_With_Multiple_Conflicting_Params",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(key, "/path/to/key.pem"),
				setDefaultValue(keyAlias, "my-key-alias"),
				setDefaultValue(predicate, "/path/to/predicate.json"),
				setDefaultValue(predicateType, "test-type"),
			},
			expectError:   true,
			errorContains: "The following parameters cannot be used with --sigstore-bundle: --key, --key-alias, --predicate, --predicate-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, err := components.ConvertContext(ctx, tt.flags...)
			assert.NoError(t, err)

			err = validateCreateEvidenceCommonContext(context)
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

func TestGetAndValidateSubject_SigstoreBundle(t *testing.T) {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "create",
		},
	}
	ctx := cli.NewContext(app, &flag.FlagSet{}, nil)

	tests := []struct {
		name            string
		flags           []components.Flag
		expectError     bool
		expectedSubject []string
	}{
		{
			name: "SigstoreBundle_NoSubjectFields",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
			},
			expectError:     false,
			expectedSubject: []string{subjectRepoPath},
		},
		{
			name: "SigstoreBundle_WithSubjectRepoPath",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
			},
			expectError:     false,
			expectedSubject: []string{subjectRepoPath},
		},
		{
			name:  "NoSigstoreBundle_NoSubject_ShouldFail",
			flags: []components.Flag{
				// No sigstore bundle and no subject fields
			},
			expectError: true,
		},
		{
			name: "NoSigstoreBundle_WithSubject_ShouldPass",
			flags: []components.Flag{
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
			},
			expectError:     false,
			expectedSubject: []string{subjectRepoPath},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, err := components.ConvertContext(ctx, tt.flags...)
			assert.NoError(t, err)

			subjects, err := getAndValidateSubject(context)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSubject, subjects)
			}
		})
	}
}

func TestValidateSigstoreBundleConflicts(t *testing.T) {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		{
			Name: "create",
		},
	}
	set := flag.NewFlagSet("create", 0)
	ctx := cli.NewContext(app, set, nil)

	tests := []struct {
		name          string
		flags         []components.Flag
		expectError   bool
		errorContains string
	}{
		{
			name: "No_Conflicts",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(subjectRepoPath, "test-repo/test-artifact"),
			},
			expectError: false,
		},
		{
			name: "Conflict_With_Key",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(key, "/path/to/key"),
			},
			expectError:   true,
			errorContains: "--key",
		},
		{
			name: "Conflict_With_Multiple_Params",
			flags: []components.Flag{
				setDefaultValue(sigstoreBundle, "/path/to/bundle.json"),
				setDefaultValue(key, "/path/to/key"),
				setDefaultValue(keyAlias, "my-key"),
				setDefaultValue(predicate, "/path/to/predicate"),
			},
			expectError:   true,
			errorContains: "--key, --key-alias, --predicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, err := components.ConvertContext(ctx, tt.flags...)
			if err != nil {
				t.Fatal(err)
			}

			err = validateSigstoreBundleArgsConflicts(context)

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

func setDefaultValue(flag string, defaultValue string) components.Flag {
	f := components.NewStringFlag(flag, flag)
	f.DefaultValue = defaultValue
	return f
}
