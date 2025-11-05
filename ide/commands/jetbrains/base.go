package jetbrains

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const (
	RepoKeyFlag   = "repo-key"
	URLSuffixFlag = "url-suffix"
	ApiType       = "jetbrainsplugins"
)

// BaseSetupConfig contains common configuration for JetBrains setup
type BaseSetupConfig struct {
	RepoKey       string
	RepositoryURL string
	URLSuffix     string
	ServerDetails *config.ServerDetails
	IsDirectURL   bool
}

// ParseBaseSetupConfig extracts common setup configuration from the context
func ParseBaseSetupConfig(c *components.Context) (*BaseSetupConfig, error) {
	cfg := &BaseSetupConfig{}

	// Check for direct URL first (argument position 1, position 0 is IDE name)
	if c.GetNumberOfArgs() > 1 && common.IsValidUrl(c.GetArgumentAt(1)) {
		cfg.RepositoryURL = c.GetArgumentAt(1)
		cfg.RepoKey = common.ExtractRepoKeyFromURL(cfg.RepositoryURL, ApiType)
		cfg.IsDirectURL = true
		return cfg, nil
	}

	// Parse flags
	cfg.RepoKey = c.GetStringFlagValue(RepoKeyFlag)
	cfg.URLSuffix = c.GetStringFlagValue(URLSuffixFlag)

	if cfg.RepoKey == "" {
		return nil, errors.New("--repo-key flag is required. Please specify the repository key for your JetBrains plugins repository")
	}

	// Get server details
	rtDetails, err := common.GetServerDetails(c)
	if err != nil {
		return nil, fmt.Errorf("failed to get server configuration: %w. Please run 'jf config add' first", err)
	}
	cfg.ServerDetails = rtDetails

	// Validate repository
	if err := common.ValidateRepository(cfg.RepoKey, rtDetails, ApiType); err != nil {
		return nil, err
	}

	// Build repository URL
	baseUrl := common.GetBaseUrl(rtDetails)
	cfg.RepositoryURL = common.BuildURL(baseUrl, ApiType, cfg.RepoKey, cfg.URLSuffix)
	cfg.IsDirectURL = false

	return cfg, nil
}
