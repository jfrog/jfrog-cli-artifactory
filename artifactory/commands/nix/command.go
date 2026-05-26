package nix

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	nixpkg "github.com/jfrog/build-info-go/flexpack/nix"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NixCommand wraps native Nix CLI tools with build-info support.
// Dispatches to the correct native tool based on nativeTool field:
//
//	nix-channel → passthrough, no build-info
//	nix-env    → run + collect deps from runtime closure
//	nix-build  → run + collect deps from output store path
//	copy       → run "nix copy" + set build properties + collect artifacts
type NixCommand struct {
	nativeTool         string // "nix-channel", "nix-env", "nix-build", "copy"
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
	repo               string
	netrcPath          string
	servicesManager    artifactory.ArtifactoryServicesManager
}

// Nix store + binary-cache layout used across this file.
const (
	// nixStorePathPrefix is the on-disk root for every store path produced by
	// Nix (`/nix/store/<hash>-<name>-<version>`). Hard-coded by Nix itself.
	nixStorePathPrefix = "/nix/store/"
	// nixResultSymlink / nixResultName are the conventional build-output
	// symlinks produced by `nix-build` (channels) and `nix build` (flakes).
	nixResultSymlink = "./result"
	nixResultName    = "result"
	// binaryCacheDirPrefix is the directory under an Artifactory Nix repo
	// where each store-path's artifacts (`*.nar.xz`, `*.narinfo`) live.
	binaryCacheDirPrefix = "binary-cache/"
)

// Artifact types reported in BuildInfo for files uploaded by `nix copy`.
const (
	artifactTypeNarXz   = "xz"
	artifactTypeNarinfo = "narinfo"
)

func NewNixCommand() *NixCommand {
	return &NixCommand{}
}

func (c *NixCommand) SetNativeTool(tool string) *NixCommand {
	c.nativeTool = tool
	return c
}

func (c *NixCommand) SetArgs(args []string) *NixCommand {
	c.args = args
	return c
}

func (c *NixCommand) SetServerDetails(serverDetails *config.ServerDetails) *NixCommand {
	c.serverDetails = serverDetails
	return c
}

func (c *NixCommand) SetBuildConfiguration(config *buildUtils.BuildConfiguration) *NixCommand {
	c.buildConfiguration = config
	return c
}

func (c *NixCommand) SetRepo(repo string) *NixCommand {
	c.repo = repo
	return c
}

func (c *NixCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// Set up auth (netrc) for Artifactory access and service manager
	if c.serverDetails != nil {
		if err := c.createNetrcFile(); err != nil {
			log.Warn("Nix authentication setup failed: " + err.Error())
		}
		defer func() {
			if c.netrcPath != "" {
				_ = os.Remove(c.netrcPath)
			}
		}()
		sm, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
		if err != nil {
			log.Warn("Could not create Artifactory service manager: " + err.Error())
		} else {
			c.servicesManager = sm
		}
	}

	switch c.nativeTool {
	case "nix-channel":
		return c.runNixChannel()
	case "nix-env":
		return c.runNixEnv()
	case "nix-build":
		return c.runNixBuild()
	case "build":
		return c.runNixFlakeBuild()
	case "copy":
		return c.runNixCopy()
	default:
		return c.runPassthrough()
	}
}

