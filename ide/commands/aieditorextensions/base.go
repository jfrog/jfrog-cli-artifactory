package aieditorextensions

import (
	"errors"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/ide/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
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

	// Resolve server details.
	// - --user/--password/--access-token/--server-id supplied → build from flags.
	// - --url supplied alone → use the configured default
	//   server and override the URL. If no default server exists, return a clear
	//   error asking the user to configure one or supply credentials.
	// - Nothing supplied → default server configuration.
	rtDetails, err := common.GetServerDetails(ctx)
	if err != nil {
		return nil, err
	}
	cfg.ServerDetails = rtDetails

	// If --url was supplied, normalize it before we make any network call.
	//  and rejects obviously wrong shapes like a full /api/… URL.
	if ctx.IsFlagSet("url") {
		normalized, nerr := common.NormalizeArtifactoryBaseUrl(rtDetails.ArtifactoryUrl, cfg.RepoKey)
		if nerr != nil {
			return nil, nerr
		}
		if normalized != rtDetails.ArtifactoryUrl {
			log.Info(fmt.Sprintf(
				"--url %q ended with the --repo-key value %q; using Artifactory base URL: %s",
				rtDetails.ArtifactoryUrl, cfg.RepoKey, normalized))
			rtDetails.ArtifactoryUrl = normalized
		}
	}

	// Validate the repository exists and is of the expected type. Credentials
	// come from whichever source GetServerDetails resolved.
	if err := common.ValidateRepository(cfg.RepoKey, rtDetails, ApiType); err != nil {
		return nil, err
	}

	baseUrl := common.GetBaseUrl(rtDetails)
	cfg.ServiceURL = common.BuildURL(baseUrl, ApiType, cfg.RepoKey, cfg.URLSuffix)
	cfg.IsDirectURL = false

	return cfg, nil
}
