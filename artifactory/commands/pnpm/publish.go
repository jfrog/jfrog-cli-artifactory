package pnpm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	artCliUtils "github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	artCoreUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const publishSummaryFile = "pnpm-publish-summary.json"

type PnpmPublishCommand struct {
	pnpmArgs           []string
	workingDirectory   string
	buildConfiguration *buildUtils.BuildConfiguration
	serverDetails      *config.ServerDetails
}

func NewPnpmPublishCommand() *PnpmPublishCommand {
	return &PnpmPublishCommand{}
}

func (ppc *PnpmPublishCommand) SetArgs(args []string) *PnpmPublishCommand {
	ppc.pnpmArgs = args
	return ppc
}

func (ppc *PnpmPublishCommand) SetBuildConfiguration(buildConfiguration *buildUtils.BuildConfiguration) *PnpmPublishCommand {
	ppc.buildConfiguration = buildConfiguration
	return ppc
}

func (ppc *PnpmPublishCommand) SetServerDetails(serverDetails *config.ServerDetails) *PnpmPublishCommand {
	ppc.serverDetails = serverDetails
	return ppc
}

func (ppc *PnpmPublishCommand) CommandName() string {
	return "rt_pnpm_publish"
}

func (ppc *PnpmPublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return ppc.serverDetails, nil
}

func (ppc *PnpmPublishCommand) Run() (err error) {
	log.Info("Running pnpm publish...")
	ppc.workingDirectory, err = coreutils.GetWorkingDirectory()
	if err != nil {
		return err
	}
	log.Debug("Working directory set to:", ppc.workingDirectory)

	collectBuildInfo, err := ppc.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}

	flags := extractPublishFlags(ppc.pnpmArgs)
	log.Debug(fmt.Sprintf("Publish flags - recursive: %v, dryRun: %v, userProvidedSummary: %v, filter args: %v, publish args: %v",
		flags.isRecursive, flags.isDryRun, flags.userProvidedSummary, flags.filterArgs, flags.publishArgs))

	if flags.isDryRun && collectBuildInfo {
		log.Warn("Dry-run mode detected. Build info will not be collected since no packages are actually published.")
		collectBuildInfo = false
	}

	if !collectBuildInfo {
		return ppc.runPnpmPublishNative(flags)
	}

	return ppc.publishWithBuildInfo(flags)
}

// publishWithBuildInfo publishes packages and collects build info.
//
// Two strategies are used depending on the publish mode:
//
//   - Single-project publish: pnpm delegates to npm, so --report-summary is not
//     supported. Instead, --json is added which makes npm output a JSON object to
//     stdout containing name, version, shasum, integrity, and filename.
//
//   - Workspace publish (-r): pnpm handles the publish natively and supports
//     --report-summary, which writes a pnpm-publish-summary.json file listing
//     all published packages. --json is NOT supported in this mode.
func (ppc *PnpmPublishCommand) publishWithBuildInfo(flags publishFlags) error {
	if flags.isRecursive {
		return ppc.publishWorkspaceWithBuildInfo(flags)
	}
	return ppc.publishSingleWithBuildInfo(flags)
}

// publishSingleWithBuildInfo handles single-project publish using --json to
// capture the published package info from npm's stdout output.
func (ppc *PnpmPublishCommand) publishSingleWithBuildInfo(flags publishFlags) error {
	if !flags.userProvidedJson {
		flags.publishArgs = append(flags.publishArgs, "--json")
	}

	stdout, err := ppc.runPnpmPublishCaptured(flags)
	if err != nil {
		return err
	}

	if err := ppc.collectSinglePublishBuildInfo(stdout); err != nil {
		log.Warn("pnpm publish completed successfully, but build info collection failed:", err.Error())
	}
	return nil
}

