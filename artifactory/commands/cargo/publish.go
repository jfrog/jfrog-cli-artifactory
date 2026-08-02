package cargo

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/build-info-go/entities"
	cargoflex "github.com/jfrog/build-info-go/flexpack/cargo"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// metadataBoolFlags are `cargo metadata`-valid boolean flags that affect resolution.
var metadataBoolFlags = map[string]bool{
	"--all-features":        true,
	"--no-default-features": true,
	"--locked":              true,
	"--frozen":              true,
	"--offline":             true,
}

// metadataValueFlags are `cargo metadata`-valid flags that take a value.
var metadataValueFlags = map[string]bool{
	"--features":      true,
	"--manifest-path": true,
}

// metadataFlagsFromArgs returns the subset of args that are valid for `cargo metadata`
// and affect dependency resolution, preserving order. Handles both "--flag value" and
// "--flag=value" forms for value flags.
func metadataFlagsFromArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case metadataBoolFlags[a]:
			out = append(out, a)
		case metadataValueFlags[a]:
			out = append(out, a)
			if i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--features=") || strings.HasPrefix(a, "--manifest-path="):
			out = append(out, a)
		}
	}
	return out
}

// buildNameNumber returns the configured build name and number (empty strings if unset).
func (c *CargoCommand) buildNameNumber() (string, string) {
	if c.buildConfiguration == nil {
		return "", ""
	}
	name, _ := c.buildConfiguration.GetBuildName()
	number, _ := c.buildConfiguration.GetBuildNumber()
	return name, number
}

// newCollector creates a Cargo FlexPack build-info collector for the working dir. Passes the
// user's -p/--package selectors through so build-info modules are limited to workspace members
// this invocation actually compiles (fix for the ripgrep/grep-index bug where sibling members
// showed up even though `cargo build -p X --features Y` never built them).
func (c *CargoCommand) newCollector() (*cargoflex.CargoFlexPack, error) {
	return cargoflex.NewCargoFlexPack(cargoflex.CargoConfig{
		WorkingDirectory:       c.workingDir,
		IncludeDevDependencies: false,
		MetadataArgs:           metadataFlagsFromArgs(c.args),
		SelectedPackages:       packageNamesFromArgs(c.args),
	})
}

// collectDeps collects dependency build-info and saves it locally.
func (c *CargoCommand) collectDeps() error {
	name, number := c.buildNameNumber()
	if name == "" || number == "" {
		log.Debug("cargo: --build-name and --build-number are both required; skipping build-info collection")
		return nil
	}
	collector, err := c.newCollector()
	if err != nil {
		return err
	}
	bi, err := collector.CollectBuildInfo(name, number)
	if err != nil {
		return err
	}
	c.enrichChecksums(bi)
	return c.saveBuildInfo(bi)
}

// collectArtifacts collects deps (module type cargo), determines the published crate artifact,
// applies build properties, and saves build-info. Used by the publish flow.
//
// `cargo publish` deletes the local .crate after uploading, so scanning target/package usually
// finds nothing; in that case the artifact is resolved from the repo it was just uploaded to via a
// single AQL call (local-first, else one Artifactory call — the artifact-collection rule).
func (c *CargoCommand) collectArtifacts() error {
	name, number := c.buildNameNumber()
	if name == "" || number == "" {
		log.Debug("cargo: --build-name and --build-number are both required; skipping artifact collection")
		return nil
	}
	collector, err := c.newCollector()
	if err != nil {
		return err
	}
	bi, err := collector.CollectBuildInfo(name, number) // deps first (module type = cargo)
	if err != nil {
		return err
	}

	// A dry-run publish uploads nothing, so there is no artifact to scan for and no repo item to
	// stamp build properties on. Record the dependency build-info only (skip artifact collection to
	// avoid spurious "not found"/set-properties errors).
	if isDryRunPublish(c.args) {
		c.enrichChecksums(bi)
		log.Info("cargo: publish --dry-run — nothing uploaded; recording dependencies only")
		return c.saveBuildInfo(bi)
	}

	repo, err := c.targetRepo()
	if err != nil {
		log.Debug("cargo: could not determine target repo: " + err.Error())
	}
	arts, err := scanCrateArtifacts(c.workingDir, repo)
	if err != nil {
		return err
	}
	// Prepare the published-crate filename up front so we can fold its checksum lookup into the
	// same AQL batch as the dep-checksum enrichment (Naveen: avoid the extra AQL query). When
	// scanCrateArtifacts already found the local .crate (checksums computed from the file), the
	// artifact list is complete and only dep enrichment is needed.
	publishedFileName := ""
	if len(arts) == 0 && repo != "" {
		publishedFileName = publishedCrateFileName(bi, c.args)
	}
	byName := c.enrichChecksumsAndFetch(bi, repo, publishedFileName)
	if publishedFileName != "" {
		if art, ok := c.assemblePublishedArtifact(publishedFileName, repo, byName); ok {
			arts = append(arts, art)
		}
	}
	// Route each artifact to the module of the member it was published from (workspace); for a
	// single-crate project this is the sole module. Falls back to module 0 if no id matches.
	for _, art := range arts {
		idx := moduleIndexForCrate(bi, art.Name)
		if idx >= 0 && idx < len(bi.Modules) {
			bi.Modules[idx].Artifacts = append(bi.Modules[idx].Artifacts, art)
		}
	}
	if repo != "" && len(arts) > 0 {
		if err := c.setBuildProperties(arts, repo, name, number); err != nil {
			log.Warn("cargo: failed to set build properties: " + err.Error())
		}
	}
	return c.saveBuildInfo(bi)
}

