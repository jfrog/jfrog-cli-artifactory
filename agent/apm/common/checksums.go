package apmcommon

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/jfrog/build-info-go/entities"
	artUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	coreArtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const aqlBatchSize = 30
const aqlWorkerCount = 15

// aqlResult is the subset of the AQL response we consume.
type aqlResult struct {
	Results []struct {
		Path       string `json:"path"`
		Name       string `json:"name"`
		ActualSha1 string `json:"actual_sha1"`
		Sha256     string `json:"sha256"`
		ActualMd5  string `json:"actual_md5"`
	} `json:"results"`
}

// ResolveChecksums resolves full checksums for registry dependencies.
// Strategy:
//  1. Previous build cache (SHA-1, MD5, SHA-256 from last build).
//  2. Batched AQL queries to Artifactory (up to 30 deps per query, 15 workers in parallel).
func ResolveChecksums(deps []ResolvedDep, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) (map[string]entities.Checksum, error) {
	servicesManager, err := coreArtUtils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, err
	}

	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return nil, err
	}
	prevDeps, cacheErr := artUtils.GetDependenciesFromLatestBuild(servicesManager, buildName, buildConfig.GetProject())
	if cacheErr != nil {
		log.Debug("Could not load previous build deps:", cacheErr.Error())
	}
	cachedChecksums := artUtils.DependenciesToChecksumMap(prevDeps)

	checksumMap, uncached := selectCachedAndUncached(deps, cachedChecksums)

	log.Info(fmt.Sprintf("Checksum resolution: %d cached, resolving %d from Artifactory.", len(deps)-len(uncached), len(uncached)))

	if len(uncached) == 0 {
		return checksumMap, nil
	}

	aqlResults := batchedAQLFetch(uncached, servicesManager)
	aqlResolved := 0
	for k, v := range aqlResults {
		checksumMap[k] = v
		if !v.IsEmpty() {
			aqlResolved++
		}
	}
	log.Debug(fmt.Sprintf("AQL resolved %d/%d uncached dependencies.", aqlResolved, len(uncached)))

	return checksumMap, nil
}

// selectCachedAndUncached is the tier-1-vs-tier-2 decision: which dependencies already have a
// checksum from the previous build's cache, and which still need an AQL lookup.
func selectCachedAndUncached(deps []ResolvedDep, cachedChecksums map[string]entities.Checksum) (cached map[string]entities.Checksum, uncached []ResolvedDep) {
	cached = make(map[string]entities.Checksum)
	for _, dep := range deps {
		if checksum, ok := cachedChecksums[dep.ID]; ok {
			cached[dep.ID] = checksum
		} else {
			uncached = append(uncached, dep)
		}
	}
	return cached, uncached
}

// batchedAQLFetch groups uncached dependencies into batches and queries Artifactory via AQL
// with up to aqlWorkerCount workers in parallel, each handling one batch of up to aqlBatchSize deps.
func batchedAQLFetch(deps []ResolvedDep, servicesManager artifactory.ArtifactoryServicesManager) map[string]entities.Checksum {
	if len(deps) == 0 {
		return map[string]entities.Checksum{}
	}

	// Group deps by repository (since AQL queries are per-repo)
	repoGroups := groupDepsByRepo(deps)

	var batches []aqlBatch
	for repo, group := range repoGroups {
		for i := 0; i < len(group); i += aqlBatchSize {
			end := i + aqlBatchSize
			if end > len(group) {
				end = len(group)
			}
			batches = append(batches, aqlBatch{repo: repo, deps: group[i:end]})
		}
	}

	checksumMap := make(map[string]entities.Checksum)

	if len(batches) == 0 {
		log.Debug("No valid dependencies with resolvable repositories; skipping AQL.")
		return checksumMap
	}

	log.Debug(fmt.Sprintf("Created %d AQL batch(es) (batch size: %d, workers: %d).", len(batches), aqlBatchSize, aqlWorkerCount))

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, aqlWorkerCount)
		errCh = make(chan error, len(batches))
	)

	for _, batch := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(b aqlBatch) {
			defer wg.Done()
			defer func() { <-sem }()

			query := buildBatchAQLQuery(b.repo, b.deps)
			log.Debug(fmt.Sprintf("Executing AQL query for repo '%s' with %d items...", b.repo, len(b.deps)))
			results, err := executeAQL(servicesManager, query)
			if err != nil {
				errCh <- fmt.Errorf("AQL batch failed for repo '%s': %w", b.repo, err)
				return
			}
			mu.Lock()
			mapAQLResults(b.deps, results, checksumMap)
			mu.Unlock()
		}(batch)
	}

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		log.Warn(fmt.Sprintf("AQL checksum resolution encountered %d error(s): %v", len(errs), errs))
	}

	return checksumMap
}

