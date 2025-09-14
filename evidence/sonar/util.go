package sonar

import (
	"net/url"

	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const defaultSonarURL = "https://sonarcloud.io"

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	exists, err := fileutils.IsFileExists(path, false)
	if err != nil {
		log.Debug("Error while checking if file exists", path, err)
	}
	return err == nil && exists
}

func resolveSonarBaseURL(ceTaskURL, serverURL string) string {
	if serverURL != "" {
		return serverURL
	}
	if ceTaskURL != "" {
		u, err := url.Parse(ceTaskURL)
		if err == nil {
			return u.Scheme + "://" + u.Host
		}
	}
	return defaultSonarURL
}