func (ppc *PnpmPublishCommand) collectSinglePublishBuildInfo(stdout []byte) error {
	published, err := parseNpmPublishJson(stdout)
	if err != nil {
		return err
	}
	if published == nil {
		log.Info("No package was published. Skipping build info collection.")
		return nil
	}

	packDir, err := os.MkdirTemp("", "jfrog-pnpm-pack-")
	if err != nil {
		return errorutils.CheckError(err)
	}
	defer func() {
		if removeErr := os.RemoveAll(packDir); removeErr != nil {
			log.Debug("Failed to cleanup pack directory:", removeErr.Error())
		}
	}()

	packages, err := ppc.packPublishedPackages(packDir, []publishedPackage{*published})
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		log.Warn("Could not pack the published package. Skipping build info.")
		return nil
	}

	checksumResults := computeChecksums(packages)
	publishRepos := getPublishConfigRepos(ppc.workingDirectory, []publishedPackage{*published})
	fallbackRepos := getRegistryRepos(ppc.workingDirectory)
	if err = ppc.saveBuildArtifacts(packages, checksumResults, []publishedPackage{*published}, publishRepos, fallbackRepos); err != nil {
		return err
	}

	ppc.tagBuildProperties([]publishedPackage{*published}, publishRepos, fallbackRepos)
	log.Info("pnpm publish finished successfully. 1 package published with build info.")
	return nil
}

// publishWorkspaceWithBuildInfo handles workspace (-r) publish using --report-summary
// to get the list of published packages from pnpm-publish-summary.json.
func (ppc *PnpmPublishCommand) publishWorkspaceWithBuildInfo(flags publishFlags) error {
	addedSummaryFlag := false
	if !flags.userProvidedSummary {
		flags.publishArgs = append(flags.publishArgs, "--report-summary")
		addedSummaryFlag = true
	}

	if err := ppc.runPnpmPublishNative(flags); err != nil {
		return err
	}

	if err := ppc.collectWorkspacePublishBuildInfo(addedSummaryFlag); err != nil {
		log.Warn("pnpm publish completed successfully, but build info collection failed:", err.Error())
	}
	return nil
}

func (ppc *PnpmPublishCommand) collectWorkspacePublishBuildInfo(addedSummaryFlag bool) error {
	summaryPath := filepath.Join(ppc.workingDirectory, publishSummaryFile)
	published, err := readPublishSummary(summaryPath)
	if err != nil {
		return err
	}
	if len(published) == 0 {
		if addedSummaryFlag {
			log.Debug("Cleaning up summary file:", summaryPath)
			if removeErr := os.Remove(summaryPath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Debug("Failed to cleanup summary file:", removeErr.Error())
			}
		}
		log.Info("No packages were published. Skipping build info collection.")
		return nil
	}
	log.Debug(fmt.Sprintf("Published %d package(s). Packing for checksum computation...", len(published)))

	packDir, err := os.MkdirTemp("", "jfrog-pnpm-pack-")
	if err != nil {
		return errorutils.CheckError(err)
	}
	defer func() {
		if removeErr := os.RemoveAll(packDir); removeErr != nil {
			log.Debug("Failed to cleanup pack directory:", removeErr.Error())
		}
	}()

	restoreSummary, err := prepareSummaryForPacking(summaryPath, addedSummaryFlag, packDir)
	if err != nil {
		return err
	}
	if restoreSummary != nil {
		defer restoreSummary()
	}

	packages, err := ppc.packPublishedPackages(packDir, published)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		log.Warn("Could not pack any published packages. Skipping build info.")
		return nil
	}

	checksumResults := computeChecksums(packages)
	log.Debug(fmt.Sprintf("Checksums computed for %d/%d package(s).", len(checksumResults), len(packages)))

	publishRepos := getPublishConfigRepos(ppc.workingDirectory, published)
	fallbackRepos := getRegistryRepos(ppc.workingDirectory)
	if err = ppc.saveBuildArtifacts(packages, checksumResults, published, publishRepos, fallbackRepos); err != nil {
		return err
	}

	ppc.tagBuildProperties(published, publishRepos, fallbackRepos)
	log.Info(fmt.Sprintf("pnpm publish finished successfully. %d package(s) published with build info.", len(packages)))
	return nil
}

// prepareSummaryForPacking ensures pnpm-publish-summary.json is not included in packed tarballs.
//
// If JFrog CLI added --report-summary, the summary file is removed before packing.
// If the user provided --report-summary, the file is copied into the tarballs temp directory,
// removed from the workspace before packing, and restored after packing.
func prepareSummaryForPacking(summaryPath string, addedSummaryFlag bool, tempDir string) (restore func(), err error) {
	_, statErr := os.Stat(summaryPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, nil
		}
		return nil, errorutils.CheckError(statErr)
	}

	log.Debug("Removing summary file before packing:", summaryPath)
	if addedSummaryFlag {
		if removeErr := os.Remove(summaryPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, errorutils.CheckError(removeErr)
		}
		return nil, nil
	}

	backupPath := filepath.Join(tempDir, filepath.Base(summaryPath))
	log.Debug("Moving summary file to temp directory before packing:", backupPath)
	if err = fileutils.MoveFile(summaryPath, backupPath); err != nil {
		return nil, err
	}

	return func() {
		log.Debug("Restoring summary file:", summaryPath)
		if err := fileutils.MoveFile(backupPath, summaryPath); err != nil {
			log.Debug("Failed to restore summary file:", err.Error())
		}
	}, nil
}

