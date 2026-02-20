package generic

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/stretchr/testify/assert"
)

func TestNewDeleteCommand(t *testing.T) {
	dc := NewDeleteCommand()
	assert.NotNil(t, dc)
	assert.Equal(t, "rt_delete", dc.CommandName())
	assert.NotNil(t, dc.Result())
}

func TestDeleteCommand_SetThreads(t *testing.T) {
	dc := NewDeleteCommand()
	dc.SetThreads(5)
	assert.Equal(t, 5, dc.Threads())
}

func TestDeleteCommand_Threads(t *testing.T) {
	dc := NewDeleteCommand()

	// Default threads should be 0
	assert.Equal(t, 0, dc.Threads())

	// Set and verify threads
	dc.SetThreads(10)
	assert.Equal(t, 10, dc.Threads())

	// Chaining should work
	result := dc.SetThreads(3)
	assert.Equal(t, dc, result)
	assert.Equal(t, 3, dc.Threads())
}

func TestGetDeleteParams(t *testing.T) {
	tests := []struct {
		name              string
		file              *spec.File
		expectedRecursive bool
	}{
		{
			name: "Basic pattern with default recursive",
			file: &spec.File{
				Pattern: "repo/path/*",
			},
			expectedRecursive: true,
		},
		{
			name: "Pattern with recursive true",
			file: &spec.File{
				Pattern:   "repo/path/*",
				Recursive: "true",
			},
			expectedRecursive: true,
		},
		{
			name: "Pattern with recursive false",
			file: &spec.File{
				Pattern:   "repo/path/*",
				Recursive: "false",
			},
			expectedRecursive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := getDeleteParams(tt.file)
			assert.NoError(t, err)
			assert.Equal(t, tt.file.Pattern, params.Pattern)
			assert.Equal(t, tt.expectedRecursive, params.Recursive)
		})
	}
}

func TestGetDeleteParams_ExcludeArtifacts(t *testing.T) {
	tests := []struct {
		name                     string
		excludeArtifacts         string
		expectedExcludeArtifacts bool
	}{
		{
			name:                     "Default exclude artifacts (false)",
			excludeArtifacts:         "",
			expectedExcludeArtifacts: false,
		},
		{
			name:                     "Exclude artifacts true",
			excludeArtifacts:         "true",
			expectedExcludeArtifacts: true,
		},
		{
			name:                     "Exclude artifacts false",
			excludeArtifacts:         "false",
			expectedExcludeArtifacts: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &spec.File{
				Pattern:          "repo/*",
				ExcludeArtifacts: tt.excludeArtifacts,
			}
			params, err := getDeleteParams(file)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedExcludeArtifacts, params.ExcludeArtifacts)
		})
	}
}

func TestGetDeleteParams_IncludeDeps(t *testing.T) {
	tests := []struct {
		name                string
		includeDeps         string
		expectedIncludeDeps bool
	}{
		{
			name:                "Default include deps (false)",
			includeDeps:         "",
			expectedIncludeDeps: false,
		},
		{
			name:                "Include deps true",
			includeDeps:         "true",
			expectedIncludeDeps: true,
		},
		{
			name:                "Include deps false",
			includeDeps:         "false",
			expectedIncludeDeps: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &spec.File{
				Pattern:     "repo/*",
				IncludeDeps: tt.includeDeps,
			}
			params, err := getDeleteParams(file)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedIncludeDeps, params.IncludeDeps)
		})
	}
}

func TestGetDeleteParams_CombinedFlags(t *testing.T) {
	file := &spec.File{
		Pattern:          "repo/path/**/*.jar",
		Recursive:        "true",
		ExcludeArtifacts: "true",
		IncludeDeps:      "true",
		Exclusions:       []string{"*-sources.jar", "*-javadoc.jar"},
	}

	params, err := getDeleteParams(file)
	assert.NoError(t, err)

	assert.Equal(t, "repo/path/**/*.jar", params.Pattern)
	assert.True(t, params.Recursive)
	assert.True(t, params.ExcludeArtifacts)
	assert.True(t, params.IncludeDeps)
	assert.Len(t, params.Exclusions, 2)
}

