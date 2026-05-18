package agentcommon

import (
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

// EvidenceLicenseErrFragment is the substring in error messages that indicates
// the Artifactory instance lacks the Enterprise+ license required for evidence.
const EvidenceLicenseErrFragment = "Enterprise+"

// WithSuppressedLogs temporarily mutes all log output while fn executes,
// then restores the previous log level.
func WithSuppressedLogs(fn func() error) error {
	if jfLogger, ok := log.GetLogger().(*log.JfrogLogger); ok {
		prev := jfLogger.GetLogLevel()
		jfLogger.SetLogLevel(-1)
		defer jfLogger.SetLogLevel(prev)
	}
	return fn()
}

// IsEvidenceLicenseError returns true when the error indicates the Artifactory
// instance does not have the license required for evidence (E+).
func IsEvidenceLicenseError(err error) bool {
	return strings.Contains(err.Error(), EvidenceLicenseErrFragment)
}