type publishFlags struct {
	publishArgs         []string
	filterArgs          []string
	isRecursive         bool
	isDryRun            bool
	userProvidedSummary bool
	userProvidedJson    bool
}

// extractPublishFlags separates -r/--recursive, --filter, --dry-run, --report-summary, and --json from publish args.
func extractPublishFlags(args []string) publishFlags {
	var flags publishFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-r" || arg == "--recursive":
			flags.isRecursive = true
		case arg == "--filter" && i+1 < len(args):
			flags.filterArgs = append(flags.filterArgs, "--filter", args[i+1])
			i++
		case strings.HasPrefix(arg, "--filter="):
			flags.filterArgs = append(flags.filterArgs, arg)
		case arg == "--dry-run":
			flags.isDryRun = true
			flags.publishArgs = append(flags.publishArgs, arg)
		case arg == "--report-summary":
			flags.userProvidedSummary = true
			flags.publishArgs = append(flags.publishArgs, arg)
		case arg == "--json":
			flags.userProvidedJson = true
			flags.publishArgs = append(flags.publishArgs, arg)
		default:
			flags.publishArgs = append(flags.publishArgs, arg)
		}
	}
	return flags
}

// runPnpmPublishNative runs pnpm publish as a native passthrough (stdout/stderr go to terminal).
func (ppc *PnpmPublishCommand) runPnpmPublishNative(flags publishFlags) error {
	args := []string{"publish"}
	if flags.isRecursive {
		args = append(args, "-r")
	}
	args = append(args, flags.filterArgs...)
	args = append(args, flags.publishArgs...)
	log.Debug("Running command: pnpm", strings.Join(args, " "))
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = ppc.workingDirectory
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errorutils.CheckError(cmd.Run())
}

// runPnpmPublishCaptured runs pnpm publish and captures stdout (used for --json output).
// Stderr still goes to the terminal so the user can see warnings/errors.
func (ppc *PnpmPublishCommand) runPnpmPublishCaptured(flags publishFlags) ([]byte, error) {
	args := []string{"publish"}
	args = append(args, flags.filterArgs...)
	args = append(args, flags.publishArgs...)
	log.Debug("Running command: pnpm", strings.Join(args, " "))
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = ppc.workingDirectory
	cmd.Stdin = os.Stdin
	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, errorutils.CheckError(err)
	}
	return stdoutBuf.Bytes(), nil
}

// npmPublishJsonOutput represents the JSON output from `pnpm publish --json`
// (which delegates to npm). Contains the published package details.
type npmPublishJsonOutput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Filename string `json:"filename"`
}

// parseNpmPublishJson parses the JSON output from `pnpm publish --json`.
func parseNpmPublishJson(data []byte) (*publishedPackage, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	var out npmPublishJsonOutput
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, errorutils.CheckErrorf("parsing pnpm publish --json output: %s", err.Error())
	}
	if out.Name == "" {
		return nil, nil
	}
	return &publishedPackage{Name: out.Name, Version: out.Version}, nil
}

type publishSummary struct {
	PublishedPackages []publishedPackage `json:"publishedPackages"`
}

type publishedPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// readPublishSummary reads and parses pnpm-publish-summary.json.
func readPublishSummary(path string) ([]publishedPackage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No publish summary file found at:", path)
			return nil, nil
		}
		return nil, errorutils.CheckError(err)
	}

	var summary publishSummary
	if err = json.Unmarshal(data, &summary); err != nil {
		return nil, errorutils.CheckErrorf("parsing publish summary: %s", err.Error())
	}

	for _, pkg := range summary.PublishedPackages {
		log.Debug(fmt.Sprintf("Published: %s@%s", pkg.Name, pkg.Version))
	}
	return summary.PublishedPackages, nil
}