// assemblePublishedArtifact builds the entities.Artifact for the just-published crate using the
// checksum map already returned by enrichChecksumsAndFetch — no extra AQL round-trip. When the
// map does not include the file (e.g. no server details, no repo, or the index has not yet caught
// up with the upload), the artifact is returned without checksums; callers still record it so the
// build-info reflects what was published.
func (c *CargoCommand) assemblePublishedArtifact(fileName, repo string, byName map[string]entities.Checksum) (entities.Artifact, bool) {
	if fileName == "" {
		log.Debug("cargo: could not derive published crate name from build-info")
		return entities.Artifact{}, false
	}
	repoPath, _, _ := crateRepoPath(fileName)
	art := entities.Artifact{Name: fileName, Path: repoPath, Type: "crate", OriginalDeploymentRepo: repo}
	if cs, ok := byName[fileName]; ok {
		art.Checksum = cs
		log.Debug("cargo: resolved published artifact " + fileName + " checksums from Artifactory (batched AQL)")
	} else if c.serverDetails != nil && repo != "" {
		log.Warn("cargo: published crate " + fileName + " not found in repo " + repo + " yet; checksums omitted")
	}
	return art, true
}

// splitModuleId splits a "name:version" build-info module id. Returns (name, "") when there is
// no parseable version (e.g. the "cargo-project" placeholder).
func splitModuleId(id string) (name, version string) {
	i := strings.LastIndex(id, ":")
	if i <= 0 || i >= len(id)-1 {
		return id, ""
	}
	return id[:i], id[i+1:]
}

// crateFileForModule returns "<name>-<version>.crate" for a module, or "" if the id lacks a version.
func crateFileForModule(id string) string {
	name, version := splitModuleId(id)
	if name == "" || version == "" {
		return ""
	}
	return name + "-" + version + ".crate"
}

// isDryRunPublish reports whether a publish invocation is a dry run (`--dry-run`/`-n`). A dry run
// builds and verifies but uploads nothing, so there is no artifact to record and no repo item to
// stamp build properties on.
func isDryRunPublish(args []string) bool {
	for _, a := range args {
		if a == "--dry-run" || a == "-n" {
			return true
		}
	}
	return false
}

// packageNameFromArgs extracts the value of cargo's -p/--package selector (space or = form).
// Returns the first occurrence; use packageNamesFromArgs when cargo commands may specify -p
// multiple times to select several workspace members at once.
func packageNameFromArgs(args []string) string {
	names := packageNamesFromArgs(args)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// packageNamesFromArgs collects all -p/--package selectors on the cargo command line, in order.
// Cargo permits repeated -p to narrow a build to several workspace members (e.g. `cargo build
// -p a -p b`); build-info collection needs the full list so it emits one module per selected
// member — not just the first one and not all workspace members.
func packageNamesFromArgs(args []string) []string {
	var names []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case (a == "-p" || a == "--package") && i+1 < len(args):
			names = append(names, args[i+1])
			i++
		case strings.HasPrefix(a, "--package="):
			names = append(names, strings.TrimPrefix(a, "--package="))
		case strings.HasPrefix(a, "-p="):
			names = append(names, strings.TrimPrefix(a, "-p="))
		}
	}
	return names
}

// publishedCrateFileName derives the "<name>-<version>.crate" of the crate a publish uploaded.
// When -p/--package selects a workspace member, it matches that member's module (to pick up its
// version); otherwise, for a single-module (single-crate) build it uses the sole module. Returns
// "" when the target is ambiguous (multi-module workspace with no -p) or the id has no version.
func publishedCrateFileName(bi *entities.BuildInfo, args []string) string {
	if bi == nil || len(bi.Modules) == 0 {
		return ""
	}
	if pkg := packageNameFromArgs(args); pkg != "" {
		for _, m := range bi.Modules {
			if name, version := splitModuleId(m.Id); name == pkg && version != "" {
				return name + "-" + version + ".crate"
			}
		}
		return ""
	}
	if len(bi.Modules) == 1 {
		return crateFileForModule(bi.Modules[0].Id)
	}
	return ""
}

