package common

import (
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

// EvidenceLicenseErrFragment is the substring in error messages that indicates
// the Artifactory instance lacks the Enterprise+ license required for evidence.
const EvidenceLicenseErrFragment = "Enterprise+"

// logLevelSilent disables all JfrogLogger output (below the minimum log level).
const logLevelSilent = -1

// WithSuppressedLogs temporarily mutes all log output while fn executes,
// then restores the previous log level.
func WithSuppressedLogs(fn func() error) error {
	if jfrogLogger, isJfrogLogger := log.GetLogger().(*log.JfrogLogger); isJfrogLogger {
		previousLogLevel := jfrogLogger.GetLogLevel()
		jfrogLogger.SetLogLevel(logLevelSilent)
		defer jfrogLogger.SetLogLevel(previousLogLevel)
	}
	return fn()
}

// IsEvidenceLicenseError returns true when the error indicates the Artifactory
// instance does not have the license required for evidence (E+).
func IsEvidenceLicenseError(err error) bool {
	return strings.Contains(err.Error(), EvidenceLicenseErrFragment)
}