// runCommand spawns `name args...` with stdout/stderr piped to the user's
// terminal and the Nix-aware environment (netrc). Used by every sub-command
// that doesn't need to capture stdout itself. nix-build is the one exception
// because it reads store paths off stdout — it builds its own *exec.Cmd
// directly.
func (c *NixCommand) runCommand(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runNixChannel executes "nix-channel" with all args. No build-info.
func (c *NixCommand) runNixChannel() error {
	log.Info("Running nix-channel")
	if err := c.runCommand("nix-channel", c.args); err != nil {
		return fmt.Errorf("nix-channel failed: %w", err)
	}
	return nil
}

// runNixEnv executes "nix-env" with args, then collects build-info from runtime closure.
func (c *NixCommand) runNixEnv() error {
	log.Info("Running nix-env")
	if err := c.runCommand("nix-env", c.args); err != nil {
		return fmt.Errorf("nix-env failed: %w", err)
	}

	// Collect build-info: resolve store path from the installed package
	if c.buildConfiguration != nil {
		return c.collectDepsFromEnvArgs()
	}
	return nil
}

// runNixBuild executes "nix-build" with args, then collects build-info from output store path.
func (c *NixCommand) runNixBuild() error {
	log.Info("Running nix-build")
	cmd := exec.Command("nix-build", c.args...)
	cmd.Env = c.buildEnv()
	cmd.Stderr = os.Stderr // Show build progress to user
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nix-build failed: %w", err)
	}

	// nix-build prints only the output store path(s) to stdout, one per line
	storePaths := strings.Fields(strings.TrimSpace(string(output)))
	if len(storePaths) > 0 {
		log.Info(fmt.Sprintf("Built: %s", strings.Join(storePaths, ", ")))
	}

	// Collect build-info from the output store paths
	if c.buildConfiguration != nil && len(storePaths) > 0 {
		return c.collectBuildInfoFromStorePaths(storePaths)
	}
	return nil
}

// runNixFlakeBuild executes "nix build" (flake-style) with args, then collects build-info
// from the ./result symlink. Unlike nix-build which prints store paths to stdout,
// "nix build" creates a ./result symlink pointing to the output store path.
func (c *NixCommand) runNixFlakeBuild() error {
	log.Info("Running nix build")
	if err := c.runCommand("nix", append([]string{"build"}, c.args...)); err != nil {
		return fmt.Errorf("nix build failed: %w", err)
	}

	// "nix build" can create multiple output symlinks: ./result, ./result-bin, ./result-man, etc.
	var storePaths []string
	matches, _ := filepath.Glob(filepath.Join(c.workingDir, "result*"))
	for _, m := range matches {
		if target, err := os.Readlink(m); err == nil && strings.HasPrefix(target, nixStorePathPrefix) {
			storePaths = append(storePaths, target)
			log.Info(fmt.Sprintf("Built: %s → %s", filepath.Base(m), target))
		}
	}

	// Collect build-info from the output store paths
	if c.buildConfiguration != nil && len(storePaths) > 0 {
		return c.collectBuildInfoFromStorePaths(storePaths)
	}
	return nil
}

// runNixCopy executes "nix copy" with args, then sets build properties on uploaded artifacts.
func (c *NixCommand) runNixCopy() error {
	// Parse repo from --to URL for property tagging
	if c.repo == "" {
		c.repo = c.parseRepoFromToArg()
	}

	// If --to points to a virtual repo, resolve to its defaultDeploymentRepo.
	// This ensures artifacts upload to the LOCAL repo (not skip because remote-cache has them).
	// Also add --refresh to force re-check (Nix's internal cache may think it already uploaded).
	if c.repo != "" && c.serverDetails != nil {
		deployRepo := c.resolveDefaultDeploymentRepo(c.repo)
		if deployRepo != "" && deployRepo != c.repo {
			log.Info(fmt.Sprintf("Resolved default deployment repo: %s → %s", c.repo, deployRepo))
			c.replaceRepoInToArg(c.repo, deployRepo)
			c.repo = deployRepo
			// Add --refresh so nix re-checks the LOCAL repo (which is empty)
			// instead of using cached knowledge from previous virtual repo checks
			c.args = append([]string{"--refresh"}, c.args...)
		}
	}

	log.Info("Running nix copy")
	if err := c.runCommand("nix", append([]string{"copy"}, c.args...)); err != nil {
		return fmt.Errorf("nix copy failed: %w", err)
	}

	// Set build properties on uploaded artifacts
	if c.buildConfiguration != nil && c.repo != "" {
		return c.tagUploadedArtifacts()
	}
	return nil
}

