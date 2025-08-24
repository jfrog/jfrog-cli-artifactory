package sonar

import (
	"net/url"
	"os"
)

const defaultSonarURL = "https://api.sonarcloud.io"

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
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