// packPublishedPackages packs only the packages that were actually published,
// using --filter for each published package name.
func (ppc *PnpmPublishCommand) packPublishedPackages(destDir string, published []publishedPackage) ([]pnpmPackResult, error) {
	args := []string{"pack", "--json", "--pack-destination", destDir}
	for _, pkg := range published {
		args = append(args, "--filter", pkg.Name)
	}

	log.Debug("Running command: pnpm", strings.Join(args, " "))
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = ppc.workingDirectory
	out, err := cmd.Output()
	if err != nil {
		return nil, errorutils.CheckErrorf("pnpm pack failed: %s", err.Error())
	}
	log.Debug(fmt.Sprintf("pnpm pack output size: %d bytes", len(out)))

	results, err := parsePackOutput(out)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		log.Debug(fmt.Sprintf("Packed: %s@%s -> %s", r.Name, r.Version, r.Filename))
	}
	return results, nil
}

// pnpmPackResult represents one entry from `pnpm pack --json` output.
type pnpmPackResult struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Filename string `json:"filename"`
}

// parsePackOutput handles both single-object and array JSON from `pnpm pack --json`.
func parsePackOutput(data []byte) ([]pnpmPackResult, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	var results []pnpmPackResult
	if err := json.Unmarshal([]byte(trimmed), &results); err == nil {
		return results, nil
	}

	var single pnpmPackResult
	if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
		return nil, errorutils.CheckErrorf("parsing pnpm pack output: %s", err.Error())
	}
	return []pnpmPackResult{single}, nil
}

// computeChecksums computes checksums for all tarballs sequentially.
func computeChecksums(packages []pnpmPackResult) map[string]entities.Checksum {
	results := make(map[string]entities.Checksum, len(packages))
	for _, pkg := range packages {
		cs, err := computeFileChecksums(pkg.Filename)
		if err != nil {
			log.Debug("Failed to compute checksums for", pkg.Filename, ":", err.Error())
			continue
		}
		results[pkg.Filename] = cs
	}
	return results
}

func (ppc *PnpmPublishCommand) saveBuildArtifacts(packages []pnpmPackResult, checksumResults map[string]entities.Checksum, published []publishedPackage, publishRepos map[string]string, fallbackRepos registryMap) error {
	pnpmBuild, err := newBuild(ppc.buildConfiguration)
	if err != nil {
		return err
	}

	customModule := ppc.buildConfiguration.GetModule()

	// Build lookup from package name to published package for deploy path computation
	publishedMap := make(map[string]publishedPackage)
	for _, pub := range published {
		publishedMap[pub.Name] = pub
	}

	for _, pkg := range packages {
		cs, ok := checksumResults[pkg.Filename]
		if !ok {
			log.Warn("No checksums for", pkg.Name, "- skipping artifact.")
			continue
		}

		// Compute the Artifactory deploy path and target repo using the normalized
		// version from the published package (registry normalizes versions like v1.0.0 → 1.0.0).
		var artifactPath, artifactRepo string
		if pub, found := publishedMap[pkg.Name]; found {
			deployDir, deployName := buildPnpmDeployPath(pub.Name, pub.Version)
			artifactPath = deployDir + "/" + deployName
			artifactRepo = resolvePublishRepo(pub.Name, publishRepos, fallbackRepos)
		}

		artifact := entities.Artifact{
			Name:                   filepath.Base(pkg.Filename),
			Type:                   "tgz",
			Path:                   artifactPath,
			OriginalDeploymentRepo: artifactRepo,
			Checksum:               cs,
		}

		moduleID := formatModuleId(pkg.Name, pkg.Version)
		if moduleID == "" {
			moduleID = pkg.Name
		}
		if customModule != "" && len(packages) == 1 {
			moduleID = customModule
		}

		if err = pnpmBuild.AddArtifacts(moduleID, entities.Npm, artifact); err != nil {
			return err
		}
		log.Info(fmt.Sprintf("Build artifact recorded: %s:%s (sha1: %s)", pkg.Name, pkg.Version, cs.Sha1))
	}
	return nil
}