// runPassthrough executes "nix <nativeTool>" for any unrecognized command.
func (c *NixCommand) runPassthrough() error {
	log.Info(fmt.Sprintf("Running nix %s", c.nativeTool))
	if err := c.runCommand("nix", append([]string{c.nativeTool}, c.args...)); err != nil {
		return fmt.Errorf("nix %s failed: %w", c.nativeTool, err)
	}
	return nil
}

// collectDepsFromEnvArgs resolves the store path from nix-env args.
// Supports channel-qualified attributes like "nixpkgs.hello" (channel.attr format).
// Plain package names (e.g., "hello") are not supported for build-info collection.
func (c *NixCommand) collectDepsFromEnvArgs() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	// Find the package attribute in args (last non-flag arg with ".", e.g., "nixpkgs.hello")
	var pkgAttr string
	for i := len(c.args) - 1; i >= 0; i-- {
		if !strings.HasPrefix(c.args[i], "-") && strings.Contains(c.args[i], ".") {
			pkgAttr = c.args[i]
			break
		}
	}
	if pkgAttr == "" {
		log.Warn("Build-info collection for nix-env requires a channel-qualified attribute (e.g., nixpkgs.hello). " +
			"Plain package names are not supported. No build-info was collected.")
		return nil
	}

	// Split "nixpkgs.hello" → channel="nixpkgs", attr="hello"
	parts := strings.SplitN(pkgAttr, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	// Resolve store path: nix-build '<channel>' -A attr --no-out-link
	cmd := exec.Command("nix-build", fmt.Sprintf("<%s>", parts[0]), "-A", parts[1], "--no-out-link")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn(fmt.Sprintf("Could not resolve store path for %s: %s", pkgAttr, string(output)))
		return err
	}

	storePaths := strings.Fields(strings.TrimSpace(string(output)))
	return c.collectBuildInfoFromStorePaths(storePaths)
}

