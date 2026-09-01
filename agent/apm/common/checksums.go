package apmcommon

import (
	"encoding/json"
	"fmt"
	"io"
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

	log.Debug(fmt.Sprintf("Created %d AQL batch(es) (batch size: %d, workers: %d).", len(batches), aqlBatchSize, aqlWorkerCount))

	var (
		mu          sync.Mutex
		checksumMap = make(map[string]entities.Checksum)
		wg          sync.WaitGroup
		sem         = make(chan struct{}, aqlWorkerCount)
		errCh       = make(chan error, len(batches))
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
func groupDepsByRepo(deps []ResolvedDep) map[string][]ResolvedDep {
	groups := make(map[string][]ResolvedDep)
	for _, dep := range deps {
		repo := extractRepoFromURL(dep.ResolvedURL)
		groups[repo] = append(groups[repo], dep)
	}
	return groups
}

// extractRepoFromURL extracts the repository name from an Artifactory resolved_url.
// Expected format: https://artifactory.example.com/artifactory/repo-name/...
func extractRepoFromURL(url string) string {
	if url == "" {
		return ""
	}
	// Split by /artifactory/ and take the part after it
	parts := strings.Split(url, "/artifactory/")
	if len(parts) < 2 {
		return ""
	}
	// Extract the repo name (first segment after /artifactory/)
	repoParts := strings.Split(parts[1], "/")
	if len(repoParts) > 0 {
		return repoParts[0]
	}
	return ""
}

// buildBatchAQLQuery constructs an AQL query for a batch of dependencies in a specific repo.
// Query looks for items by their full path: /owner/repo/version/archive
func buildBatchAQLQuery(repo string, deps []ResolvedDep) string {
	var clauses []string
	for _, dep := range deps {
		// Use the dependency ID as the search key (owner/repo:version)
		// AQL queries by path/name, so we build a clause for the archive name
		clause := fmt.Sprintf(`{"name":%q}`, dep.ID)
		clauses = append(clauses, clause)
	}
	return fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("name","actual_sha1","sha256","actual_md5")`,
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
			Name:         it.Name,
			Actual_Sha1:  it.ActualSha1,
			Sha256:       it.Sha256,
			Actual_Md5:   it.ActualMd5,
		})
	}
	return results, nil
}

// mapAQLResults maps AQL results to the checksumMap by matching dependency IDs.
func mapAQLResults(deps []ResolvedDep, results []servicesUtils.ResultItem, checksumMap map[string]entities.Checksum) {
	resultsByKey := make(map[string]servicesUtils.ResultItem)
	for _, r := range results {
		resultsByKey[r.Name] = r
	}

	matched := 0
	var resolvedIDs, missedIDs []string
	for _, dep := range deps {
		if r, ok := resultsByKey[dep.ID]; ok {
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
