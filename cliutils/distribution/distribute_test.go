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
		{name: "Low", flags: map[string]string{"priority": "Low"}, expectError: false},
		{name: "HIGH", flags: map[string]string{"priority": "HIGH"}, expectError: false},
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

func TestGetPriorityFlagValuePreservesCase(t *testing.T) {
	assert.Equal(t, "", GetPriorityFlagValue(newPriorityTestContext(t, nil, nil)))
	assert.Equal(t, "high", GetPriorityFlagValue(newPriorityTestContext(t, nil, map[string]string{"priority": "high"})))
	assert.Equal(t, "HIGH", GetPriorityFlagValue(newPriorityTestContext(t, nil, map[string]string{"priority": "HIGH"})))
}

func TestValidateReleaseBundleDistributeCmdPriority(t *testing.T) {
	t.Run("accepted any case", func(t *testing.T) {
		ctx := newPriorityTestContext(t, []string{"bundle", "1.0.0"}, map[string]string{"priority": "High", "site": "edge1"})
		assert.NoError(t, ValidateReleaseBundleDistributeCmd(ctx))
	})
	t.Run("rejected unknown value", func(t *testing.T) {
		ctx := newPriorityTestContext(t, []string{"bundle", "1.0.0"}, map[string]string{"priority": "bogus", "site": "edge1"})
		require.Error(t, ValidateReleaseBundleDistributeCmd(ctx))
	})
}