// moduleIndexForCrate returns the index of the module whose id maps to the given crate filename,
// or 0 when none matches (single-module fallback).
func moduleIndexForCrate(bi *entities.BuildInfo, crateFile string) int {
	if bi == nil {
		return 0
	}
	for i, m := range bi.Modules {
		if crateFileForModule(m.Id) == crateFile && crateFile != "" {
			return i
		}
	}
	return 0
}

// targetRepo determines and virtual-resolves the deployment repo from the
// registry index URL configured in .cargo/config.toml.
func (c *CargoCommand) targetRepo() (string, error) {
	regName := registryNameFromArgs(c.args)
	indexURL := cargoRegistryIndexURL(c.workingDir, regName)
	repo := extractRepoNameFromURL(indexURL)
	if repo == "" || c.serverDetails == nil {
		return repo, nil
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return "", err
	}
	return resolveDeploymentRepo(repo, sm), nil
}

// setBuildProperties sets build.name/number/timestamp on the uploaded artifacts in
// a single batched SetProps call (mirrors conan's BuildPropertySetter).
func (c *CargoCommand) setBuildProperties(arts []entities.Artifact, repo, name, number string) error {
	if len(arts) == 0 || c.serverDetails == nil {
		return nil
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager: %w", err)
	}

	writer, err := content.NewContentWriter(content.DefaultKey, true, false)
	if err != nil {
		return err
	}
	for _, a := range arts {
		artifactPath := a.Path
		if strings.HasSuffix(a.Path, a.Name) {
			artifactPath = dirOf(a.Path)
		}
		writer.Write(specutils.ResultItem{
			Repo:        repo,
			Path:        artifactPath,
			Name:        a.Name,
			Actual_Sha1: a.Sha1,
			Actual_Md5:  a.Md5,
			Sha256:      a.Sha256,
		})
	}
	if err := writer.Close(); err != nil {
		return err
	}

	reader := content.NewContentReader(writer.GetFilePath(), content.DefaultKey)
	defer func() {
		if err := reader.Close(); err != nil {
			log.Debug("cargo: failed to close reader: " + err.Error())
		}
	}()

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", name, number, timestamp)
	if c.buildConfiguration != nil {
		if projectKey := c.buildConfiguration.GetProject(); projectKey != "" {
			props += fmt.Sprintf(";build.project=%s", projectKey)
		}
	}

	_, err = sm.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true, IsRecursive: true})
	if err != nil {
		return fmt.Errorf("set properties: %w", err)
	}
	log.Info(fmt.Sprintf("cargo: set build properties on %d artifacts (batch)", len(arts)))
	return nil
}

// enrichChecksums fills any dependency checksums missing after local-cache resolution
// by querying Artifactory. Best-effort: logs and continues on any error.
func (c *CargoCommand) enrichChecksums(bi *entities.BuildInfo) {
	c.enrichChecksumsAndFetch(bi, "", "")
}

// enrichChecksumsAndFetch is the publish-flow variant: same batched Artifactory query that
// enrichChecksums performs, plus the just-published crate's file name folded into the same AQL
// so its checksums arrive in the same round-trip. When repo is "" the target repo is resolved
// via c.targetRepo(); pass it explicitly (already resolved) to avoid re-computing. Returns the
// checksum map so the caller can pick out the extra names; safe to ignore when there are none.
func (c *CargoCommand) enrichChecksumsAndFetch(bi *entities.BuildInfo, resolvedRepo, extraName string) map[string]entities.Checksum {
	if c.serverDetails == nil {
		return nil
	}
	repo := resolvedRepo
	if repo == "" {
		r, err := c.targetRepo()
		if err != nil {
			log.Debug("cargo: no target repo for checksum enrichment; skipping")
			return nil
		}
		repo = r
	}
	if repo == "" {
		log.Debug("cargo: no target repo for checksum enrichment; skipping")
		return nil
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("cargo: could not create service manager for checksum enrichment: " + err.Error())
		return nil
	}
	var extras []string
	if extraName != "" {
		extras = []string{extraName}
	}
	byName, err := enrichAndLookup(bi, repo, sm, extras)
	if err != nil {
		log.Warn("cargo: checksum enrichment failed: " + err.Error())
	}
	return byName
}