// collectBuildInfoFromStorePaths collects the runtime closure via "nix path-info --json -r",
// resolves checksums via Artifactory AQL, then saves build-info locally.
// All dependency collection logic lives here — no external collector needed.
func (c *NixCommand) collectBuildInfoFromStorePaths(storePaths []string) error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Collecting build info for Nix project: %s/%s", buildName, buildNumber))

	deps, depGraph, err := collectRuntimeClosure(storePaths)
	if err != nil {
		log.Warn("Failed to collect runtime dependencies: " + err.Error())
	}

	buildInfo := buildNixBuildInfo(buildName, buildNumber, filepath.Base(c.workingDir), deps, depGraph)

	// Resolve checksums from Artifactory AQL — the .nar.xz file hash is what Artifactory stores.
	// narHash from nix path-info is a hash of the NAR byte stream, not the uploaded file, so it
	// cannot be used directly.
	if c.servicesManager != nil && len(buildInfo.Modules) > 0 {
		searchRepo := c.repo
		if searchRepo == "" {
			searchRepo = c.parseRepoFromSubstituter()
		}

		if searchRepo != "" {
			// Build the per-dep dir map once so we can query Artifactory in
			// ONE shot instead of two AQL calls per dependency.
			depDir := make(map[string]string, len(buildInfo.Modules[0].Dependencies))
			seen := make(map[string]bool, len(buildInfo.Modules[0].Dependencies))
			dirPaths := make([]string, 0, len(buildInfo.Modules[0].Dependencies))
			for _, dep := range buildInfo.Modules[0].Dependencies {
				storePath, ok := deps[dep.Id]
				if !ok {
					continue
				}
				dir := binaryCacheDirPrefix + nixpkg.ExtractStoreHash(storePath)
				depDir[dep.Id] = dir
				if !seen[dir] {
					seen[dir] = true
					dirPaths = append(dirPaths, dir)
				}
			}

			narXzFiles, err := c.searchClosureArtifacts(searchRepo, dirPaths, "*.nar.xz")
			if err != nil {
				log.Warn("Could not enrich dependency checksums from Artifactory: " + err.Error())
			}
			narXzByDir := firstByDir(narXzFiles)

			resolved := 0
			for i, dep := range buildInfo.Modules[0].Dependencies {
				dir, ok := depDir[dep.Id]
				if !ok {
					continue
				}
				f, ok := narXzByDir[dir]
				if !ok {
					continue
				}
				if f.Checksum.Sha1 != "" || f.Checksum.Sha256 != "" {
					buildInfo.Modules[0].Dependencies[i].Checksum = f.Checksum
					resolved++
				}
			}
			if resolved > 0 {
				log.Info(fmt.Sprintf("Resolved %d dep checksum(s) from Artifactory", resolved))
			}
		}
	}

	if c.buildConfiguration != nil {
		if moduleOverride := c.buildConfiguration.GetModule(); moduleOverride != "" && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Id = moduleOverride
		}
	}

	projectKey := ""
	if c.buildConfiguration != nil {
		projectKey = c.buildConfiguration.GetProject()
	}
	if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}

	log.Info(fmt.Sprintf("Nix build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// nixStorePathInfo mirrors the JSON output of "nix path-info --json -r" for a single path.
type nixStorePathInfo struct {
	NarHash    string   `json:"narHash"`
	NarSize    int64    `json:"narSize"`
	References []string `json:"references,omitempty"`
}

// collectRuntimeClosure runs "nix path-info --json --recursive" on the given store paths
// and returns:
//   - deps: map of depID → store path for every dependency (root paths excluded)
//   - depGraph: forward graph depID → []depID (built from References)
func collectRuntimeClosure(rootPaths []string) (deps map[string]string, depGraph map[string][]string, err error) {
	args := append([]string{"path-info", "--json", "--recursive"}, rootPaths...)
	output, err := exec.Command("nix", args...).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("nix path-info failed: %w", err)
	}

	var pathInfoMap map[string]nixStorePathInfo
	if err := json.Unmarshal(output, &pathInfoMap); err != nil {
		return nil, nil, fmt.Errorf("parse nix path-info output: %w", err)
	}

	rootIDs := make(map[string]bool, len(rootPaths))
	for _, storePath := range rootPaths {
		rootIDs[nixpkg.StorePathToDepID(storePath)] = true
	}

	depGraph = make(map[string][]string)
	for parentPath, info := range pathInfoMap {
		parentID := nixpkg.StorePathToDepID(parentPath)
		for _, refPath := range info.References {
			if refPath == parentPath {
				continue
			}
			depGraph[parentID] = append(depGraph[parentID], nixpkg.StorePathToDepID(refPath))
		}
	}

	deps = make(map[string]string)
	for storePath := range pathInfoMap {
		depID := nixpkg.StorePathToDepID(storePath)
		if rootIDs[depID] {
			continue
		}
		deps[depID] = storePath
	}

	log.Debug(fmt.Sprintf("Collected %d runtime dependencies from store closure", len(deps)))
	return deps, depGraph, nil
}

// buildNixBuildInfo assembles an entities.BuildInfo from collected Nix dependencies.
func buildNixBuildInfo(buildName, buildNumber, projectName string, deps map[string]string, depGraph map[string][]string) *entities.BuildInfo {
	requestedBy := make(map[string][]string)
	for parent, children := range depGraph {
		for _, child := range children {
			requestedBy[child] = append(requestedBy[child], parent)
		}
	}

	module := entities.Module{
		Id:   projectName,
		Type: entities.Nix,
	}

	for depID := range deps {
		entityDep := entities.Dependency{
			Id:     depID,
			Scopes: []string{"runtime"},
		}
		for _, parent := range requestedBy[depID] {
			entityDep.RequestedBy = append(entityDep.RequestedBy, []string{parent})
		}
		module.Dependencies = append(module.Dependencies, entityDep)
	}

	return &entities.BuildInfo{
		Name:    buildName,
		Number:  buildNumber,
		Started: time.Now().Format(entities.TimeFormat),
		Agent: &entities.Agent{
			Name:    "nix",
			Version: getNixVersion(),
		},
		BuildAgent: &entities.Agent{
			Name:    "Generic",
			Version: "1.0",
		},
		Modules: []entities.Module{module},
	}
}

// getNixVersion returns the installed nix version string (e.g. "2.24.12").
func getNixVersion() string {
	out, err := exec.Command("nix", "--version").Output()
	if err != nil {
		return "unknown"
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return strings.TrimSpace(string(out))
}

// tagUploadedArtifacts sets build properties on artifacts uploaded by nix copy.
func (c *NixCommand) tagUploadedArtifacts() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Tagging uploaded artifacts with build info: %s/%s", buildName, buildNumber))

	// Find the store path from args (./result or /nix/store/...)
	storePath := c.findStorePathFromArgs()
	if storePath == "" {
		return fmt.Errorf("no store path found in args")
	}

	// Get all store paths in the closure
	cmd := exec.Command("nix", "path-info", "--recursive", storePath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nix path-info failed: %w", err)
	}

	closurePaths := strings.Fields(strings.TrimSpace(string(output)))
	log.Info(fmt.Sprintf("Found %d store path(s) in closure", len(closurePaths)))

	// Pre-compute every binary-cache directory for the closure. Each store
	// path produces exactly one `binary-cache/<hash>` directory; the rest of
	// this function reads files out of those directories in three batched
	// queries (SetProps, *.nar.xz, *.narinfo) instead of the previous
	// four-queries-per-store-path loop.
	dirPaths := make([]string, 0, len(closurePaths))
	for _, storePath := range closurePaths {
		dirPaths = append(dirPaths, binaryCacheDirPrefix+nixpkg.ExtractStoreHash(storePath))
	}

	// 1. ONE SetProps for every file in every closure directory.
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if err := c.setBuildPropertiesBatch(c.repo, dirPaths, "*", props); err != nil {
		log.Warn("Failed to set build properties on uploaded artifacts: " + err.Error())
	}

	// 2. ONE AQL for every *.nar.xz in the closure, indexed by dir.
	narXzFiles, err := c.searchClosureArtifacts(c.repo, dirPaths, "*.nar.xz")
	if err != nil {
		log.Warn("Failed to enumerate .nar.xz artifacts: " + err.Error())
	}
	narXzByDir := firstByDir(narXzFiles)

	// 3. ONE AQL for every *.narinfo in the closure, indexed by dir.
	narinfoFiles, err := c.searchClosureArtifacts(c.repo, dirPaths, "*.narinfo")
	if err != nil {
		log.Warn("Failed to enumerate .narinfo artifacts: " + err.Error())
	}
	narinfoByDir := firstByDir(narinfoFiles)

	// Assemble artifacts in closure order so the BuildInfo output is stable.
	// Behavior preserved from the previous per-path loop:
	//   - nar.xz: appended ONLY when a matching file was found in Artifactory.
	//   - narinfo: appended for EVERY closure dir (the previous code also did
	//     this unconditionally — getArtifactChecksums silently returned an
	//     empty Checksum when the file was missing, but the artifact entry
	//     was still recorded with its computed name+path).
	var artifacts []entities.Artifact
	for _, storePath := range closurePaths {
		storeHash := nixpkg.ExtractStoreHash(storePath)
		dirPath := binaryCacheDirPrefix + storeHash

		if f, ok := narXzByDir[dirPath]; ok {
			artifacts = append(artifacts, entities.Artifact{
				Name:                   f.Name,
				Type:                   artifactTypeNarXz,
				Path:                   f.Path,
				OriginalDeploymentRepo: c.repo,
				Checksum:               f.Checksum,
			})
		}

		narinfoName := storeHash + ".narinfo"
		narinfoPath := dirPath + "/" + narinfoName
		narinfoChecksum := entities.Checksum{}
		if f, ok := narinfoByDir[dirPath]; ok {
			narinfoChecksum = f.Checksum
		}
		artifacts = append(artifacts, entities.Artifact{
			Name:                   narinfoName,
			Type:                   artifactTypeNarinfo,
			Path:                   narinfoPath,
			OriginalDeploymentRepo: c.repo,
			Checksum:               narinfoChecksum,
		})
	}

	if len(artifacts) > 0 {
		buildInfo := &entities.BuildInfo{
			Name:    buildName,
			Number:  buildNumber,
			Started: time.Now().Format(entities.TimeFormat),
			Agent: &entities.Agent{
				Name:    "build-info-go",
				Version: "1.0.0",
			},
			BuildAgent: &entities.Agent{
				Name:    "Nix",
				Version: getNixVersion(),
			},
			Modules: []entities.Module{
				{
					Id:        filepath.Base(c.workingDir),
					Type:      entities.Nix,
					Artifacts: artifacts,
				},
			},
		}

		// Apply --module override so nix copy's module ID matches
		// nix-env / nix-build's module ID. Without this, --module is
		// silently ignored on the artifact-tagging path and the
		// published build-info ends up with two separate modules.
		if c.buildConfiguration != nil {
			if moduleOverride := c.buildConfiguration.GetModule(); moduleOverride != "" {
				buildInfo.Modules[0].Id = moduleOverride
			}
		}

		projectKey := ""
		if c.buildConfiguration != nil {
			projectKey = c.buildConfiguration.GetProject()
		}
		if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
			return fmt.Errorf("failed to save build info: %w", err)
		}
		log.Info(fmt.Sprintf("Tagged %d artifact(s) with build properties", len(artifacts)))
	}

	return nil
}

