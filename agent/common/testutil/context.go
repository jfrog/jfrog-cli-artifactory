package testutil

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// NewCLIContext returns a components.Context with a no-op PrintCommandHelp handler.
func NewCLIContext(args ...string) *components.Context {
	ctx := &components.Context{Arguments: args}
	ctx.PrintCommandHelp = func(string) error { return nil }
	return ctx
}
