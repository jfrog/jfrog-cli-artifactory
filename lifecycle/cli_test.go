package lifecycle

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	clientTestUtils "github.com/jfrog/jfrog-client-go/utils/tests"
	"github.com/stretchr/testify/assert"
)

func TestValidateCreateReleaseBundleContext(t *testing.T) {
	testRuns := []struct {
		name        string
		args        []string
		flags       []string
		expectError bool
	}{
		{"withoutArgs", []string{}, []string{}, true},
		{"oneArg", []string{"one"}, []string{}, true},
		{"twoArgs", []string{"one", "two"}, []string{}, true},
		{"extraArgs", []string{"one", "two", "three", "four"}, []string{}, true},
		{"bothSources", []string{"one", "two", "three"}, []string{flagkit.Builds + "=/path/to/file", flagkit.ReleaseBundles + "=/path/to/file"}, true},
		{"noSources", []string{"one", "two", "three"}, []string{}, true},
		{"builds without signing key", []string{"name", "version"}, []string{flagkit.Builds + "=/path/to/file"}, false},
		{"builds correct", []string{"name", "version"}, []string{
			flagkit.Builds + "=/path/to/file", flagkit.SigningKey + "=key"}, false},
		{"releaseBundles without signing key", []string{"name", "version"}, []string{flagkit.ReleaseBundles + "=/path/to/file"}, false},
		{"releaseBundles correct", []string{"name", "version"}, []string{
			flagkit.ReleaseBundles + "=/path/to/file", flagkit.SigningKey + "=key"}, false},
		{"spec without signing key", []string{"name", "version", "env"}, []string{"spec=/path/to/file"}, true},
		{"spec correct", []string{"name", "version"}, []string{
			"spec=/path/to/file", flagkit.SigningKey + "=key"}, false},
		// Draft flag tests - draft flag should not affect validation
		{"spec with draft flag", []string{"name", "version"}, []string{
			"spec=/path/to/file", flagkit.SigningKey + "=key"}, false},
		{"builds with draft flag", []string{"name", "version"}, []string{
			flagkit.Builds + "=/path/to/file", flagkit.SigningKey + "=key"}, false},
	}

	for _, test := range testRuns {
		t.Run(test.name, func(t *testing.T) {
			context, buffer := CreateContext(t, test.flags, test.args, nil)
			err := validateCreateReleaseBundleContext(context)
			if test.expectError {
				assert.Error(t, err, buffer)
			} else {
				assert.NoError(t, err, buffer)
			}
		})
	}
}

// Validates that the project option does not override the project field in the spec file.
func TestCreateReleaseBundleSpecWithProject(t *testing.T) {
	projectKey := "myproj"
	specFile := filepath.Join("testdata", "specfile.json")
	context, _ := CreateContext(t, []string{"spec=" + specFile, "project=" + projectKey}, []string{}, nil)
	creationSpec, err := getReleaseBundleCreationSpec(context)
	assert.NoError(t, err)
	assert.Equal(t, creationSpec.Get(0).Pattern, "path/to/file")
	creationSpec.Get(0).Project = ""
	assert.Equal(t, projectKey, pluginsCommon.GetProject(context))
}

