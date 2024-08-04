package cli

import (
	"github.com/golang/mock/gomock"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	"reflect"
	"testing"
	"unsafe"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
)

func TestCreateEvidence_Context(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name      string
		context   *components.Context
		expectErr bool
	}{
		{
			name:      "InvalidContext - Missing Subject",
			context:   createCustomContext("somePredicate", "InToto", "PGP", "", ""),
			expectErr: true,
		},
		{
			name:      "InvalidContext - Missing Predicate",
			context:   createCustomContext("", "InToto", "PGP", "someBundle", ""),
			expectErr: true,
		},
		{
			name:      "InvalidContext - Subject Duplication",
			context:   createCustomAndRBContext("somePredicate", "InToto", "PGP", "someBundle", "1.0.0", "rb", "rbv"),
			expectErr: true,
		},
		{
			name:      "ValidContext - ReleaseBundle",
			context:   createRBContext("somePredicate", "InToto", "PGP", "someBundle:1", "1.0.0"),
			expectErr: false,
		},
		{
			name:      "ValidContext - RepoPath",
			context:   createCustomContext("somePredicate", "InToto", "PGP", "path", "sha256"),
			expectErr: false,
		},
		{
			name:      "ValidContext - Build",
			context:   createBuildContext("somePredicate", "InToto", "PGP", "name", "number"),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			execFunc = func(command commands.Command) error {
				return nil
			}

			// Replace execFunc with the mockExec function
			defer func() { execFunc = exec }() // Restore original execFunc after test

			err := createEvidence(tt.context)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func createCommonContext(ctx *components.Context, _predicate, _predicateType, _key string) *components.Context {
	setStringFlagValue(ctx, predicate, _predicate)
	setStringFlagValue(ctx, predicateType, _predicateType)
	setStringFlagValue(ctx, key, _key)
	return ctx
}

func createCustomAndRBContext(_predicate, _predicateType, _key, repoPath, sha256, rb, rbv string) *components.Context {
	ctx := &components.Context{
		Arguments: []string{},
	}
	createCommonContext(ctx, _predicate, _predicateType, _key)
	setStringFlagValue(ctx, subjectRepoPath, repoPath)
	setStringFlagValue(ctx, subjectSha256, sha256)
	setStringFlagValue(ctx, releaseBundle, rb)
	setStringFlagValue(ctx, releaseBundleVersion, rbv)
	return ctx
}

func createCustomContext(_predicate, _predicateType, _key, repoPath, sha256 string) *components.Context {
	ctx := &components.Context{
		Arguments: []string{},
	}
	createCommonContext(ctx, _predicate, _predicateType, _key)
	setStringFlagValue(ctx, subjectRepoPath, repoPath)
	setStringFlagValue(ctx, subjectSha256, sha256)
	return ctx
}

func createRBContext(_predicate, _predicateType, _key, rb, rbv string) *components.Context {
	ctx := &components.Context{
		Arguments: []string{},
	}
	createCommonContext(ctx, _predicate, _predicateType, _key)
	setStringFlagValue(ctx, releaseBundle, rb)
	setStringFlagValue(ctx, releaseBundleVersion, rbv)
	return ctx
}

func createBuildContext(_predicate, _predicateType, _key, _buildName, _buildNumber string) *components.Context {
	ctx := &components.Context{
		Arguments: []string{},
	}
	createCommonContext(ctx, _predicate, _predicateType, _key)
	setStringFlagValue(ctx, buildName, _buildName)
	setStringFlagValue(ctx, buildNumber, _buildNumber)
	return ctx
}

func setStringFlagValue(ctx *components.Context, flagName, value string) {
	val := reflect.ValueOf(ctx).Elem()
	stringFlags := val.FieldByName("stringFlags")

	// If the field is not settable, we need to make it settable
	if !stringFlags.CanSet() {
		stringFlags = reflect.NewAt(stringFlags.Type(), unsafe.Pointer(stringFlags.UnsafeAddr())).Elem()
	}

	if stringFlags.IsNil() {
		stringFlags.Set(reflect.MakeMap(stringFlags.Type()))
	}
	stringFlags.SetMapIndex(reflect.ValueOf(flagName), reflect.ValueOf(value))
}
