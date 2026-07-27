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
func ResolveChecksums(deps []ResolvedDep, sd *config.ServerDetails, buildConfig *buildUtils.BuildConfiguration) (map[string]entities.Checksum, error) {
	checksumMap := make(map[string]entities.Checksum)

	servicesManager, err := coreArtUtils.CreateServiceManager(sd, -1, 0, false)
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

	var uncached []ResolvedDep
	for _, dep := range deps {
		if cs, ok := cachedChecksums[dep.ID]; ok {
			checksumMap[dep.ID] = cs
		} else {
			uncached = append(uncached, dep)
		}
	}

	log.Info(fmt.Sprintf("Checksum resolution: %d cached, resolving %d from Artifactory.", len(deps)-len(uncached), len(uncached)))

	if len(uncached) == 0 {
		return checksumMap, nil
	}

	headResults := resolveChecksumsByHead(uncached, servicesManager)
	for _, dep := range uncached {
		if cs, ok := headResults[dep.ID]; ok {
			checksumMap[dep.ID] = cs
		} else if dep.SHA256 != "" {
			checksumMap[dep.ID] = entities.Checksum{Sha256: dep.SHA256}
		}
	}
	return checksumMap, nil
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
		go func(d ResolvedDep) {
			defer wg.Done()
			defer func() { <-sem }()
			fileDetails, _, err := servicesManager.Client().GetRemoteFileDetails(d.ResolvedURL, &clientDetails)
			if err != nil {
				log.Debug(fmt.Sprintf("HEAD checksum lookup failed for %s: %s", d.ID, err.Error()))
				return
			}
			mu.Lock()
			checksumMap[d.ID] = fileDetails.Checksum
			mu.Unlock()
		}(dep)
	}

	wg.Wait()
	return checksumMap
}