// moduleOverrideIndex selects which module a --module override should rename: the module of the
// -p/--package-selected member when present (so a workspace `publish -p X --module Y` renames X's
// module, keeping its routed artifact), else the sole/first module (single-crate case; mirrors the
// nix/go convention of overriding the primary module).
func moduleOverrideIndex(bi *entities.BuildInfo, args []string) int {
	if bi == nil || len(bi.Modules) == 0 {
		return 0
	}
	if pkg := packageNameFromArgs(args); pkg != "" {
		for i, m := range bi.Modules {
			if name, _ := splitModuleId(m.Id); name == pkg {
				return i
			}
		}
	}
	return 0
}

// applyModuleOverride renames the module at idx to moduleName when --module is provided,
// mirroring the nix/go convention (nix/command.go, golang/go.go) of honoring an explicit module
// name. No-op when the name is empty or idx is out of range.
//
// The build-info convention (verified in build-info-go golang_test.go / yarn_test.go) is that every
// dependency's requestedBy path TERMINATES at the module id — it anchors each path back to the build
// module. Renaming the module therefore also rewrites those terminal ids (which equal the previous
// module id) so the "requestedBy terminal == module.Id" invariant is preserved.
func applyModuleOverride(bi *entities.BuildInfo, moduleName string, idx int) {
	if moduleName == "" || bi == nil || idx < 0 || idx >= len(bi.Modules) {
		return
	}
	oldId := bi.Modules[idx].Id
	bi.Modules[idx].Id = moduleName
	if oldId == "" || oldId == moduleName {
		return
	}
	for di := range bi.Modules[idx].Dependencies {
		for _, path := range bi.Modules[idx].Dependencies[di].RequestedBy {
			for ei := range path {
				if path[ei] == oldId {
					path[ei] = moduleName
				}
			}
		}
	}
}

// saveBuildInfo persists the collected build-info locally for later publishing.
// Mirrors conan's saveBuildInfo (commands/conan/upload.go).
func (c *CargoCommand) saveBuildInfo(buildInfo *entities.BuildInfo) error {
	if c.buildConfiguration != nil {
		applyModuleOverride(buildInfo, c.buildConfiguration.GetModule(), moduleOverrideIndex(buildInfo, c.args))
	}

	service := buildUtils.CreateBuildInfoService()

	var projectKey string
	if c.buildConfiguration != nil {
		projectKey = c.buildConfiguration.GetProject()
	}
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	log.Info("cargo build info saved locally")
	return nil
}

// cargoConfigToml is the subset of .cargo/config.toml we parse.
type cargoConfigToml struct {
	Registries map[string]struct {
		Index string `toml:"index"`
	} `toml:"registries"`
}

// parseCargoRegistries returns registry name -> index URL, merging cargo's config sources the way
// cargo resolves them: the user-global $CARGO_HOME/config.toml (default ~/.cargo/config.toml) — this
// is what `jf setup cargo` writes — as a base, overlaid by the project-local
// <workingDir>/.cargo/config.toml (project entries win). This lets `jf cargo` locate registries
// whether they came from `jf setup cargo` (global) or a project-committed .cargo/config.toml.
func parseCargoRegistries(workingDir string) map[string]string {
	out := map[string]string{}
	// Global (lowest precedence) — written by `jf setup cargo`.
	if home, err := cargoHome(); err == nil && home != "" {
		readRegistriesInto(filepath.Join(home, "config.toml"), out)
	}
	// Project-local (highest precedence) overlays the global entries.
	readRegistriesInto(filepath.Join(workingDir, ".cargo", "config.toml"), out)
	return out
}

// readRegistriesInto parses one cargo config.toml and merges its [registries.<name>] index URLs
// into out (existing keys are overwritten). Missing/invalid files are skipped (debug-logged).
func readRegistriesInto(configPath string, out map[string]string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Debug("cargo: could not read " + configPath + ": " + err.Error())
		return
	}
	var cfg cargoConfigToml
	if err := toml.Unmarshal(data, &cfg); err != nil {
		log.Debug("cargo: could not parse " + configPath + ": " + err.Error())
		return
	}
	for name, reg := range cfg.Registries {
		if reg.Index != "" {
			out[name] = reg.Index
		}
	}
}

// cargoRegistryIndexURL reads <workingDir>/.cargo/config.toml and returns the
// index URL of [registries.<registryName>]. Returns "" on any error or if absent.
func cargoRegistryIndexURL(workingDir, registryName string) string {
	if registryName == "" {
		return ""
	}
	return parseCargoRegistries(workingDir)[registryName]
}

// dirOf returns the directory portion of a forward-slash repo path.
func dirOf(p string) string { return path.Dir(p) }