type aqlBatch struct {
	repo string
	deps []ResolvedDep
}

// groupDepsByRepo groups dependencies by their repository (extracted from their resolved_url).
// Dependencies with empty or malformed ResolvedURL are grouped under empty key; batchedAQLFetch
// will skip them with a warning.
func groupDepsByRepo(deps []ResolvedDep) map[string][]ResolvedDep {
	groups := make(map[string][]ResolvedDep)
	for _, dep := range deps {
		if dep.ResolvedURL == "" {
			log.Debug(fmt.Sprintf("Skipping dependency %s: empty ResolvedURL", dep.ID))
			continue
		}
		repo := extractRepoFromURL(dep.ResolvedURL)
		if repo == "" {
			log.Warn(fmt.Sprintf("Could not extract repository from ResolvedURL for %s: %s", dep.ID, dep.ResolvedURL))
			continue
		}
		groups[repo] = append(groups[repo], dep)
	}
	return groups
}

// agentPackagesURLPattern matches an APM agentpackages download URL and captures the
// repository, package owner, package name, and version. Every ResolvedDep.ResolvedURL in
// this package has this exact shape (it comes straight from apm.lock.yaml's resolved_url),
// so there is no other format to account for:
//
//	https://<host>/artifactory/api/agentpackages/<repo>/v1/packages/<owner>/<name>/versions/<version>/download
var agentPackagesURLPattern = regexp.MustCompile(`/artifactory/api/agentpackages/([^/]+)/v1/packages/([^/]+)/([^/]+)/versions/([^/]+)/download`)

// parseAgentPackagesURL extracts (owner, name, version) from an APM resolved_url.
// ok is false if the URL doesn't match the agentpackages download shape.
func parseAgentPackagesURL(url string) (owner, name, version string, ok bool) {
	m := agentPackagesURLPattern.FindStringSubmatch(url)
	if m == nil {
		return "", "", "", false
	}
	return m[2], m[3], m[4], true
}

// extractRepoFromURL extracts the repository name from an Artifactory resolved_url.
// Handles both classic and APM URL formats:
// - Classic: https://artifactory.example.com/artifactory/repo-name/...
// - APM: https://artifactory.example.com/artifactory/api/agentpackages/repo-name/...
func extractRepoFromURL(url string) string {
	if url == "" {
		return ""
	}

	// Try APM format first: /artifactory/api/agentpackages/repo-name/
	apmPrefix := "/artifactory/api/agentpackages/"
	if idx := strings.Index(url, apmPrefix); idx != -1 {
		remainder := url[idx+len(apmPrefix):]
		if slash := strings.Index(remainder, "/"); slash != -1 {
			repo := remainder[:slash]
			if repo != "" {
				return repo
			}
		}
	}

	// Fall back to classic format: /artifactory/repo-name/
	classicPrefix := "/artifactory/"
	if idx := strings.Index(url, classicPrefix); idx != -1 {
		remainder := url[idx+len(classicPrefix):]
		if slash := strings.Index(remainder, "/"); slash != -1 {
			repo := remainder[:slash]
			if repo != "" {
				return repo
			}
		}
	}

	return ""
}

// extractArtifactPath extracts the path within the repository for an APM package.
// For APM URLs, this is: /owner/package-name (used for deduplication with path+name).
func extractArtifactPath(url string) string {
	owner, name, _, ok := parseAgentPackagesURL(url)
	if !ok {
		return ""
	}
	return owner + "/" + name
}

// extractArtifactFilename returns the real artifact filename Artifactory stores the package
// under: <name>-<version>.zip. The download URL itself ends in "/download", which is not
// a real filename, so it cannot be derived by taking the URL's last path segment.
func extractArtifactFilename(url string) string {
	_, name, version, ok := parseAgentPackagesURL(url)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s-%s.zip", name, version)
}