// findStorePathFromArgs finds a store path or ./result from the command args.
func (c *NixCommand) findStorePathFromArgs() string {
	for _, arg := range c.args {
		if strings.HasPrefix(arg, nixStorePathPrefix) {
			return arg
		}
		if arg == nixResultSymlink || arg == nixResultName {
			resolved, err := os.Readlink(arg)
			if err == nil {
				return resolved
			}
		}
		// Check if it's a path that resolves to a store path
		if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "http") {
			resolved, err := filepath.Abs(arg)
			if err == nil {
				link, err := os.Readlink(resolved)
				if err == nil && strings.HasPrefix(link, nixStorePathPrefix) {
					return link
				}
			}
		}
	}
	return ""
}

// resolveDefaultDeploymentRepo queries Artifactory to find the defaultDeploymentRepo
// for a virtual repo. Returns empty if not virtual or no default deployment repo.
func (c *NixCommand) resolveDefaultDeploymentRepo(repoName string) string {
	if c.servicesManager == nil {
		return ""
	}
	repoDetails := &services.VirtualRepositoryBaseParams{}
	if err := c.servicesManager.GetRepository(repoName, repoDetails); err != nil {
		log.Debug(fmt.Sprintf("Could not determine type for repo '%s', using as-is: %s", repoName, err.Error()))
		return ""
	}
	if repoDetails.Rclass == services.VirtualRepositoryRepoType {
		if repoDetails.DefaultDeploymentRepo == "" {
			log.Warn(fmt.Sprintf("Virtual repository '%s' has no default deployment repository configured.", repoName))
			return ""
		}
		return repoDetails.DefaultDeploymentRepo
	}
	return ""
}