func TestGetReleaseBundleCreationSpec(t *testing.T) {

	t.Run("Spec Flag Set", func(t *testing.T) {
		specFile := filepath.Join("testdata", "specfile.json")
		ctx, _ := CreateContext(t, []string{"spec=" + specFile}, []string{}, nil)

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.NoError(t, err)
		assert.NotNil(t, spec)
	})

	t.Run("Build Name and Number Set via Flags", func(t *testing.T) {
		ctx, _ := CreateContext(t, []string{"build-name=Common-builds", "build-number=1.0.0"}, []string{}, nil)

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.NoError(t, err)
		assert.NotNil(t, spec)
		assert.Equal(t, "Common-builds/1.0.0", spec.Files[0].Build)
	})

	t.Run("Build Name and Number Set via Env Variables", func(t *testing.T) {

		setEnvBuildNameCallBack := clientTestUtils.SetEnvWithCallbackAndAssert(t, coreutils.BuildName, "Common-builds")
		defer setEnvBuildNameCallBack()
		setEnvBuildNumberCallBack := clientTestUtils.SetEnvWithCallbackAndAssert(t, coreutils.BuildNumber, "2.0.0")
		defer setEnvBuildNumberCallBack()

		ctx, _ := CreateContext(t, []string{}, []string{}, nil)

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.NoError(t, err)
		assert.NotNil(t, spec)
		assert.Equal(t, "Common-builds/2.0.0", spec.Files[0].Build)
	})

	t.Run("Missing Build Name and Number", func(t *testing.T) {
		ctx, _ := CreateContext(t, []string{}, []string{}, nil)

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.Error(t, err)
		assert.Nil(t, spec)
		assert.EqualError(t, err, "either the --spec flag must be provided, or both --build-name and --build-number flags (or their corresponding environment variables JFROG_CLI_BUILD_NAME and JFROG_CLI_BUILD_NUMBER) must be set")
	})

	t.Run("Only One Build Variable Set", func(t *testing.T) {
		ctx, _ := CreateContext(t, []string{"build-name=Common-builds"}, []string{}, nil)

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.Error(t, err)
		assert.Nil(t, spec)
		assert.EqualError(t, err, "either the --spec flag must be provided, or both --build-name and --build-number flags (or their corresponding environment variables JFROG_CLI_BUILD_NAME and JFROG_CLI_BUILD_NUMBER) must be set")
	})

	t.Run("One Env Variable One Flag", func(t *testing.T) {
		ctx, _ := CreateContext(t, []string{"build-name=Common-builds"}, []string{}, nil)
		setEnvBuildNumberCallBack := clientTestUtils.SetEnvWithCallbackAndAssert(t, coreutils.BuildNumber, "2.0.0")
		defer setEnvBuildNumberCallBack()

		spec, err := getReleaseBundleCreationSpec(ctx)

		assert.NoError(t, err)
		assert.NotNil(t, spec)
		assert.Equal(t, "Common-builds/2.0.0", spec.Files[0].Build)
	})
}

func CreateContext(t *testing.T, testStringFlags, testArgs []string, testBoolFlags map[string]bool) (*components.Context, *bytes.Buffer) {
	ctx := &components.Context{}
	for _, testStringFlag := range testStringFlags {
		stringFlagPair := strings.SplitN(testStringFlag, "=", 2)
		if len(stringFlagPair) < 2 {
			t.Error("Invalid string flag format. Expected format: flag=value")
		}
		ctx.AddStringFlag(stringFlagPair[0], stringFlagPair[1])
	}
	for k, v := range testBoolFlags {
		ctx.AddBoolFlag(k, v)
	}
	ctx.Arguments = testArgs
	ctx.PrintCommandHelp = func(commandName string) error {
		return nil
	}
	return ctx, &bytes.Buffer{}
}

func TestDraftFlagReading(t *testing.T) {
	testRuns := []struct {
		name          string
		boolFlags     map[string]bool
		expectedDraft bool
	}{
		{
			name:          "draft flag not set",
			boolFlags:     nil,
			expectedDraft: false,
		},
		{
			name:          "draft flag set to true",
			boolFlags:     map[string]bool{flagkit.Draft: true},
			expectedDraft: true,
		},
		{
			name:          "draft flag set to false",
			boolFlags:     map[string]bool{flagkit.Draft: false},
			expectedDraft: false,
		},
	}

	for _, test := range testRuns {
		t.Run(test.name, func(t *testing.T) {
			context, _ := CreateContext(t, []string{}, []string{}, test.boolFlags)
			draftValue := context.GetBoolFlagValue(flagkit.Draft)
			assert.Equal(t, test.expectedDraft, draftValue)
		})
	}
}

