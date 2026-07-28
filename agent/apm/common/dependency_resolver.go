package apmcommon

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
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
func ResolveDependencies(lockfilePath string) ([]ResolvedDep, error) {
	lockfile, err := LoadLockFile(lockfilePath)
	if err != nil {
		return nil, err
	}

	workingDir := filepath.Dir(lockfilePath)

	var deps []ResolvedDep
	for _, pkg := range lockfile.RegistryPackages() {
		scopes, requestedBy := resolveScopeAndRequestedBy(workingDir, pkg.RepoURL)
		deps = append(deps, ResolvedDep{
			ID:          pkg.DepID(),
			RepoURL:     pkg.RepoURL,
			SHA256:      SHA256Hex(pkg.ResolvedHash),
			ResolvedURL: pkg.ResolvedURL,
			Scopes:      scopes,
			RequestedBy: requestedBy,
		})
	}
	return deps, nil
}

// ToEntitiesDependency converts a ResolvedDep to entities.Dependency with resolved checksums.
// Type is "zip" — confirmed live against Artifactory's real agentpackages storage layout.
func (d ResolvedDep) ToEntitiesDependency(cs entities.Checksum) entities.Dependency {
	return entities.Dependency{
		Id:          d.ID,
		Type:        "zip",
		Scopes:      d.Scopes,
		RequestedBy: d.RequestedBy,
		Checksum:    cs,
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
// runtime scope with no requestedBy rather than failing the whole build-info collection - the
// dependency's id/checksum are still correct either way.
func resolveScopeAndRequestedBy(workingDir, repoURL string) (scopes []string, requestedBy [][]string) {
	// repoURL comes from apm.lock.yaml, not a trusted CLI arg - a tampered lockfile could set
	// it to something starting with "-" to smuggle an extra flag into the apm invocation
	// below. Real repo_url values are always "owner/repo"; reject anything flag-shaped instead
	// of passing it through.
	if strings.HasPrefix(repoURL, "-") {
		log.Debug(fmt.Sprintf("Refusing to run apm deps why for suspicious repo_url %q, defaulting to runtime scope", repoURL))
		return []string{"runtime"}, nil
	}

	cmd := exec.Command("apm", "deps", "why", repoURL, "--json") // #nosec G204 -- repoURL is validated above to reject flag-shaped values; exec.Command never invokes a shell, so no injection vector remains
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug(fmt.Sprintf("apm deps why %s failed, defaulting to runtime scope: %s", repoURL, err))
		return []string{"runtime"}, nil
	}
	return parseDepsWhyOutput(out, repoURL)
}

// parseDepsWhyOutput turns `apm deps why --json` output into a scope and requestedBy chains.
// Split out from resolveScopeAndRequestedBy so the parsing logic is testable without shelling
// out to a real apm binary.
func parseDepsWhyOutput(out []byte, repoURL string) (scopes []string, requestedBy [][]string) {
	var result apmDepsWhyResult
	if err := json.Unmarshal(out, &result); err != nil {
		log.Debug(fmt.Sprintf("could not parse apm deps why %s output, defaulting to runtime scope: %s", repoURL, err))
		return []string{"runtime"}, nil
	}

	if result.Package.IsDirect {
		return []string{"runtime"}, nil
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
	return []string{"transitive"}, requestedBy
}