// replaceRepoInToArg replaces the repo name in the --to URL argument.
func (c *NixCommand) replaceRepoInToArg(oldRepo, newRepo string) {
	for i, arg := range c.args {
		if strings.Contains(arg, "/api/nix/"+oldRepo) {
			c.args[i] = strings.Replace(arg, "/api/nix/"+oldRepo, "/api/nix/"+newRepo, 1)
		}
		// Also check next arg if this is "--to"
		if arg == "--to" && i+1 < len(c.args) && strings.Contains(c.args[i+1], "/api/nix/"+oldRepo) {
			c.args[i+1] = strings.Replace(c.args[i+1], "/api/nix/"+oldRepo, "/api/nix/"+newRepo, 1)
		}
	}
}

// parseRepoFromSubstituter reads nix.conf and extracts the repo name from the substituter URL.
func (c *NixCommand) parseRepoFromSubstituter() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "nix", "nix.conf"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "substituters") || strings.HasPrefix(line, "extra-substituters") {
			if idx := strings.Index(line, "/api/nix/"); idx != -1 {
				repo := line[idx+len("/api/nix/"):]
				// Remove query params (?priority=1) and trailing whitespace
				if qIdx := strings.IndexAny(repo, "? \t#"); qIdx != -1 {
					repo = repo[:qIdx]
				}
				repo = strings.TrimSuffix(repo, "/")
				if repo != "" {
					return repo
				}
			}
		}
	}
	return ""
}

