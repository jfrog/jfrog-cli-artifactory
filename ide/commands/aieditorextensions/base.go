package aieditorextensions

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

const (
	RepoKeyFlag      = "repo-key"
	URLSuffixFlag    = "url-suffix"
	ProductJsonPath  = "product-json-path"
	ApiType          = "aieditorextensions"
	DefaultURLSuffix = "_apis/public/gallery"
)

// BaseSetupConfig contains common configuration for AI Editor Extensions setup
type BaseSetupConfig struct {
	RepoKey       string
	ServiceURL    string
	URLSuffix     string
	ServerDetails *config.ServerDetails
	IsDirectURL   bool
}

func ParseBaseSetupConfig(ctx *components.Context) (*BaseSetupConfig, error) {
	cfg := &BaseSetupConfig{}

	// Check for direct URL first (argument position 1, position 0 is IDE name)
	if ctx.GetNumberOfArgs() > 1 && common.IsValidUrl(ctx.GetArgumentAt(1)) {
		cfg.ServiceURL = ctx.GetArgumentAt(1)
		cfg.RepoKey = common.ExtractRepoKeyFromURL(cfg.ServiceURL, ApiType)
		cfg.IsDirectURL = true
		return cfg, nil
	}

	// Parse flags
	cfg.RepoKey = ctx.GetStringFlagValue(RepoKeyFlag)
	cfg.URLSuffix = ctx.GetStringFlagValue(URLSuffixFlag)
	if cfg.URLSuffix == "" {
		cfg.URLSuffix = DefaultURLSuffix
	}

	if cfg.RepoKey == "" {
		return nil, errors.New("--repo-key flag is required. Please specify the repository key for your AI Editor Extensions repository")
	}

	// Get server details
	rtDetails, err := common.GetServerDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server configuration: %w. Please run 'jf config add' first", err)
	}
	cfg.ServerDetails = rtDetails

	// Validate repository
	if err := common.ValidateRepository(cfg.RepoKey, rtDetails, ApiType); err != nil {
		return nil, err
	}

	// Build service URL
	baseUrl := common.GetBaseUrl(rtDetails)
	cfg.ServiceURL = common.BuildURL(baseUrl, ApiType, cfg.RepoKey, cfg.URLSuffix)
	cfg.IsDirectURL = false

	return cfg, nil
}
