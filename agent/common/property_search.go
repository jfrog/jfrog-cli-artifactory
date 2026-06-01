package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// PropertySearchResult is one artifact hit from Artifactory GET api/search/prop.
type PropertySearchResult struct {
	Repo    string
	Name    string
	Version string
	URI     string
}

// PropertySearchOptions configures a property search by package name key.
type PropertySearchOptions struct {
	NamePropertyKey string
	Query           string
	RepoKey         string
}

type propSearchResponse struct {
	Results []propSearchResultItem `json:"results"`
}

type propSearchResultItem struct {
	URI string `json:"uri"`
}

// SearchByProperty calls GET api/search/prop?{namePropertyKey}={query}[&repos={repoKey}].
func SearchByProperty(serverDetails *config.ServerDetails, opts PropertySearchOptions) ([]PropertySearchResult, error) {
	query, err := validatePropertySearchOpts(opts)
	if err != nil {
		return nil, err
	}
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	artURL := clientutils.AddTrailingSlashIfNeeded(sm.GetConfig().GetServiceDetails().GetUrl())
	searchURL := propertySearchRequestURL(artURL, opts, query)
	uris, err := fetchPropertySearchURIs(sm, searchURL)
	if err != nil {
		return nil, err
	}
	return propertySearchResultsFromURIs(uris), nil
}

func validatePropertySearchOpts(opts PropertySearchOptions) (string, error) {
	if strings.TrimSpace(opts.NamePropertyKey) == "" {
		return "", fmt.Errorf("name property key is required for property search")
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return "", fmt.Errorf("search query is required for property search")
	}
	return query, nil
}

func propertySearchRequestURL(artURL string, opts PropertySearchOptions, query string) string {
	searchURL := fmt.Sprintf("%sapi/search/prop?%s=%s", artURL, opts.NamePropertyKey, url.QueryEscape(query))
	if strings.TrimSpace(opts.RepoKey) != "" {
		searchURL += "&repos=" + url.QueryEscape(opts.RepoKey)
	}
	return searchURL
}

func fetchPropertySearchURIs(sm artifactory.ArtifactoryServicesManager, searchURL string) ([]string, error) {
	log.Debug("Property search request:", searchURL)

	httpDetails := sm.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	resp, body, _, err := sm.Client().SendGet(searchURL, true, &httpDetails)
	if err != nil {
		return nil, err
	}
	if err = errorutils.CheckResponseStatusWithBody(resp, body, http.StatusOK); err != nil {
		return nil, err
	}
	var wrapper propSearchResponse
	if err = json.Unmarshal(body, &wrapper); err != nil {
		return nil, errorutils.CheckErrorf("failed to parse property search response: %s", err.Error())
	}
	uris := make([]string, len(wrapper.Results))
	for i, item := range wrapper.Results {
		uris[i] = item.URI
	}
	return uris, nil
}

func propertySearchResultsFromURIs(uris []string) []PropertySearchResult {
	results := make([]PropertySearchResult, 0, len(uris))
	for _, uri := range uris {
		parsed, ok := ParsePropertySearchURI(uri)
		if !ok {
			log.Warn(fmt.Sprintf("Skipping property search result with unparseable URI: %s", uri))
			continue
		}
		results = append(results, parsed)
	}
	return results
}

// ParsePropertySearchURI extracts repo, slug, and version from a storage URI like:
// https://host/artifactory/api/storage/{repo}/{slug}/{version}/{slug}-{version}.zip
func ParsePropertySearchURI(uri string) (PropertySearchResult, bool) {
	idx := strings.Index(uri, "/api/storage/")
	if idx == -1 {
		return PropertySearchResult{}, false
	}
	path := uri[idx+len("/api/storage/"):]
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 3 {
		return PropertySearchResult{}, false
	}
	return PropertySearchResult{
		Repo:    parts[0],
		Name:    parts[1],
		Version: parts[2],
		URI:     uri,
	}, true
}

// GetItemPropertyDescription returns the first non-empty value among descriptionPropertyKeys on repoPath.
func GetItemPropertyDescription(
	serverDetails *config.ServerDetails,
	repoPath string,
	descriptionPropertyKeys []string,
) (string, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return "", err
	}
	props, err := sm.GetItemProps(repoPath)
	if err != nil {
		return "", err
	}
	for _, key := range descriptionPropertyKeys {
		if descs, ok := props.Properties[key]; ok && len(descs) > 0 {
			return descs[0], nil
		}
	}
	return "", nil
}

// SearchRowsByProperty runs property search and resolves optional description properties per hit.
func SearchRowsByProperty(
	serverDetails *config.ServerDetails,
	opts PropertySearchOptions,
	descriptionPropertyKeys []string,
) ([]SearchResultRow, error) {
	hits, err := SearchByProperty(serverDetails, opts)
	if err != nil {
		return nil, err
	}
	rows := make([]SearchResultRow, 0, len(hits))
	for _, hit := range hits {
		desc := ""
		repoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", hit.Repo, hit.Name, hit.Version, hit.Name, hit.Version)
		d, err := GetItemPropertyDescription(serverDetails, repoPath, descriptionPropertyKeys)
		if err != nil {
			log.Debug(fmt.Sprintf("Could not fetch description for %s: %s", repoPath, err.Error()))
		} else {
			desc = d
		}
		rows = append(rows, SearchResultRow{
			Name:        hit.Name,
			Version:     hit.Version,
			Repository:  hit.Repo,
			Description: desc,
		})
	}
	return rows, nil
}
