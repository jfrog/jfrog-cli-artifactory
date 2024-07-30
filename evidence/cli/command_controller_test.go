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

	mockExec := func(command commands.Command) error {
		return nil
	}

	tests := []struct {
		name      string
		context   *components.Context
		expectErr bool
		mockExec  execCommandFunc
	}{
		{
			name:      "InvalidContext - Missing Subject",
			context:   createContext("somePredicate", "InToto", "PGP", "", ""),
			expectErr: true,
			mockExec:  mockExec,
		},
		{
			name:      "InvalidContext - Missing Predicate",
			context:   createContext("", "InToto", "PGP", "someBundle", ""),
			expectErr: true,
			mockExec:  mockExec,
		},
		{
			name:      "InvalidContext - Subject Duplication",
			context:   createContext("somePredicate", "InToto", "PGP", "someBundle", "path"),
			expectErr: true,
			mockExec:  mockExec,
		},
		{
			name:      "ValidContext - ReleaseBundle",
			context:   createContext("somePredicate", "InToto", "PGP", "someBundle:1", ""),
			expectErr: false,
			mockExec:  mockExec,
		},
		{
			name:      "ValidContext - RepoPath",
			context:   createContext("somePredicate", "InToto", "PGP", "", "path"),
			expectErr: false,
			mockExec:  mockExec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			execFunc = tt.mockExec             // Replace execFunc with the mockExec function
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
	setStringFlagValue(ctx, EvdPredicate, predicate)
	setStringFlagValue(ctx, EvdPredicateType, predicateType)
	setStringFlagValue(ctx, EvdKey, key)
	setStringFlagValue(ctx, EvdRepoPath, repoPath)
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
