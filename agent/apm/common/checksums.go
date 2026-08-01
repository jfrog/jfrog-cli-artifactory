package apmcommon

import (
	"fmt"
	"sync"

	"github.com/jfrog/build-info-go/entities"
	artUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	coreArtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const headWorkerCount = 15

// ResolveChecksums resolves full checksums for registry dependencies.
// Strategy:
//  1. Previous build cache (SHA-1, MD5, SHA-256 from last build).
//  2. HTTP HEAD against each dependency's resolved_url (already the exact download URL — no
//     repo/path reconstruction or query needed), reading Artifactory's X-Checksum-* response
//     headers directly. Confirmed live to match AQL results exactly, and the same mechanism
//     ocicontainer/docker already uses for artifacts it can't resolve via AQL.
//  3. Fallback: use lockfile SHA-256 only when the HEAD request finds no match.
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

	headResults := resolveChecksumsByHead(uncached, servicesManager)
	for id, checksum := range applyHeadResultsOrLockfileFallback(uncached, headResults) {
		checksumMap[id] = checksum
	}
	return checksumMap, nil
}

// selectCachedAndUncached is the tier-1-vs-tier-2 decision: which dependencies already have a
// checksum from the previous build's cache, and which still need a HEAD lookup. Pulled out on
// its own, taking plain maps/slices rather than a live ArtifactoryServicesManager, so this
// selection rule is unit-testable without a real Artifactory connection.
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

// applyHeadResultsOrLockfileFallback is the tier-2-vs-tier-3 decision: for every dependency that
// missed the build cache, use its HEAD-request checksum if one came back with an actual checksum
// value, else fall back to the lockfile's own SHA-256 (dependencies with neither are simply
// omitted - no checksum recorded). A HEAD request can succeed (no error, entry present in
// headResults) while still returning an empty Checksum{} - e.g. Artifactory responding without
// any X-Checksum-* headers - and that must not block the lockfile fallback the same way a real
// miss wouldn't. Pulled out on its own, taking a plain results map rather than making the HTTP
// calls itself, so this selection rule is unit-testable without a real HTTP client.
func applyHeadResultsOrLockfileFallback(uncached []ResolvedDep, headResults map[string]entities.Checksum) map[string]entities.Checksum {
	resolved := make(map[string]entities.Checksum, len(uncached))
	for _, dep := range uncached {
		if checksum, ok := headResults[dep.ID]; ok && hasAnyChecksum(checksum) {
			resolved[dep.ID] = checksum
		} else if dep.SHA256 != "" {
			resolved[dep.ID] = entities.Checksum{Sha256: dep.SHA256}
		}
	}
	return resolved
}

func hasAnyChecksum(checksum entities.Checksum) bool {
	return checksum.Sha1 != "" || checksum.Sha256 != "" || checksum.Md5 != ""
}

// resolveChecksumsByHead issues one HTTP HEAD per dependency against its resolved_url and reads
// sha1/md5/sha256 straight from Artifactory's X-Checksum-* response headers.
func resolveChecksumsByHead(deps []ResolvedDep, servicesManager artifactory.ArtifactoryServicesManager) map[string]entities.Checksum {
	clientDetails := servicesManager.GetConfig().GetServiceDetails().CreateHttpClientDetails()

	var (
		mu          sync.Mutex
		wg          sync.WaitGroup
		sem         = make(chan struct{}, headWorkerCount)
		checksumMap = make(map[string]entities.Checksum, len(deps))
	)

	for _, dep := range deps {
		if dep.ResolvedURL == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(dep ResolvedDep) {
			defer wg.Done()
			defer func() { <-sem }()
			fileDetails, _, err := servicesManager.Client().GetRemoteFileDetails(dep.ResolvedURL, &clientDetails)
			if err != nil {
				log.Debug(fmt.Sprintf("HEAD checksum lookup failed for %s: %s", dep.ID, err.Error()))
				return
			}
			mu.Lock()
			checksumMap[dep.ID] = fileDetails.Checksum
			mu.Unlock()
		}(dep)
	}

	wg.Wait()
	return checksumMap
}
