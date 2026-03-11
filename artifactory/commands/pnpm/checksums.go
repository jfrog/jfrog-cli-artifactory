package pnpm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jfrog/build-info-go/entities"
	artUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	aqlBatchSize   = 30
	aqlWorkerCount = 15
)

func fetchChecksums(deps []parsedDep, serverDetails *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration, workingDir string) (map[string]entities.Checksum, error) {
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return nil, err
	}

	checksumMap := make(map[string]entities.Checksum)

	// Tier 1: Load from previous build cache
	buildName, err := buildConfig.GetBuildName()
	if err != nil {
		return nil, err
	}
	log.Debug("Loading checksums from previous build:", buildName)
	previousBuildDeps, err := artUtils.GetDependenciesFromLatestBuild(servicesManager, buildName)
	if err != nil {
		log.Debug("Could not load previous build dependencies:", err.Error())
	} else {
		log.Debug(fmt.Sprintf("Found %d dependencies in previous build cache.", len(previousBuildDeps)))
	}
	cachedChecksums := artUtils.DependenciesToChecksumMap(previousBuildDeps)

	var uncachedDeps []parsedDep
	var cachedIDs []string
	for _, pd := range deps {
		depID := pd.dep.name + ":" + pd.dep.version
		if cs, ok := cachedChecksums[depID]; ok {
			checksumMap[depID] = cs
			cachedIDs = append(cachedIDs, depID)
		} else {
			uncachedDeps = append(uncachedDeps, pd)
		}
	}

	if len(cachedIDs) > 0 {
		log.Debug(fmt.Sprintf("Resolved %d dependencies from previous build cache: %v", len(cachedIDs), cachedIDs))
	}
	if len(uncachedDeps) > 0 {
		uncachedIDs := make([]string, len(uncachedDeps))
		for i, pd := range uncachedDeps {
			uncachedIDs[i] = pd.dep.name + ":" + pd.dep.version
		}
		log.Debug(fmt.Sprintf("Fetching checksums via AQL for %d dependencies: %v", len(uncachedIDs), uncachedIDs))
	}
	log.Info(fmt.Sprintf("Checksum resolution: %d cached, %d require AQL lookup.", len(cachedIDs), len(uncachedDeps)))

	if len(uncachedDeps) == 0 {
		log.Debug("All dependencies resolved from previous build cache. No AQL queries needed.")
		return checksumMap, nil
	}

	// Tier 2: Batched AQL for uncached deps
	aqlChecksums := batchedAQLFetch(uncachedDeps, servicesManager, workingDir)

	aqlResolved := 0
	for k, v := range aqlChecksums {
		checksumMap[k] = v
		if !v.IsEmpty() {
			aqlResolved++
		}
	}
	log.Debug(fmt.Sprintf("AQL resolved %d/%d uncached dependencies.", aqlResolved, len(uncachedDeps)))
	return checksumMap, nil
}

func batchedAQLFetch(deps []parsedDep, servicesManager artifactory.ArtifactoryServicesManager, workingDir string) map[string]entities.Checksum {
	repoGroups := groupByRepo(deps, workingDir)

	var batches []aqlBatch
	for repo, group := range repoGroups {
		aqlRepo := resolveAqlRepo(repo, servicesManager)
		log.Debug(fmt.Sprintf("Repo '%s' → AQL target '%s': %d dependencies.", repo, aqlRepo, len(group)))
		for i := 0; i < len(group); i += aqlBatchSize {
			end := i + aqlBatchSize
			if end > len(group) {
				end = len(group)
			}
			batches = append(batches, aqlBatch{repo: aqlRepo, deps: group[i:end]})
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
			results, err := artUtils.ExecuteAqlQuery(servicesManager, query)
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

func buildBatchAQLQuery(repo string, deps []parsedDep) string {
	var clauses []string
	for _, dp := range deps {
		clause := fmt.Sprintf(
			`{"$and":[{"path":"%s"},{"name":"%s"}]}`,
			dp.parts.dirPath, dp.parts.fileName,
		)
		clauses = append(clauses, clause)
	}
	return fmt.Sprintf(
		`items.find({"repo":"%s","$or":[%s]}).include("repo","path","name","actual_sha1","sha256","actual_md5")`,
		repo, strings.Join(clauses, ","),
	)
}

func mapAQLResults(deps []parsedDep, results []servicesUtils.ResultItem, checksumMap map[string]entities.Checksum) {
	resultsByKey := make(map[string]servicesUtils.ResultItem)
	for _, r := range results {
		key := r.Path + "/" + r.Name
		resultsByKey[key] = r
	}

	matched := 0
	var resolvedIDs, missedIDs []string
	for _, dp := range deps {
		key := dp.parts.dirPath + "/" + dp.parts.fileName
		if r, ok := resultsByKey[key]; ok {
			depID := dp.dep.name + ":" + dp.dep.version
			checksumMap[depID] = entities.Checksum{
				Sha1:   r.Actual_Sha1,
				Md5:    r.Actual_Md5,
				Sha256: r.Sha256,
			}
			resolvedIDs = append(resolvedIDs, depID)
			matched++
		} else {
			missedIDs = append(missedIDs, dp.dep.name+":"+dp.dep.version)
		}
	}
	if len(resolvedIDs) > 0 {
		log.Debug(fmt.Sprintf("AQL checksums resolved for %d dependencies: %v", len(resolvedIDs), resolvedIDs))
	}
	if len(missedIDs) > 0 {
		log.Debug(fmt.Sprintf("No AQL results for %d dependencies: %v", len(missedIDs), missedIDs))
	}
}
