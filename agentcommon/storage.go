package agentcommon

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

// jfrogClientResponseErrPrefix matches errorutils.GenerateResponseError in jfrog-client-go.
const jfrogClientResponseErrPrefix = "server response: "

// PackageVersionExists reports whether {repoKey}/{slug}/{version}/ exists in Artifactory
// using the generic storage API. A 404 on the path is reported as "does not exist".
func PackageVersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	_, err = serviceManager.FolderInfo(fmt.Sprintf("%s/%s/%s", repoKey, slug, version))
	if err == nil {
		return true, nil
	}
	if isNotFoundErr(err) {
		return false, nil
	}
	return false, err
}

func isNotFoundErr(err error) bool {
	statusCode, ok := jfrogClientResponseStatusCode(err)
	return ok && statusCode == http.StatusNotFound
}

// jfrogClientResponseStatusCode extracts the HTTP status code from errors returned by
// jfrog-client-go HTTP helpers (via errorutils.GenerateResponseError).
func jfrogClientResponseStatusCode(err error) (int, bool) {
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
	if convErr != nil {
		return 0, false
	}
	return statusCode, true
}