// parseRepoFromToArg extracts the repo name from --to URL.
func (c *NixCommand) parseRepoFromToArg() string {
	for i, arg := range c.args {
		var url string
		if arg == "--to" && i+1 < len(c.args) {
			url = c.args[i+1]
		} else if strings.HasPrefix(arg, "--to=") {
			url = strings.TrimPrefix(arg, "--to=")
		}
		if url != "" {
			if idx := strings.Index(url, "/api/nix/"); idx != -1 {
				repo := url[idx+len("/api/nix/"):]
				repo = strings.TrimSuffix(repo, "/")
				if repo != "" {
					log.Info(fmt.Sprintf("Parsed repo '%s' from --to URL", repo))
					return repo
				}
			}
		}
	}
	return ""
}

// createNetrcFile creates a temporary netrc file for Nix authentication.
func (c *NixCommand) createNetrcFile() error {
	user := c.serverDetails.User
	password := c.serverDetails.Password
	if password == "" {
		password = c.serverDetails.AccessToken
	}
	if user == "" || password == "" {
		return fmt.Errorf("no credentials configured (need user+password or user+access-token)")
	}

	// Use net/url so we never have to slice the URL string by hand
	// (the previous host[idx+3:] / host[:idx] pair worked, but slipping a
	// scheme-less or malformed URL through manual indexing is exactly the
	// class of bug to avoid). url.Parse handles ports, IPv6 brackets, and
	// userinfo correctly; falling back to the raw string is a no-op when
	// the URL is already just a bare host.
	host := c.serverDetails.ArtifactoryUrl
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		host = parsed.Host
	} else {
		host = strings.TrimSuffix(host, "/")
		if slash := strings.Index(host, "/"); slash != -1 {
			host = host[:slash]
		}
	}

	netrcContent := fmt.Sprintf("machine %s\nlogin %s\npassword %s\n", host, user, password)
	tmpFile, err := os.CreateTemp("", "nix-netrc-")
	if err != nil {
		return fmt.Errorf("create netrc temp file: %w", err)
	}
	if _, err = tmpFile.WriteString(netrcContent); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("write netrc file: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return fmt.Errorf("close netrc file: %w", err)
	}
	c.netrcPath = tmpFile.Name()
	return nil
}

// buildEnv returns the current environment with NIX_CONFIG for netrc auth.
func (c *NixCommand) buildEnv() []string {
	env := os.Environ()
	if c.netrcPath != "" {
		env = append(env, "NIX_CONFIG=netrc-file = "+c.netrcPath)
	}
	return env
}

// aqlFile is a flattened view of one AQL ResultItem.
type aqlFile struct {
	Name     string // file name (no directory)
	Dir      string // directory portion of the path within the repo (item.Path)
	Path     string // full path within the repo: Dir + "/" + Name (or just Name when Dir is ".")
	Checksum entities.Checksum
}