// tagBuildProperties sets build.name, build.number, build.timestamp on published
// artifacts in Artifactory. Constructs the artifact path directly from the package
// name/version and registry config, avoiding a SearchFiles API call.
func (ppc *PnpmPublishCommand) tagBuildProperties(published []publishedPackage, publishRepos map[string]string, fallbackRepos registryMap) {
	if ppc.serverDetails == nil {
		log.Debug("No server details configured. Skipping build property tagging.")
		return
	}
	servicesManager, err := artCoreUtils.CreateServiceManager(ppc.serverDetails, -1, 0, false)
	if err != nil {
		log.Warn("Unable to create service manager for build property tagging:", err.Error())
		return
	}

	props, err := buildUtils.CreateBuildPropsFromConfiguration(ppc.buildConfiguration)
	if err != nil {
		log.Warn("Unable to create build properties:", err.Error())
		return
	}
	if props == "" {
		log.Debug("No build properties to set. Skipping.")
		return
	}

	var items []specutils.ResultItem
	for _, pkg := range published {
		repo := resolvePublishRepo(pkg.Name, publishRepos, fallbackRepos)
		if repo == "" {
			log.Debug(fmt.Sprintf("Could not determine target repo for '%s'. Skipping property tagging for this package.", pkg.Name))
			continue
		}
		deployPath, tarballName := buildPnpmDeployPath(pkg.Name, pkg.Version)
		items = append(items, specutils.ResultItem{
			Repo: repo,
			Path: deployPath,
			Name: tarballName,
		})
	}

	if len(items) == 0 {
		log.Debug("No artifacts to tag with build properties.")
		return
	}

	pathToFile, err := artCliUtils.WriteResultItemsToFile(items)
	if err != nil {
		log.Warn("Unable to write result items for build property tagging:", err.Error())
		return
	}
	defer func() {
		if err := os.Remove(pathToFile); err != nil && !os.IsNotExist(err) {
			log.Debug("Failed to cleanup result items file:", err.Error())
		}
	}()

	reader := content.NewContentReader(pathToFile, content.DefaultKey)
	defer func() {
		if err := reader.Close(); err != nil {
			log.Debug("Failed to close reader:", err.Error())
		}
	}()

	_, err = servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true})
	if err != nil {
		log.Warn("Unable to set build properties on published artifacts:", err.Error(),
			"\nThis may cause the build to not properly link with artifacts. You can add build properties manually.")
		return
	}
	log.Info(fmt.Sprintf("Build properties set on %d published artifact(s).", len(items)))
}

// buildNpmDeployPath constructs the Artifactory path and tarball name for an npm package.
// Follows the standard npm registry layout:
//
//	unscoped: <name>/-/<name>-<version>.tgz
//	scoped:   @scope/<name>/-/@scope/<name>-<version>.tgz
func buildPnpmDeployPath(packageName, version string) (path, name string) {
	tarball := fmt.Sprintf("%s-%s.tgz", packageName, version)
	if strings.HasPrefix(packageName, "@") {
		return packageName + "/-", tarball
	}
	return packageName + "/-", tarball
}

// publishing in pnpm can be overridden per package by setting publishConfig.registry in package.json.
// getPublishConfigRepos reads publishConfig.registry from each published package's
// package.json via a single pnpm exec call with filters.
// Returns a map of package name -> Artifactory repo name.
func getPublishConfigRepos(workingDir string, published []publishedPackage) map[string]string {
	result := make(map[string]string)
	if len(published) == 0 {
		return result
	}

	args := []string{"--include-workspace-root"}
	for _, pkg := range published {
		args = append(args, "--filter", pkg.Name)
	}
	// Node 10+ compatible; outputs {"pkgName":"registryUrl"} per package
	script := `var p=require("./package.json");var r={};if(p.publishConfig&&p.publishConfig.registry){r[p.name]=p.publishConfig.registry}console.log(JSON.stringify(r))`
	args = append(args, "exec", "--", "node", "-e", script)

	log.Debug("Running command: pnpm", strings.Join(args, " "))
	cmd := exec.Command("pnpm", args...)
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug("Could not read publishConfig from workspace packages:", err.Error())
		return result
	}

	// pnpm exec runs the script per filtered package; each outputs a JSON object.
	// json.Unmarshal into an existing map merges keys, so we accumulate all entries.
	rawRepos := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line[0] == '{' {
			if err := json.Unmarshal([]byte(line), &rawRepos); err != nil {
				log.Debug("Could not parse publishConfig output:", line)
			}
		}
	}
	for name, registry := range rawRepos {
		if repo := extractRepoFromRegistryURL(registry); repo != "" {
			result[name] = repo
			log.Debug(fmt.Sprintf("Publish repo for '%s': %s (from publishConfig.registry)", name, repo))
		}
	}
	return result
}

func computeFileChecksums(filePath string) (entities.Checksum, error) {
	details, err := fileutils.GetFileDetails(filePath, true)
	if err != nil {
		return entities.Checksum{}, err
	}
	return details.Checksum, nil
}
