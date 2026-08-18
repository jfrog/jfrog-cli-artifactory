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

// ParseBaseSetupConfig resolves the AI Editor Extensions gallery configuration.
//
// The positional URL flow fetches a per-user reference token via
// AIEditorExtensionGenerateToken and appends it to the ServiceURL, so that
// curated downloads are attributable to the individual user. The flag flow
// (--repo-key with --url/--server-id/default config) is intentionally
// unchanged and does not tokenize the URL.
//
// Supported input surfaces:
//   - Positional URL:  jf ide setup <ide> https://<host>/artifactory/api/aieditorextensions/<repo>[/...]
//   - --url flag (base) + --repo-key
//   - --repo-key only (uses default server / --server-id)
func ParseBaseSetupConfig(ctx *components.Context) (*BaseSetupConfig, error) {
	if ctx.GetNumberOfArgs() > 1 && common.IsValidUrl(ctx.GetArgumentAt(1)) {
		return parsePositionalURLConfig(ctx)
	}
	return parseFlagConfig(ctx)
}

// parsePositionalURLConfig handles `jf ide setup <ide> <URL>`.
// The URL must contain /api/<ApiType>/<repo-key>/. We derive base URL + repo
// key from the URL, resolve server credentials from standard flags/config,
// call AIEditorExtensionGenerateToken, and append the returned referenceToken.
func parsePositionalURLConfig(ctx *components.Context) (*BaseSetupConfig, error) {
	rawURL := ctx.GetArgumentAt(1)

	// A positional URL already identifies the Artifactory host, so combining
	// it with --server-id (which brings its own host + saved credentials) is
	// ambiguous. Reject the combination up front rather than silently picking
	// one or the other.
	if ctx.IsFlagSet("server-id") {
		return nil, fmt.Errorf(
			"positional URL and --server-id cannot be combined; the URL already identifies the Artifactory host. " +
				"Drop --server-id and let the CLI use the default 'jf config' server " +
				"(or supply --access-token or --user + --password), " +
				"or drop the positional URL and use --repo-key with --server-id.")
	}

	cfg := &BaseSetupConfig{
		ServiceURL:  rawURL,
		IsDirectURL: true,
	}

	baseURL, repoKey, ok := common.SplitApiURL(rawURL, ApiType)
	if !ok {
		return nil, fmt.Errorf(
			"positional URL does not contain '/api/%s/<repo-key>/' — cannot resolve repo key to fetch a reference token. "+
				"Use --repo-key with server credentials, or pass a URL of the form "+
				"https://<host>/artifactory/api/%s/<repo>/_apis/public/gallery", ApiType, ApiType)
	}
	cfg.RepoKey = repoKey

	rtDetails, err := resolveServerDetails(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve server credentials to fetch reference token for repository '%s': %w", repoKey, err)
	}
	cfg.ServerDetails = rtDetails

	token, err := FetchReferenceToken(rtDetails, repoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain reference token for repository '%s': %w", repoKey, err)
	}
	cfg.ServiceURL = AppendReferenceToken(rawURL, token)
	return cfg, nil
}

// parseFlagConfig handles the flag-driven input surfaces. This flow is
// intentionally unchanged by RTECO-1568: it does not fetch a per-user
// reference token. Only the positional URL flow tokenizes the ServiceURL.
func parseFlagConfig(ctx *components.Context) (*BaseSetupConfig, error) {
	cfg := &BaseSetupConfig{}
	cfg.RepoKey = ctx.GetStringFlagValue(RepoKeyFlag)
	cfg.URLSuffix = ctx.GetStringFlagValue(URLSuffixFlag)
	if cfg.URLSuffix == "" {
		cfg.URLSuffix = DefaultURLSuffix
	}

	if cfg.RepoKey == "" {
		if ctx.GetNumberOfArgs() > 1 {
			return nil, fmt.Errorf(
				"positional argument '%s' is not a valid URL (a scheme and host are required, e.g. https://<host>/artifactory/api/%s/<repo>/_apis/public/gallery); "+
					"and --repo-key was not supplied. Provide either a fully-qualified URL positionally or set --repo-key",
				ctx.GetArgumentAt(1), ApiType)
		}
		return nil, errors.New("--repo-key flag is required. Please specify the repository key for your AI Editor Extensions repository")
	}

	if err := common.RequireAuthWhenUrlProvided(ctx); err != nil {
		return nil, err
	}

	rtDetails, err := common.GetServerDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server configuration: %w. Please run 'jf config add' first", err)
	}
	cfg.ServerDetails = rtDetails

	if err := common.ValidateRepository(cfg.RepoKey, rtDetails, ApiType); err != nil {
		return nil, err
	}

	baseUrl := common.GetBaseUrl(rtDetails)
	cfg.ServiceURL = common.BuildURL(baseUrl, ApiType, cfg.RepoKey, cfg.URLSuffix)
	cfg.IsDirectURL = false

	return cfg, nil
}

// resolveServerDetails picks the server credentials to use when only a
// positional URL was supplied. It honors --server-id / --access-token /
// --user+--password / default `jf config` — in that order — via
// common.GetServerDetails, and falls back to constructing details from the
// URL's base host if no configured server matches.
func resolveServerDetails(ctx *components.Context, urlBaseURL string) (*config.ServerDetails, error) {
	if ctx.IsFlagSet("access-token") || (ctx.IsFlagSet("user") && ctx.IsFlagSet("password")) {
		// Explicit creds on the CLI take precedence; pair them with the URL's base.
		return common.BuildServerDetailsFromBaseURL(ctx, urlBaseURL)
	}
	// Otherwise defer to --server-id / default jf config resolution.
	return common.GetServerDetails(ctx)
}