// buildClosureAql composes ONE AQL query that selects every file in `repo`
// whose `path` matches any of `dirPaths` and whose `name` matches `namePattern`.
// Duplicate dirPaths are removed so the resulting $or clause stays compact.
// Pre-condition: dirPaths is non-empty (callers check). The query shape mirrors
// the original per-dir search exactly (`{"path": "<dir>", "name": {"$match": "<pat>"}}`),
// just OR'd together so one round trip covers the whole closure.
func buildClosureAql(repo string, dirPaths []string, namePattern string) string {
	seen := make(map[string]bool, len(dirPaths))
	clauses := make([]string, 0, len(dirPaths))
	for _, dir := range dirPaths {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		clauses = append(clauses,
			fmt.Sprintf(`{"$and":[{"path":"%s"},{"name":{"$match":"%s"}}]}`, dir, namePattern))
	}
	return fmt.Sprintf(`{"repo":"%s","$or":[%s]}`, repo, strings.Join(clauses, ","))
}

// searchClosureArtifacts runs ONE AQL search across every (dirPath, namePattern)
// in `dirPaths` and returns the matching items. Replaces N per-directory
// findNarFile/getArtifactChecksums calls with a single network round trip.
// Returns nil when there are no dirs to query or no service manager configured.
func (c *NixCommand) searchClosureArtifacts(repo string, dirPaths []string, namePattern string) ([]aqlFile, error) {
	if c.servicesManager == nil || repo == "" || len(dirPaths) == 0 {
		return nil, nil
	}

	searchQuery := buildClosureAql(repo, dirPaths, namePattern)
	reader, err := c.servicesManager.SearchFiles(services.SearchParams{
		CommonParams: &specutils.CommonParams{Aql: specutils.Aql{ItemsFind: searchQuery}},
	})
	if err != nil {
		return nil, fmt.Errorf("AQL search in %s: %w", repo, err)
	}
	defer func() { _ = reader.Close() }()

	var items []aqlFile
	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		pathInRepo := item.Name
		if item.Path != "." {
			pathInRepo = item.Path + "/" + item.Name
		}
		items = append(items, aqlFile{
			Name: item.Name,
			Dir:  item.Path,
			Path: pathInRepo,
			Checksum: entities.Checksum{
				Sha1:   item.Actual_Sha1,
				Sha256: item.Sha256,
				Md5:    item.Actual_Md5,
			},
		})
	}
	return items, nil
}

// setBuildPropertiesBatch applies `props` to every file matching
// (repo, dirPaths, namePattern) in a single SearchFiles + SetProps round trip,
// replacing N per-directory SetProps calls.
func (c *NixCommand) setBuildPropertiesBatch(repo string, dirPaths []string, namePattern, props string) error {
	if c.servicesManager == nil || repo == "" || len(dirPaths) == 0 {
		return nil
	}

	searchQuery := buildClosureAql(repo, dirPaths, namePattern)
	reader, err := c.servicesManager.SearchFiles(services.SearchParams{
		CommonParams: &specutils.CommonParams{Aql: specutils.Aql{ItemsFind: searchQuery}},
	})
	if err != nil {
		return fmt.Errorf("AQL search for properties in %s: %w", repo, err)
	}
	defer func() { _ = reader.Close() }()

	if _, err := c.servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props}); err != nil {
		return fmt.Errorf("set props in %s: %w", repo, err)
	}
	return nil
}

// firstByDir indexes the AQL results by their directory, keeping the first
// hit per directory — preserves the original `findNarFile` semantics, which
// returned the first matching record only.
func firstByDir(files []aqlFile) map[string]aqlFile {
	out := make(map[string]aqlFile, len(files))
	for _, f := range files {
		if _, exists := out[f.Dir]; !exists {
			out[f.Dir] = f
		}
	}
	return out
}

func (c *NixCommand) getBuildNameAndNumber() (string, string, error) {
	if c.buildConfiguration == nil {
		return "", "", fmt.Errorf("no build configuration")
	}
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return "", "", fmt.Errorf("build name not configured")
	}
	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return "", "", fmt.Errorf("build number not configured")
	}
	return buildName, buildNumber, nil
}

func (c *NixCommand) CommandName() string { return "rt_nix" }

func (c *NixCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func saveBuildInfoLocally(buildInfo *entities.BuildInfo, projectKey string) error {
	service := buildUtils.CreateBuildInfoService()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	return nil
}
