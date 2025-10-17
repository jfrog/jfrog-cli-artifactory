package strategies

import (
	"os"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// BuildStrategy defines the interface for different build execution strategies
type BuildStrategy interface {
	Execute(cmdParams []string, buildConfig *build.BuildConfiguration) error
}

// BuildStrategyFactory creates the appropriate strategy based on command and environment
type BuildStrategyFactory struct{}

func (f *BuildStrategyFactory) CreateStrategy(cmdParams []string) BuildStrategy {
	// Check if JFROG_RUN_NATIVE is set
	if os.Getenv("JFROG_RUN_NATIVE") == "true" {
		// In native mode, check if this is a buildx command
		if isUsingBuildX(cmdParams) {
			log.Debug("Using BuildX Strategy (JFROG_RUN_NATIVE=true with buildx build)")
			return NewBuildXStrategy()
		}
		// Regular native mode
		log.Debug("Using RunNative Strategy (JFROG_RUN_NATIVE=true)")
		return NewRunNativeStrategy()
	}

	// Default to legacy when JFROG_RUN_NATIVE is not set
	log.Debug("Using Legacy Strategy (traditional JFrog approach)")
	return NewLegacyStrategy()
}

// isUsingBuildX checks if the command parameters contain "buildx build"
func isUsingBuildX(cmdParams []string) bool {
	// Convert command params to a single string and check for "buildx build"
	cmdString := strings.Join(cmdParams, " ")
	return strings.Contains(cmdString, "buildx build")
}
