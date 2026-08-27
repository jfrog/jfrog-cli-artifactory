package aieditorextensions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	// aiEditorExtensionTokenAPI is the Artifactory REST path that issues a
	// per-user reference token used by curated AI editor extension downloads.
	// #nosec G101 -- REST endpoint path, not a credential.
	aiEditorExtensionTokenAPI = "api/setMeUp/AIEditorExtensionGenerateToken"

	tokenHTTPRetries            = 3
	tokenHTTPRetryWaitMilliSecs = 1000
)

// generateTokenResponse mirrors the JSON returned by AIEditorExtensionGenerateToken.
type generateTokenResponse struct {
	ReferenceToken string `json:"referenceToken"`
	RepoKey        string `json:"repoKey"`
}

// FetchReferenceToken calls GET api/setMeUp/AIEditorExtensionGenerateToken?repoKey=<repoKey>
// against the configured Artifactory and returns the per-user referenceToken.
// The token is what makes curated extension downloads attributable to the
// individual user rather than an anonymous gallery consumer.
func FetchReferenceToken(serverDetails *config.ServerDetails, repoKey string) (string, error) {
	if serverDetails == nil {
		return "", fmt.Errorf("server details are required to obtain a reference token")
	}
	if strings.TrimSpace(repoKey) == "" {
		return "", fmt.Errorf("repository key is required to obtain a reference token")
	}

	serviceManager, err := utils.CreateServiceManager(serverDetails, tokenHTTPRetries, tokenHTTPRetryWaitMilliSecs, false)
	if err != nil {
		return "", fmt.Errorf("failed to create service manager: %w", err)
	}

	artURL := clientutils.AddTrailingSlashIfNeeded(serviceManager.GetConfig().GetServiceDetails().GetUrl())
	requestURL := fmt.Sprintf("%s%s?repoKey=%s", artURL, aiEditorExtensionTokenAPI, url.QueryEscape(repoKey))
	log.Debug("Fetching AI editor extension reference token:", requestURL)

	httpDetails := serviceManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	resp, body, _, err := serviceManager.Client().SendGet(requestURL, true, &httpDetails)
	if err != nil {
		return "", fmt.Errorf("failed to call reference token endpoint: %w", err)
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		return "", err
	}

	var parsed generateTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse reference token response: %w", err)
	}
	if strings.TrimSpace(parsed.ReferenceToken) == "" {
		return "", fmt.Errorf("reference token endpoint returned an empty referenceToken for repository '%s'", repoKey)
	}
	return parsed.ReferenceToken, nil
}
