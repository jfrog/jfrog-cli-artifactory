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
			context:   createContext("somePredicate", "InToto", "PGP", "", ""),
			expectErr: true,
		},
		{
			name:      "InvalidContext - Missing Predicate",
			context:   createContext("", "InToto", "PGP", "someBundle", ""),
			expectErr: true,
		},
		{
			name:      "InvalidContext - Subject Duplication",
			context:   createContext("somePredicate", "InToto", "PGP", "someBundle", "path"),
			expectErr: true,
		},
		{
			name:      "ValidContext - ReleaseBundle",
			context:   createContext("somePredicate", "InToto", "PGP", "someBundle:1", ""),
			expectErr: false,
		},
		{
			name:      "ValidContext - RepoPath",
			context:   createContext("somePredicate", "InToto", "PGP", "", "path"),
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

func createContext(predicate string, predicateType string, key string, rb string, repoPath string) *components.Context {
	ctx := &components.Context{
		Arguments: []string{},
	}
	setStringFlagValue(ctx, predicate, predicate)
	setStringFlagValue(ctx, predicateType, predicateType)
	setStringFlagValue(ctx, key, key)
	setStringFlagValue(ctx, repoPath, repoPath)
	setStringFlagValue(ctx, releaseBundle, rb)
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
