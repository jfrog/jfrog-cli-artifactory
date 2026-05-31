package common

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
)

// DeleteVersion deletes the entire version directory for a package (plugin, skill, etc.)
// at {artURL}{repoKey}/{slug}/{version}/ using an HTTP DELETE request.
func DeleteVersion(serverDetails *config.ServerDetails, repoKey, slug, version string) error {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create service manager for deletion: %w", err)
	}
	artURL := clientutils.AddTrailingSlashIfNeeded(sm.GetConfig().GetServiceDetails().GetUrl())
	deletePath := fmt.Sprintf("%s%s/%s/%s/", artURL, repoKey, slug, version)
	httpDetails := sm.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	resp, body, err := sm.Client().SendDelete(deletePath, nil, &httpDetails)
	if err != nil {
		return fmt.Errorf("failed to delete %s/%s/%s: %w", repoKey, slug, version, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to delete %s/%s/%s: HTTP %d — %s", repoKey, slug, version, resp.StatusCode, string(body))
	}
	return nil
}

// ErrVersionExistenceUnknown indicates the version path could not be checked (e.g. network
// error or an Artifactory response whose HTTP status could not be read). Callers should not
// treat this as "version does not exist".
var ErrVersionExistenceUnknown = errors.New("package version existence could not be determined")

// jfrogClientResponseErrPrefix is the prefix used by errorutils.GenerateResponseError in jfrog-client-go.
// There is no exported *ResponseError type in jfrog-client-go today; this prefix is the stable fallback.
const jfrogClientResponseErrPrefix = "server response: "

// httpStatusCoder may be implemented by jfrog-client-go response errors when exposed.
type httpStatusCoder interface {
	StatusCode() int
}

// PackageVersionExists reports whether {repoKey}/{slug}/{version}/ exists in Artifactory
// using the generic storage API. A 404 on the path is reported as "does not exist".
// When the HTTP status cannot be determined, returns ErrVersionExistenceUnknown.
func PackageVersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	_, err = serviceManager.FolderInfo(fmt.Sprintf("%s/%s/%s", repoKey, slug, version))
	if err == nil {
		return true, nil
	}
	if statusCode, hasStatusCode := jfrogClientHTTPStatusCode(err); hasStatusCode {
		if statusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return false, fmt.Errorf("%w: %w", ErrVersionExistenceUnknown, err)
}

// jfrogClientHTTPStatusCode extracts an HTTP status from jfrog-client-go errors by walking the chain.
// It prefers errors.As against types that implement StatusCode(). When jfrog-client-go does not expose
// a typed response error, it falls back to parsing messages from errorutils.GenerateResponseError only.
// Remove statusFromJFrogGenerateResponseError once jfrog-client-go exports a response error with StatusCode().
// If neither applies, ok is false and callers must not infer 404 from the error text.
func jfrogClientHTTPStatusCode(err error) (int, bool) {
	for chainedErr := err; chainedErr != nil; chainedErr = errors.Unwrap(chainedErr) {
		var statusCoder httpStatusCoder
		if errors.As(chainedErr, &statusCoder) {
			if code := statusCoder.StatusCode(); isValidHTTPStatusCode(code) {
				return code, true
			}
		}
		if code, hasStatusCode := statusFromJFrogGenerateResponseError(chainedErr); hasStatusCode {
			return code, true
		}
	}
	return 0, false
}

// statusFromJFrogGenerateResponseError parses errors produced by errorutils.GenerateResponseError.
// Accepted example: "server response: 404 Not Found"
// Accepted with body: "server response: 404 Not Found\n{\"errors\":[]}"
// Not accepted: "failed to access repo 404-something", "connection refused"
func statusFromJFrogGenerateResponseError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, jfrogClientResponseErrPrefix) {
		return 0, false
	}
	statusLine := strings.TrimSpace(msg[len(jfrogClientResponseErrPrefix):])
	if newline := strings.IndexByte(statusLine, '\n'); newline >= 0 {
		statusLine = statusLine[:newline]
	}
	codeStr, _, found := strings.Cut(statusLine, " ")
	if !found {
		codeStr = statusLine
	}
	statusCode, convErr := strconv.Atoi(codeStr)
	if convErr != nil || !isValidHTTPStatusCode(statusCode) {
		return 0, false
	}
	return statusCode, true
}

func isValidHTTPStatusCode(code int) bool {
	return code >= 100 && code <= 599
}