// buildBatchAQLQuery constructs an AQL query for a batch of dependencies in a specific repo.
// Queries by artifact filename (name) since path-based queries don't work reliably for APM packages.
func buildBatchAQLQuery(repo string, deps []ResolvedDep) string {
	var clauses []string
	for _, dep := range deps {
		// Extract the real artifact filename from the ResolvedURL
		// For APM URLs, this parses the path to extract name and version
		artifactFilename := extractArtifactFilename(dep.ResolvedURL)
		if artifactFilename == "" {
			log.Debug(fmt.Sprintf("Could not extract artifact filename from ResolvedURL for %s: %s", dep.ID, dep.ResolvedURL))
			continue
		}
		// Query by name in the repository
		clause := fmt.Sprintf(`{"name":%q}`, artifactFilename)
		clauses = append(clauses, clause)
	}
	if len(clauses) == 0 {
		// Return a query that will return no results if we can't extract any filenames
		return fmt.Sprintf(`items.find({"repo":%q,"name":"/NEVER_MATCHES/"}).include("name","actual_sha1","sha256","actual_md5")`, repo)
	}
	return fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("path","name","actual_sha1","sha256","actual_md5")`,
		repo, strings.Join(clauses, ","),
	)
}

// executeAQL runs an AQL query against Artifactory and returns the parsed results.
func executeAQL(servicesManager artifactory.ArtifactoryServicesManager, query string) ([]servicesUtils.ResultItem, error) {
	body, err := servicesManager.Aql(query)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		return nil, err
	}
	defer func() {
		if cerr := body.Close(); cerr != nil {
			log.Debug("checksums: aql body close: " + cerr.Error())
		}
	}()

	return parseAQLResults(body)
}

// parseAQLResults parses the AQL response body into structured results.
func parseAQLResults(r io.Reader) ([]servicesUtils.ResultItem, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var res aqlResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse aql response: %w", err)
	}
	var results []servicesUtils.ResultItem
	for _, it := range res.Results {
		results = append(results, servicesUtils.ResultItem{
			Path:        it.Path,
			Name:        it.Name,
			Actual_Sha1: it.ActualSha1,
			Sha256:      it.Sha256,
			Actual_Md5:  it.ActualMd5,
		})
	}
	return results, nil
}

// mapAQLResults maps AQL results to the checksumMap by matching dependency artifact paths.
// Results are keyed by path+name to handle owner collisions (e.g., owner-a/tool and owner-b/tool both produce tool-1.0.0.zip).
func mapAQLResults(deps []ResolvedDep, results []servicesUtils.ResultItem, checksumMap map[string]entities.Checksum) {
	// Build a map of (path+name) to results for matching
	resultsByPathAndName := make(map[string]servicesUtils.ResultItem)
	for _, r := range results {
		// Combine path and name to create unique key (handles owner collisions).
		// Normalize path: remove leading slash if present (AQL returns "/owner/name", we need "owner/name").
		path := strings.TrimPrefix(r.Path, "/")
		key := path + "/" + r.Name
		resultsByPathAndName[key] = r
	}

	matched := 0
	var resolvedIDs, missedIDs []string
	for _, dep := range deps {
		// Extract artifact path from the ResolvedURL for matching
		artifactPath := extractArtifactPath(dep.ResolvedURL)
		if artifactPath == "" {
			missedIDs = append(missedIDs, dep.ID)
			continue
		}

		// Extract filename for composite key matching
		artifactFilename := extractArtifactFilename(dep.ResolvedURL)
		if artifactFilename == "" {
			missedIDs = append(missedIDs, dep.ID)
			continue
		}

		// Look up by path+name (path from URL includes owner, prevents collisions)
		key := artifactPath + "/" + artifactFilename
		if r, ok := resultsByPathAndName[key]; ok {
			checksumMap[dep.ID] = entities.Checksum{
				Sha1:   r.Actual_Sha1,
				Md5:    r.Actual_Md5,
				Sha256: r.Sha256,
			}
			resolvedIDs = append(resolvedIDs, dep.ID)
			matched++
		} else {
			missedIDs = append(missedIDs, dep.ID)
		}
	}
	if len(resolvedIDs) > 0 {
		log.Debug(fmt.Sprintf("AQL checksums resolved for %d dependencies: %v", len(resolvedIDs), resolvedIDs))
	}
	if len(missedIDs) > 0 {
		log.Debug(fmt.Sprintf("No AQL results for %d dependencies: %v", len(missedIDs), missedIDs))
	}
}

// hasAnyChecksum returns true if the checksum contains at least one non-empty hash value.
func hasAnyChecksum(checksum entities.Checksum) bool {
	return checksum.Sha1 != "" || checksum.Sha256 != "" || checksum.Md5 != ""
}
