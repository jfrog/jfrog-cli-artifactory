package common

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type SkillVersion struct {
	Version   string `json:"version"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	Changelog string `json:"changelog,omitempty"`
}

type SkillSearchResult struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

// API response wrappers matching Artifactory skills API format:
//   versions: { "items": [...] }
//   search:   { "skills": [...] }
type versionsResponse struct {
	Versions []SkillVersion `json:"items"`
}

type searchResponse struct {
	Skills []SkillSearchResult `json:"skills"`
}

// ListVersions returns available versions for a skill from the skills API.
func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]SkillVersion, error) {
	url := buildSkillsAPIURL(serverDetails, repoKey, fmt.Sprintf("skills/%s/versions", slug))
	body, err := doSkillsAPIGet(serverDetails, url)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for skill '%s': %w", slug, err)
	}

	var wrapper versionsResponse
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse versions response: %w", err)
	}
	return wrapper.Versions, nil
}

// SearchSkills searches for skills matching a query string.
func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]SkillSearchResult, error) {
	url := buildSkillsAPIURL(serverDetails, repoKey, fmt.Sprintf("search?q=%s&limit=%d", query, limit))
	body, err := doSkillsAPIGet(serverDetails, url)
	if err != nil {
		return nil, fmt.Errorf("failed to search skills: %w", err)
	}

	var wrapper searchResponse
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return wrapper.Skills, nil
}

// VersionExists checks if a specific version of a skill exists.
func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	versions, err := ListVersions(serverDetails, repoKey, slug)
	if err != nil {
		return false, err
	}
	for _, v := range versions {
		if v.Version == version {
			return true, nil
		}
	}
	return false, nil
}

func buildSkillsAPIURL(serverDetails *config.ServerDetails, repoKey, path string) string {
	baseURL := strings.TrimRight(serverDetails.ArtifactoryUrl, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(serverDetails.Url, "/")
	}
	return fmt.Sprintf("%s/api/skills/%s/api/v1/%s", baseURL, repoKey, path)
}

func doSkillsAPIGet(serverDetails *config.ServerDetails, url string) ([]byte, error) {
	log.Debug("Skills API request:", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	artAuth, err := serverDetails.CreateArtAuthConfig()
	if err != nil {
		return nil, err
	}
	addAuthHeaders(req, artAuth)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skills API returned status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func addAuthHeaders(req *http.Request, artAuth auth.ServiceDetails) {
	if artAuth.GetAccessToken() != "" {
		req.Header.Set("Authorization", "Bearer "+artAuth.GetAccessToken())
		return
	}
	if artAuth.GetUser() != "" && artAuth.GetPassword() != "" {
		req.SetBasicAuth(artAuth.GetUser(), artAuth.GetPassword())
	}
}
