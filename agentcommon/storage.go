package agentcommon

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

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
	msg := err.Error()
	return strings.Contains(msg, "404") || strings.Contains(msg, "Not Found")
}