func TestGetDeleteParams_WithExclusions(t *testing.T) {
	file := &spec.File{
		Pattern: "repo/*.bin",
		Exclusions: []string{
			"*-test.bin",
			"*-debug.bin",
			"temp/*",
		},
	}

	params, err := getDeleteParams(file)

	assert.NoError(t, err)
	assert.Len(t, params.Exclusions, 3)
	assert.Contains(t, params.Exclusions, "*-test.bin")
	assert.Contains(t, params.Exclusions, "*-debug.bin")
	assert.Contains(t, params.Exclusions, "temp/*")
}

func TestGetDeleteParams_EmptyPattern(t *testing.T) {
	file := &spec.File{
		Pattern: "",
	}

	params, err := getDeleteParams(file)

	assert.NoError(t, err)
	assert.Equal(t, "", params.Pattern)
}

func TestDeleteCommand_ResultInitialized(t *testing.T) {
	dc := NewDeleteCommand()

	// Result should be initialized (not nil)
	result := dc.Result()
	assert.NotNil(t, result)

	// Default counts should be 0
	assert.Equal(t, 0, result.SuccessCount())
	assert.Equal(t, 0, result.FailCount())
}

func TestDeleteCommand_GenericCommandEmbedded(t *testing.T) {
	dc := NewDeleteCommand()

	// Test GenericCommand methods are accessible
	dc.SetDryRun(true)
	assert.True(t, dc.DryRun())

	dc.SetQuiet(true)
	assert.True(t, dc.Quiet())

	dc.SetRetries(3)
	assert.Equal(t, 3, dc.Retries())
}

// TestErrorsJoin verifies that errors.Join works correctly for combining errors
// This tests the behavior used in DeleteFiles when both delete and length errors occur
func TestErrorsJoin(t *testing.T) {
	err1 := errors.New("delete error")
	err2 := errors.New("length error")

	combined := errors.Join(err1, err2)
	assert.Error(t, combined)
	assert.Contains(t, combined.Error(), "delete error")
	assert.Contains(t, combined.Error(), "length error")

	// Test with nil errors
	combinedWithNil := errors.Join(nil, err2)
	assert.Error(t, combinedWithNil)
	assert.Contains(t, combinedWithNil.Error(), "length error")

	combinedBothNil := errors.Join(nil, nil)
	assert.NoError(t, combinedBothNil)
}

// TestDeleteFilesCountBehavior documents the expected behavior of count calculations
// deletedCount represents successfully deleted items
// length - deletedCount represents failed items
func TestDeleteFilesCountBehavior(t *testing.T) {
	tests := []struct {
		name                 string
		deletedCount         int
		totalLength          int
		expectedSuccessCount int
		expectedFailedCount  int
	}{
		{
			name:                 "All items deleted successfully",
			deletedCount:         10,
			totalLength:          10,
			expectedSuccessCount: 10,
			expectedFailedCount:  0,
		},
		{
			name:                 "Some items failed",
			deletedCount:         7,
			totalLength:          10,
			expectedSuccessCount: 7,
			expectedFailedCount:  3,
		},
		{
			name:                 "All items failed",
			deletedCount:         0,
			totalLength:          10,
			expectedSuccessCount: 0,
			expectedFailedCount:  10,
		},
		{
			name:                 "No items to delete",
			deletedCount:         0,
			totalLength:          0,
			expectedSuccessCount: 0,
			expectedFailedCount:  0,
		},
		{
			name:                 "Partial success (404 scenario)",
			deletedCount:         5,
			totalLength:          8,
			expectedSuccessCount: 5,
			expectedFailedCount:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the count calculation in DeleteFiles
			successCount := tt.deletedCount
			failedCount := tt.totalLength - tt.deletedCount

			assert.Equal(t, tt.expectedSuccessCount, successCount)
			assert.Equal(t, tt.expectedFailedCount, failedCount)
		})
	}
}