func TestValidateUpdateReleaseBundleContext(t *testing.T) {
	testRuns := []struct {
		name        string
		args        []string
		flags       []string
		boolFlags   map[string]bool
		expectError bool
	}{
		// Argument validation tests
		{
			name:        "no arguments - should fail",
			args:        []string{},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: true,
		},
		{
			name:        "one argument - should fail",
			args:        []string{"bundle-name"},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: true,
		},
		{
			name:        "three arguments - should fail",
			args:        []string{"bundle-name", "1.0.0", "extra"},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: true,
		},
		// Operation flag validation tests
		{
			name:        "missing add flag - should fail",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   nil,
			expectError: true,
		},
		{
			name:        "add flag set to false - should fail",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   map[string]bool{flagkit.AddSources: false},
			expectError: true,
		},
		// Source validation tests
		{
			name:        "no sources provided - should fail",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: true,
		},
		{
			name:        "spec file provided with add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{"spec=/path/to/file"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
		{
			name:        "source-type-builds provided with add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SourceTypeBuilds + "=name=build1,id=123"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
		{
			name:        "source-type-release-bundles provided with add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SourceTypeReleaseBundles + "=name=rb1,version=1.0"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
		{
			name:        "both source-type flags provided with add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SourceTypeBuilds + "=name=build1,id=123", flagkit.SourceTypeReleaseBundles + "=name=rb1,version=1.0"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
		// Project flag tests
		{
			name:        "spec with project flag and add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{"spec=/path/to/file", "project=my-project"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
		{
			name:        "source-type-builds with project flag and add flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SourceTypeBuilds + "=name=build1,id=123", "project=my-project"},
			boolFlags:   map[string]bool{flagkit.AddSources: true},
			expectError: false,
		},
	}

	for _, test := range testRuns {
		t.Run(test.name, func(t *testing.T) {
			context, buffer := CreateContext(t, test.flags, test.args, test.boolFlags)
			err := validateUpdateReleaseBundleContext(context)
			if test.expectError {
				assert.Error(t, err, buffer)
			} else {
				assert.NoError(t, err, buffer)
			}
		})
	}
}

func TestSyncFlagReading(t *testing.T) {
	testRuns := []struct {
		name         string
		boolFlags    map[string]bool
		expectedSync bool
	}{
		{
			// Default value when not set - note: in real CLI the default is true, but in test context it's false
			name:         "sync flag not set",
			boolFlags:    nil,
			expectedSync: false,
		},
		{
			name:         "sync flag set to true",
			boolFlags:    map[string]bool{flagkit.Sync: true},
			expectedSync: true,
		},
		{
			name:         "sync flag set to false",
			boolFlags:    map[string]bool{flagkit.Sync: false},
			expectedSync: false,
		},
	}

	for _, test := range testRuns {
		t.Run(test.name, func(t *testing.T) {
			context, _ := CreateContext(t, []string{}, []string{}, test.boolFlags)
			syncValue := context.GetBoolFlagValue(flagkit.Sync)
			assert.Equal(t, test.expectedSync, syncValue)
		})
	}
}

func TestValidateFinalizeReleaseBundleContext(t *testing.T) {
	testRuns := []struct {
		name        string
		args        []string
		flags       []string
		boolFlags   map[string]bool
		expectError bool
	}{
		// Argument validation tests
		{
			name:        "no arguments - should fail",
			args:        []string{},
			flags:       []string{},
			boolFlags:   nil,
			expectError: true,
		},
		{
			name:        "one argument - should fail",
			args:        []string{"bundle-name"},
			flags:       []string{},
			boolFlags:   nil,
			expectError: true,
		},
		{
			name:        "three arguments - should fail",
			args:        []string{"bundle-name", "1.0.0", "extra"},
			flags:       []string{},
			boolFlags:   nil,
			expectError: true,
		},
		{
			name:        "two arguments - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{},
			boolFlags:   nil,
			expectError: false,
		},
		// With optional flags
		{
			name:        "with signing key - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SigningKey + "=my-key"},
			boolFlags:   nil,
			expectError: false,
		},
		{
			name:        "with project flag - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{"project=my-project"},
			boolFlags:   nil,
			expectError: false,
		},
		{
			name:        "with sync flag true - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{},
			boolFlags:   map[string]bool{flagkit.Sync: true},
			expectError: false,
		},
		{
			name:        "with sync flag false - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{},
			boolFlags:   map[string]bool{flagkit.Sync: false},
			expectError: false,
		},
		{
			name:        "with all optional flags - should pass",
			args:        []string{"bundle-name", "1.0.0"},
			flags:       []string{flagkit.SigningKey + "=my-key", "project=my-project"},
			boolFlags:   map[string]bool{flagkit.Sync: true},
			expectError: false,
		},
	}

	for _, test := range testRuns {
		t.Run(test.name, func(t *testing.T) {
			context, buffer := CreateContext(t, test.flags, test.args, test.boolFlags)
			err := validateFinalizeReleaseBundleContext(context)
			if test.expectError {
				assert.Error(t, err, buffer)
			} else {
				assert.NoError(t, err, buffer)
			}
		})
	}
}
