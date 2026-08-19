package distribution

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPriorityTestContext(t *testing.T, args []string, flags map[string]string) *components.Context {
	t.Helper()
	ctx := &components.Context{Arguments: args}
	for k, v := range flags {
		ctx.AddStringFlag(k, v)
	}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}

func TestValidatePriorityFlag(t *testing.T) {
	tests := []struct {
		name        string
		flags       map[string]string
		expectError bool
	}{
		{name: "omitted", flags: nil, expectError: false},
		{name: "low", flags: map[string]string{"priority": "low"}, expectError: false},
		{name: "medium", flags: map[string]string{"priority": "medium"}, expectError: false},
		{name: "high", flags: map[string]string{"priority": "high"}, expectError: false},
		{name: "mixed case", flags: map[string]string{"priority": "HiGh"}, expectError: false},
		{name: "trimmed", flags: map[string]string{"priority": "  medium  "}, expectError: false},
		{name: "invalid", flags: map[string]string{"priority": "urgent"}, expectError: true},
		{name: "empty string flag", flags: map[string]string{"priority": ""}, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newPriorityTestContext(t, []string{"bundle", "1.0.0"}, tt.flags)
			err := ValidatePriorityFlag(ctx)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestGetPriorityFlagValue(t *testing.T) {
	assert.Equal(t, "", GetPriorityFlagValue(newPriorityTestContext(t, nil, nil)))
	assert.Equal(t, "high", GetPriorityFlagValue(newPriorityTestContext(t, nil, map[string]string{"priority": "HIGH"})))
	assert.Equal(t, "medium", GetPriorityFlagValue(newPriorityTestContext(t, nil, map[string]string{"priority": "  Medium "})))
}

func TestValidateReleaseBundleDistributeCmdPriority(t *testing.T) {
	t.Run("valid priority", func(t *testing.T) {
		ctx := newPriorityTestContext(t, []string{"bundle", "1.0.0"}, map[string]string{"priority": "high", "site": "edge1"})
		assert.NoError(t, ValidateReleaseBundleDistributeCmd(ctx))
	})
	t.Run("invalid priority", func(t *testing.T) {
		ctx := newPriorityTestContext(t, []string{"bundle", "1.0.0"}, map[string]string{"priority": "bogus", "site": "edge1"})
		require.Error(t, ValidateReleaseBundleDistributeCmd(ctx))
	})
}
