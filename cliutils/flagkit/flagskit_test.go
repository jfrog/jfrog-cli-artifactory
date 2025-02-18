package flagkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandFlags(t *testing.T) {
	for cmdKey, flags := range commandFlags {
		t.Run(cmdKey, func(t *testing.T) {
			assert.Equal(t, len(flags), len(commandFlags[cmdKey]), "Number of flags mismatch for command: %s", cmdKey)

			for _, flag := range flags {
				_, exists := flagsMap[flag]
				if !exists {
					t.Logf("Flag %s not found in flagsMap for command: %s", flag, cmdKey)
				}
				assert.True(t, exists, "Flag %s not found in flagsMap for command: %s", flag, cmdKey)
			}
		})
	}
}
