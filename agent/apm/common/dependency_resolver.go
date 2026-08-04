package apmcommon

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// depsWhyWorkerCount bounds concurrent `apm deps why` subprocesses (mirrors headWorkerCount).
const depsWhyWorkerCount = 15

// depsWhyTimeout bounds a single `apm deps why` subprocess, so a hang can't block forever.
const depsWhyTimeout = 30 * time.Second

// Dependency scope names. A dependency gets exactly one of these, chosen by finalScope's
// priority ladder (prod > dev > transitive) - the same mutually-exclusive model pnpm's own
// resolver in this repo uses (artifactory/commands/pnpm/dependency_resolver.go's addScope),
// rather than combining them.
const (
	apmScopeProd       = "prod"
	apmScopeDev        = "dev"
	apmScopeTransitive = "transitive"
)

// finalScope picks a single scope from whether a dependency is direct and whether it's a dev
// dependency, following pnpm's priority ladder: prod > dev > transitive. isDev is apm's own
// lockfile is_dev flag, which already resolves a dependency needed by both a prod and a dev
// path to false, so prod-over-dev priority falls out for free.
func finalScope(isDirect, isDev bool) string {
	if isDirect && !isDev {
		return apmScopeProd
	}
	if isDev {
		return apmScopeDev
	}
	return apmScopeTransitive
}

// ResolvedDep holds a single APM registry dependency ready for build-info.
type ResolvedDep struct {
	ID          string // "owner/repo:version"
	RepoURL     string // "owner/repo" - needed to query `apm deps why`
	SHA256      string // hex-encoded SHA-256 from lockfile
	ResolvedURL string // full agentpackages download URL from the lockfile
	Scopes      []string
	RequestedBy [][]string
}

// ResolveDependencies reads the lockfile and returns only registry-sourced dependencies.
// Resolves each dependency's scope/requestedBy concurrently (bounded by depsWhyWorkerCount).
func ResolveDependencies(lockfilePath string) ([]ResolvedDep, error) {
	lockfile, err := LoadLockFile(lockfilePath)
	if err != nil {
		return nil, err
	}

	workingDir := filepath.Dir(lockfilePath)
	packages := lockfile.RegistryPackages()
	deps := make([]ResolvedDep, len(packages))

	var wg sync.WaitGroup
	sem := make(chan struct{}, depsWhyWorkerCount)
	for i, pkg := range packages {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, pkg ApmLockedPackage) {
			defer wg.Done()
			defer func() { <-sem }()
			isDirect, requestedBy := resolveDirectAndRequestedBy(workingDir, pkg.RepoURL)
			deps[i] = ResolvedDep{
				ID:          pkg.DepID(),
				RepoURL:     pkg.RepoURL,
				SHA256:      SHA256Hex(pkg.ResolvedHash),
				ResolvedURL: pkg.ResolvedURL,
				Scopes:      []string{finalScope(isDirect, pkg.IsDev)},
				RequestedBy: requestedBy,
			}
		}(i, pkg)
	}
	wg.Wait()
	return deps, nil
}

// ToEntitiesDependency converts a ResolvedDep to entities.Dependency with resolved checksums.
func (dep ResolvedDep) ToEntitiesDependency(checksum entities.Checksum) entities.Dependency {
	return entities.Dependency{
		Id:          dep.ID,
		Type:        apmPackageFileExtension,
		Scopes:      dep.Scopes,
		RequestedBy: dep.RequestedBy,
		Checksum:    checksum,
	}
}

// requestedByMaxPaths caps requestedBy paths per dependency, bounding fan-in from widely-shared
// packages (e.g. a diamond dependency's base), mirroring entities.RequestedByMaxLength.
const requestedByMaxPaths = entities.RequestedByMaxLength

// apmDepsWhyResult is the `apm deps why <repo_url> --json` response.
type apmDepsWhyResult struct {
	Package struct {
		IsDirect bool `json:"is_direct"`
	} `json:"package"`
	Paths []struct {
		Chain []struct {
			RepoURL string `json:"repo_url"`
		} `json:"chain"`
	} `json:"paths"`
}

// resolveDirectAndRequestedBy shells out to `apm deps why <repoURL> --json` in workingDir to
// determine whether a dependency is direct or transitive, and - for transitive ones - which
// package(s) requested it. Best-effort: any failure defaults to isDirect=true (matching the
// old "default to prod scope" fallback) with no requestedBy, rather than failing the whole
// build-info collection.
func resolveDirectAndRequestedBy(workingDir, repoURL string) (isDirect bool, requestedBy [][]string) {
	// repoURL comes from apm.lock.yaml, not a trusted CLI arg - reject flag-shaped values so a
	// tampered lockfile can't smuggle an extra flag into the apm invocation below.
	if strings.HasPrefix(repoURL, "-") {
		log.Debug(fmt.Sprintf("Refusing to run apm deps why for suspicious repo_url %q, defaulting to direct", repoURL))
		return true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), depsWhyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ApmBinaryName, "deps", "why", repoURL, "--json") // #nosec G204 -- repoURL is validated above to reject flag-shaped values; exec.Command never invokes a shell, so no injection vector remains
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug(fmt.Sprintf("apm deps why %s failed, defaulting to direct: %s", repoURL, err))
		return true, nil
	}
	return parseDepsWhyOutput(out, repoURL)
}

// parseDepsWhyOutput turns `apm deps why --json` output into a direct/transitive flag and
// requestedBy chains. Split out from resolveDirectAndRequestedBy so the parsing logic is
// testable without shelling out to a real apm binary.
func parseDepsWhyOutput(out []byte, repoURL string) (isDirect bool, requestedBy [][]string) {
	var result apmDepsWhyResult
	if err := json.Unmarshal(out, &result); err != nil {
		log.Debug(fmt.Sprintf("could not parse apm deps why %s output, defaulting to direct: %s", repoURL, err))
		return true, nil
	}

	if result.Package.IsDirect {
		return true, nil
	}

	for _, path := range result.Paths {
		if len(requestedBy) >= requestedByMaxPaths {
			break // widely-shared package (e.g. a diamond dependency's base) - cap fan-in
		}
		if len(path.Chain) <= 1 {
			continue // no parent to report
		}
		parents := path.Chain[:len(path.Chain)-1] // drop the target package itself
		chain := make([]string, 0, len(parents))
		for _, node := range parents {
			chain = append(chain, node.RepoURL)
		}
		requestedBy = append(requestedBy, chain)
	}
	return false, requestedBy
}
