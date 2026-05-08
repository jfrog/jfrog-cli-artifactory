package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedAgentsList_SortedAndStable(t *testing.T) {
	first := SupportedAgentsList()
	for range 20 {
		assert.Equal(t, first, SupportedAgentsList())
	}
	parts := strings.Split(first, ", ")
	for i := 1; i < len(parts); i++ {
		assert.LessOrEqual(t, parts[i-1], parts[i])
	}
}
