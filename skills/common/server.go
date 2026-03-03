package common

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type RepositoryInfo struct {
	Key         string `json:"key"`
	PackageType string `json:"packageType"`
	Type        string `json:"type"`
}

func GetServerDetails(c *components.Context) (*config.ServerDetails, error) {
	if hasServerConfigFlags(c) {
		return pluginsCommon.CreateArtifactoryDetailsByFlags(c)
	}
	rtDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("no default server configured. Use 'jf config add' or provide --url and --access-token flags")
	}
	if rtDetails.ArtifactoryUrl == "" && rtDetails.Url == "" {
		return nil, fmt.Errorf("no Artifactory URL configured")
	}
	return rtDetails, nil
}

func hasServerConfigFlags(c *components.Context) bool {
	return c.IsFlagSet("url") ||
		c.IsFlagSet("user") ||
		c.IsFlagSet("access-token") ||
		c.IsFlagSet("server-id") ||
		(c.IsFlagSet("password") && (c.IsFlagSet("url") || c.IsFlagSet("server-id")))
}

// ResolveRepo resolves the skills repository key from: --repo flag > RT_REPO_KEY env > auto-discovery.
func ResolveRepo(c *components.Context, serverDetails *config.ServerDetails) (string, error) {
	if c.IsFlagSet("repo") {
		repoKey := c.GetStringFlagValue("repo")
		if repoKey != "" {
			return repoKey, nil
		}
	}
	if envRepo := os.Getenv("RT_REPO_KEY"); envRepo != "" {
		return envRepo, nil
	}
	return discoverSkillsRepo(serverDetails)
}

func discoverSkillsRepo(serverDetails *config.ServerDetails) (string, error) {
	baseURL := strings.TrimRight(serverDetails.ArtifactoryUrl, "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(serverDetails.Url, "/")
	}
	url := baseURL + "/api/repositories?type=local&packageType=skills"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	artAuth, err := serverDetails.CreateArtAuthConfig()
	if err != nil {
		return "", fmt.Errorf("failed to create auth config: %w", err)
	}
	if artAuth.GetAccessToken() != "" {
		req.Header.Set("Authorization", "Bearer "+artAuth.GetAccessToken())
	} else if artAuth.GetUser() != "" && artAuth.GetPassword() != "" {
		req.SetBasicAuth(artAuth.GetUser(), artAuth.GetPassword())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query repositories: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list repositories (status %d): %s", resp.StatusCode, string(body))
	}

	var repos []RepositoryInfo
	if err := json.Unmarshal(body, &repos); err != nil {
		return "", fmt.Errorf("failed to parse repository list: %w", err)
	}

	if len(repos) == 0 {
		return "", fmt.Errorf("no local skills repositories found. Create one in Artifactory or specify --repo")
	}
	if len(repos) == 1 {
		log.Info("Using skills repository:", repos[0].Key)
		return repos[0].Key, nil
	}

	repoNames := make([]string, len(repos))
	for i, r := range repos {
		repoNames[i] = r.Key
	}

	log.Info("Multiple skills repositories found:", repoNames)
	return repos[0].Key, nil
}
