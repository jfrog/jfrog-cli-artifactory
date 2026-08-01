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

// depsWhyWorkerCount bounds how many `apm deps why` subprocesses run concurrently - mirrors
// headWorkerCount in checksums.go, the same bounded-concurrency budget for a similar
// per-dependency subprocess/request fan-out.
const depsWhyWorkerCount = 15

// depsWhyTimeout bounds a single `apm deps why` subprocess. Without it, a hang (not a failure)
// would block forever and the "best-effort, falls back to prod scope" guarantee below would
// never actually trigger.
const depsWhyTimeout = 30 * time.Second

// Dependency scope names. "prod" matches the direct-dependency label the newer sibling
// FlexPack integrations (Alpine's AlpineScopeProd, Cargo's "prod") converged on, rather than
// "runtime" (the older, now-minority convention nix alone still uses).
const (
	apmScopeProd       = "prod"
	apmScopeTransitive = "transitive"
)

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
// Resolves each dependency's scope/requestedBy concurrently (bounded by depsWhyWorkerCount),
// since each one spawns its own `apm deps why` subprocess and a large lockfile would otherwise
// pay subprocess-startup + I/O cost sequentially, one dependency at a time.
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
			scopes, requestedBy := resolveScopeAndRequestedBy(workingDir, pkg.RepoURL)
			deps[i] = ResolvedDep{
				ID:          pkg.DepID(),
				RepoURL:     pkg.RepoURL,
				SHA256:      SHA256Hex(pkg.ResolvedHash),
				ResolvedURL: pkg.ResolvedURL,
				Scopes:      scopes,
				RequestedBy: requestedBy,
			}
		}(i, pkg)
	}
	wg.Wait()
	return deps, nil
}

// ToEntitiesDependency converts a ResolvedDep to entities.Dependency with resolved checksums.
// Type is "zip" — confirmed live against Artifactory's real agentpackages storage layout.
func (dep ResolvedDep) ToEntitiesDependency(checksum entities.Checksum) entities.Dependency {
	return entities.Dependency{
		Id:          dep.ID,
		Type:        "zip",
		Scopes:      dep.Scopes,
		RequestedBy: dep.RequestedBy,
		Checksum:    checksum,
	}
}

// requestedByMaxPaths caps how many distinct requestedBy paths are reported per dependency,
// mirroring entities.RequestedByMaxLength - the same limit golang.go/yarn.go/uv_flexpack.go
// apply to len(dependency.RequestedBy) to bound fan-in from widely-shared packages (the
// common runaway case; a diamond dependency is exactly this: many packages sharing one base).
const requestedByMaxPaths = entities.RequestedByMaxLength

// apmDepsWhyResult is the `apm deps why <repo_url> --json` response. Preferred over the
// lockfile's own depth/resolved_by fields, which aren't part of any documented schema (and
// aren't modeled in ApmLockedPackage) - this is a stable, documented command surface built
// specifically to answer "is this direct, and who pulled it in".
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

// resolveScopeAndRequestedBy shells out to `apm deps why <repoURL> --json` in workingDir to
// determine whether a dependency is direct or transitive, and - for transitive ones - which
// package(s) requested it. Preferred over the lockfile's own depth/resolved_by fields: `deps
// why` is a documented, stable command built for exactly this question, and naturally handles
// a dependency reachable through more than one parent (each returned path becomes one
// RequestedBy chain), which a single resolved_by string in the lockfile can't represent.
//
// Best-effort: if apm isn't on PATH or the command fails for any reason, this falls back to
// prod scope with no requestedBy rather than failing the whole build-info collection - the
// dependency's id/checksum are still correct either way.
func resolveScopeAndRequestedBy(workingDir, repoURL string) (scopes []string, requestedBy [][]string) {
	// repoURL comes from apm.lock.yaml, not a trusted CLI arg - a tampered lockfile could set
	// it to something starting with "-" to smuggle an extra flag into the apm invocation
	// below. Real repo_url values are always "owner/repo"; reject anything flag-shaped instead
	// of passing it through.
	if strings.HasPrefix(repoURL, "-") {
		log.Debug(fmt.Sprintf("Refusing to run apm deps why for suspicious repo_url %q, defaulting to prod scope", repoURL))
		return []string{apmScopeProd}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), depsWhyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apm", "deps", "why", repoURL, "--json") // #nosec G204 -- repoURL is validated above to reject flag-shaped values; exec.Command never invokes a shell, so no injection vector remains
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug(fmt.Sprintf("apm deps why %s failed, defaulting to prod scope: %s", repoURL, err))
		return []string{apmScopeProd}, nil
	}
	return parseDepsWhyOutput(out, repoURL)
}

// parseDepsWhyOutput turns `apm deps why --json` output into a scope and requestedBy chains.
// Split out from resolveScopeAndRequestedBy so the parsing logic is testable without shelling
// out to a real apm binary.
func parseDepsWhyOutput(out []byte, repoURL string) (scopes []string, requestedBy [][]string) {
	var result apmDepsWhyResult
	if err := json.Unmarshal(out, &result); err != nil {
		log.Debug(fmt.Sprintf("could not parse apm deps why %s output, defaulting to prod scope: %s", repoURL, err))
		return []string{apmScopeProd}, nil
	}

	if result.Package.IsDirect {
		return []string{apmScopeProd}, nil
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
	return []string{apmScopeTransitive}, requestedBy
}
