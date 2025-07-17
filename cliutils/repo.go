package cliutils

import (
	"fmt"
	"strings"
)

// ExtractRepoNameFromURL extracts the repository name (key) from a given URL or path string.
// It trims protocol and trailing slashes, then returns the last segment after splitting by '/'.
// Returns an error if the input is empty or not valid.
func ExtractRepoNameFromURL(configUrl string) (string, error) {
	url := strings.TrimSpace(configUrl)
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimSuffix(url, "/")
	if url == "" {
		return "", fmt.Errorf("config URL is empty")
	}
	urlParts := strings.Split(url, "/")
	if len(urlParts) < 2 {
		return "", fmt.Errorf("config URL is not valid")
	}
	return urlParts[len(urlParts)-1], nil
}
