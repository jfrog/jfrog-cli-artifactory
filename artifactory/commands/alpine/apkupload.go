package alpine

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	bibuild "github.com/jfrog/build-info-go/build"
	biUtils "github.com/jfrog/build-info-go/build/utils"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/crypto"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	uploadThreads                = 1
	uploadHTTPRetries            = 3
	uploadHTTPRetryWaitMilliSecs = 0
	// Negative httpRetries tells CreateServiceManager to keep the client default retry count.
	defaultHTTPRetries            = -1
	defaultHTTPRetryWaitMilliSecs = 0
)

var apkFilenamePattern = regexp.MustCompile(`^(.+)-([^-]+-r\d+)\.([^.]+)\.apk$`)

// apkFilenameNoArchPattern matches filenames produced by `apk fetch` which omit the architecture:
// e.g. "zlib-1.3.2-r0.apk" → name="zlib", version="1.3.2-r0"
var apkFilenameNoArchPattern = regexp.MustCompile(`^(.+)-([^-]+-r\d+)\.apk$`)

// ApkUploadCommand uploads a local .apk file to an Artifactory Alpine repository.
type ApkUploadCommand struct {
	commandName        string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	filePath           string
	repoKey            string
	alpineVersion      string
	branch             string
	arch               string
	username           string
	password           string
}

// NewApkUploadCommand constructs an ApkUploadCommand for the given local file path.
func NewApkUploadCommand(filePath string) *ApkUploadCommand {
	return &ApkUploadCommand{commandName: "apk-upload", filePath: filePath}
}

// SetServerDetails sets the Artifactory server config.
func (apkCmd *ApkUploadCommand) SetServerDetails(serverDetails *config.ServerDetails) *ApkUploadCommand {
	apkCmd.serverDetails = serverDetails
	return apkCmd
}

// SetBuildConfiguration sets the build configuration.
func (apkCmd *ApkUploadCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *ApkUploadCommand {
	apkCmd.buildConfiguration = bc
	return apkCmd
}

// SetRepo sets the Artifactory Alpine repository key.
func (apkCmd *ApkUploadCommand) SetRepo(repoKey string) *ApkUploadCommand {
	apkCmd.repoKey = repoKey
	return apkCmd
}

// SetAlpineVersion sets the Alpine release tag (e.g. "v3.20").
func (apkCmd *ApkUploadCommand) SetAlpineVersion(version string) *ApkUploadCommand {
	apkCmd.alpineVersion = version
	return apkCmd
}

// SetBranch sets the Alpine repository branch (main, community, edge).
func (apkCmd *ApkUploadCommand) SetBranch(branch string) *ApkUploadCommand {
	apkCmd.branch = branch
	return apkCmd
}

// SetArch overrides the architecture parsed from the filename.
func (apkCmd *ApkUploadCommand) SetArch(arch string) *ApkUploadCommand {
	apkCmd.arch = arch
	return apkCmd
}

// SetUsername sets the username CLI flag override.
func (apkCmd *ApkUploadCommand) SetUsername(username string) *ApkUploadCommand {
	apkCmd.username = username
	return apkCmd
}

// SetPassword sets the password CLI flag override.
func (apkCmd *ApkUploadCommand) SetPassword(password string) *ApkUploadCommand {
	apkCmd.password = password
	return apkCmd
}

// CommandName satisfies the Command interface.
func (apkCmd *ApkUploadCommand) CommandName() string { return apkCmd.commandName }

// ServerDetails satisfies the Command interface.
func (apkCmd *ApkUploadCommand) ServerDetails() (*config.ServerDetails, error) {
	return apkCmd.serverDetails, nil
}

// Run uploads the .apk file, sets artifact properties, and optionally records Build Info.
func (apkCmd *ApkUploadCommand) Run() error {
	// branch defaults to main when not provided.
	if apkCmd.branch == "" {
		apkCmd.branch = "main"
	}

	// --alpine-version is required — it defines part of the upload path
	// (<repo>/<alpine-version>/<branch>/<arch>/<file>) and cannot be inferred
	// from the filename (official Alpine .apk files carry no version in their name).
	// Resolve alpine version: --alpine-version flag > the running host's /etc/alpine-release.
	// The .apk carries no target Alpine version (neither the filename nor .PKGINFO record it),
	// so when the flag is omitted we fall back to the host release — with a warning, since the
	// host version may differ from the package's intended target (e.g. cross-release builds).
	if apkCmd.alpineVersion == "" {
		if sysVer := detectSystemAlpineVersion(); sysVer != "" {
			apkCmd.alpineVersion = sysVer
			log.Warn(fmt.Sprintf("No --alpine-version provided; falling back to the host's Alpine version %q from /etc/alpine-release. "+
				"Pass --alpine-version to be explicit — the host version may differ from the package's target.", sysVer))
		}
	}
	if apkCmd.alpineVersion == "" {
		return errorutils.CheckErrorf("--alpine-version is required (e.g. --alpine-version v3.21) — could not auto-detect it from /etc/alpine-release")
	}
	// Normalize to Alpine's canonical "v"-prefixed form (e.g. "3.21" -> "v3.21") so uploads land
	// under the same <version> segment that `jf setup apk` and native apk use. Without this, an
	// upload with "3.21" would sit at <repo>/3.21/... while apk reads from <repo>/v3.21/....
	if !strings.HasPrefix(apkCmd.alpineVersion, "v") {
		apkCmd.alpineVersion = "v" + apkCmd.alpineVersion
	}

	// Resolve arch with priority: --arch flag > embedded .PKGINFO arch > system arch.
	// The .PKGINFO value is the package's true target arch (correct even for cross-arch
	// uploads); the system arch is a last-resort fallback and may be wrong for cross-arch
	// or noarch packages, so we warn when using it.
	if apkCmd.arch == "" {
		if arch, archErr := archFromEmbeddedPkgInfo(apkCmd.filePath); archErr == nil && arch != "" {
			apkCmd.arch = arch
			log.Info(fmt.Sprintf("No --arch provided, detected arch %q from the package's .PKGINFO.", arch))
		} else if sysArch := detectSystemArch(); sysArch != "" {
			apkCmd.arch = sysArch
			log.Warn(fmt.Sprintf("No --arch provided and none found in .PKGINFO; falling back to system arch %q. "+
				"Pass --arch to be explicit — the system arch may be wrong for cross-arch or noarch packages.", sysArch))
		}
	}
	if apkCmd.arch == "" {
		return errorutils.CheckErrorf("--arch is required (e.g. --arch x86_64) — could not auto-detect it from the package's .PKGINFO or the system")
	}
	if err := validateArtifactoryPathSegment("alpine-version", apkCmd.alpineVersion); err != nil {
		return err
	}
	if err := validateArtifactoryPathSegment("branch", apkCmd.branch); err != nil {
		return err
	}
	if err := validateArtifactoryPathSegment("arch", apkCmd.arch); err != nil {
		return err
	}

	filename := filepath.Base(apkCmd.filePath)
	pkgName, pkgVersion, err := parseApkFilename(filename)
	if err != nil {
		return err
	}

	if apkCmd.serverDetails == nil {
		return errorutils.CheckErrorf("no JFrog server configured — run 'jf c add' first or pass --server-id")
	}
	if apkCmd.username != "" {
		apkCmd.serverDetails.SetUser(apkCmd.username)
	}
	if apkCmd.password != "" {
		apkCmd.serverDetails.SetPassword(apkCmd.password)
	}

	rtURL := apkCmd.serverDetails.GetArtifactoryUrl()

	// Resolve repo key with priority: --repo flag > /etc/apk/repositories
	repoFromFlag := apkCmd.repoKey != ""
	if apkCmd.repoKey == "" {
		apkCmd.repoKey = resolveRepoFromRepositoriesFile(rtURL)
		if apkCmd.repoKey != "" {
			log.Info(fmt.Sprintf("No --repo provided, resolved repo from /etc/apk/repositories: %s", apkCmd.repoKey))
		}
	}
	if apkCmd.repoKey == "" {
		return errorutils.CheckErrorf("--repo is required for upload (or run 'jf setup apk' to configure /etc/apk/repositories first)")
	}

	// An explicit --repo is hard user intent: fail fast if Artifactory does not have it,
	if repoFromFlag {
		if err := ensureRepoExists(apkCmd.repoKey, apkCmd.serverDetails); err != nil {
			return err
		}
	}

	// If the resolved repo is virtual, resolve it to its default deployment (local) repo.
	apkCmd.repoKey, err = resolveLocalUploadRepo(apkCmd.repoKey, apkCmd.serverDetails)
	if err != nil {
		return err
	}

	// Artifactory only indexes Alpine packages deployed under
	// <repo>/<alpine-version>/<branch>/<arch>/<file>. Omitting the branch segment
	// leaves the package outside every APKINDEX, so it is always included here.
	target := fmt.Sprintf("%s/%s/%s/%s/%s", apkCmd.repoKey, apkCmd.alpineVersion, apkCmd.branch, apkCmd.arch, filename)

	collectBuildInfo, err := apkCmd.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Uploading %s → %s", filename, target))
	if err := apkCmd.uploadWithArtifactory(target, pkgName, pkgVersion, collectBuildInfo); err != nil {
		return err
	}
	log.Info("Upload successful.")

	if collectBuildInfo {
		fileDetails, detailsErr := crypto.GetFileDetails(apkCmd.filePath, true)
		if detailsErr != nil {
			log.Warn("Build Info artifact recording failed: could not compute local checksums:", detailsErr)
		} else if err := apkCmd.recordBuildInfoArtifact(filename, pkgName, pkgVersion, apkCmd.arch, fileDetails.Checksum); err != nil {
			log.Warn("Build Info artifact recording failed:", err)
		}
	}
	return nil
}

func (apkCmd *ApkUploadCommand) uploadWithArtifactory(target, pkgName, pkgVersion string, collectBuildInfo bool) error {
	servicesManager, err := artutils.CreateUploadServiceManager(apkCmd.serverDetails, uploadThreads,
		uploadHTTPRetries, uploadHTTPRetryWaitMilliSecs, false, nil)
	if err != nil {
		return errorutils.CheckErrorf("failed to create Artifactory upload service: %w", err)
	}

	up := services.NewUploadParams()
	up.Pattern = apkCmd.filePath
	up.Target = target
	up.Flat = true

	alpineProps := fmt.Sprintf(
		"os.name=alpine;os.version=%s;os.arch=%s;apk.name=%s;apk.version=%s",
		apkCmd.alpineVersion, apkCmd.arch, pkgName, pkgVersion,
	)
	up.TargetProps, err = specutils.ParseProperties(alpineProps)
	if err != nil {
		return errorutils.CheckErrorf("failed to parse Alpine artifact properties: %w", err)
	}

	if collectBuildInfo {
		up.BuildProps, err = buildUtils.CreateBuildPropsFromConfiguration(apkCmd.buildConfiguration)
		if err != nil {
			return errorutils.CheckErrorf("failed to create build properties: %w", err)
		}
		summary, uploadErr := servicesManager.UploadFilesWithSummary(artifactory.UploadServiceOptions{}, up)
		if uploadErr != nil {
			return errorutils.CheckErrorf("failed to upload %s: %w", filepath.Base(apkCmd.filePath), uploadErr)
		}
		defer closeUploadSummaryReaders(summary)
		if summary.TotalFailed > 0 {
			return errorutils.CheckErrorf("failed to upload the Alpine package to Artifactory. See Artifactory logs for more details.")
		}
		if summary.TotalSucceeded < 1 {
			return errorutils.CheckErrorf("upload finished with 0 succeeded files for %s", filepath.Base(apkCmd.filePath))
		}
		return nil
	}

	totalUploaded, totalFailed, err := servicesManager.UploadFiles(artifactory.UploadServiceOptions{}, up)
	if err != nil {
		return errorutils.CheckErrorf("failed to upload %s: %w", filepath.Base(apkCmd.filePath), err)
	}
	if totalFailed > 0 {
		return errorutils.CheckErrorf("failed to upload the Alpine package to Artifactory. See Artifactory logs for more details.")
	}
	if totalUploaded < 1 {
		return errorutils.CheckErrorf("upload finished with 0 succeeded files for %s", filepath.Base(apkCmd.filePath))
	}
	return nil
}

func closeUploadSummaryReaders(summary *specutils.OperationSummary) {
	if summary == nil {
		return
	}
	if summary.TransferDetailsReader != nil {
		_ = summary.TransferDetailsReader.Close()
	}
	if summary.ArtifactsDetailsReader != nil {
		_ = summary.ArtifactsDetailsReader.Close()
	}
}

// recordBuildInfoArtifact saves the uploaded artifact (and its dependencies) to the local Build Info cache.
func (apkCmd *ApkUploadCommand) recordBuildInfoArtifact(filename, pkgName, pkgVersion, arch string, checksum crypto.Checksum) error {
	buildObj, err := buildUtils.PrepareBuildPrerequisites(apkCmd.buildConfiguration)
	if err != nil {
		return err
	}

	moduleID := alpineModuleID(apkCmd.buildConfiguration.GetModule(), apkCmd.repoKey, arch, apkCmd.alpineVersion)

	// Build the Artifactory path to use as requestedBy — matches the artifact path
	// recorded in the same module (including the <branch> segment) so reviewers can
	// trace dep → artifact directly.
	artifactoryPath := fmt.Sprintf("%s/%s/%s/%s/%s", apkCmd.repoKey, apkCmd.alpineVersion, apkCmd.branch, arch, filename)
	deps, err := apkCmd.collectApkDependencies(pkgName, apkCmd.filePath, artifactoryPath)
	if err != nil {
		log.Warn("Failed to collect APK dependencies for build info:", err)
	}

	module := entities.Module{
		Id:   moduleID,
		Type: entities.Apk,
		Artifacts: []entities.Artifact{{
			Name: fmt.Sprintf("%s:%s:%s", pkgName, pkgVersion, arch),
			Path: fmt.Sprintf("%s/%s/%s/%s/%s", apkCmd.repoKey, apkCmd.alpineVersion, apkCmd.branch, arch, filename),
			Checksum: entities.Checksum{
				Sha1:   checksum.Sha1,
				Sha256: checksum.Sha256,
				Md5:    checksum.Md5,
			},
		}},
		Dependencies: deps,
	}
	buildInfo := &entities.BuildInfo{Modules: []entities.Module{module}}
	return buildObj.SaveBuildInfo(buildInfo)
}

// parseApkFilename extracts name and version from an Alpine package filename.
// Both official Alpine naming (<name>-<version>-<release>.apk, no arch in filename)
// and the extended form (<name>-<version>-<release>.<arch>.apk) are accepted.
// Arch is always sourced from the --arch flag or auto-detection, never from the filename.
func parseApkFilename(filename string) (name, version string, err error) {
	if m := apkFilenamePattern.FindStringSubmatch(filename); m != nil {
		return m[1], m[2], nil
	}
	if m := apkFilenameNoArchPattern.FindStringSubmatch(filename); m != nil {
		return m[1], m[2], nil
	}
	return "", "", errorutils.CheckErrorf(
		"cannot parse Alpine package filename %q — expected <name>-<ver>-<rel>.apk or <name>-<ver>-<rel>.<arch>.apk",
		filename,
	)
}

// ensureRepoExists verifies that repoKey exists in Artifactory.
// Used when the user explicitly passed --repo so we fail before attempting upload.
func ensureRepoExists(repoKey string, serverDetails *config.ServerDetails) error {
	if serverDetails == nil {
		return errorutils.CheckErrorf(
			"cannot validate --repo '%s': no JFrog server configured — run 'jf c add' or pass --server-id",
			repoKey,
		)
	}
	servicesManager, err := artutils.CreateServiceManager(serverDetails, defaultHTTPRetries, defaultHTTPRetryWaitMilliSecs, false)
	if err != nil {
		return errorutils.CheckErrorf("failed to create Artifactory service manager to validate --repo '%s': %w", repoKey, err)
	}
	exists, err := servicesManager.IsRepoExists(repoKey)
	if err != nil {
		return errorutils.CheckErrorf("failed to validate --repo '%s': %w", repoKey, err)
	}
	if !exists {
		return errorutils.CheckErrorf("repository '%s' not found — check --repo or create the repository in Artifactory", repoKey)
	}
	return nil
}

// resolveLocalUploadRepo ensures the target repo for upload is a local repository.
// If repoKey is a virtual repository, it resolves to its DefaultDeploymentRepo.
// Returns an error if the virtual repo has no default deployment repo configured.
func resolveLocalUploadRepo(repoKey string, serverDetails *config.ServerDetails) (string, error) {
	servicesManager, err := artutils.CreateServiceManager(serverDetails, defaultHTTPRetries, defaultHTTPRetryWaitMilliSecs, false)
	if err != nil {
		log.Debug("Could not create services manager for repo type check, using repo as-is:", err)
		return repoKey, nil
	}
	repoDetails := &services.VirtualRepositoryBaseParams{}
	if err = servicesManager.GetRepository(repoKey, repoDetails); err != nil {
		log.Debug(fmt.Sprintf("Could not determine type for repo '%s', using as-is: %s", repoKey, err))
		return repoKey, nil
	}
	if repoDetails.Rclass == services.VirtualRepositoryRepoType {
		if repoDetails.DefaultDeploymentRepo == "" {
			return "", errorutils.CheckErrorf(
				"virtual repository '%s' has no default deployment repository configured — "+
					"set one in Artifactory UI or pass a local repository with --repo", repoKey)
		}
		log.Info(fmt.Sprintf("Resolved virtual repository '%s' → local repository '%s'.", repoKey, repoDetails.DefaultDeploymentRepo))
		return repoDetails.DefaultDeploymentRepo, nil
	}
	return repoKey, nil
}

// resolveRepoFromRepositoriesFile reads /etc/apk/repositories and returns the
// repo key of the first JFrog Artifactory entry whose URL contains rtURL.
func resolveRepoFromRepositoriesFile(rtURL string) string {
	data, err := os.ReadFile("/etc/apk/repositories")
	if err != nil {
		return ""
	}
	rtHost := strings.TrimRight(rtURL, "/")
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, rtHost) {
			continue
		}
		// URL format: https://rt.example.com/artifactory/<repoKey>/vX.YY/main
		rest := strings.TrimPrefix(line, rtHost+"/")
		// strip leading "artifactory/" if present
		rest = strings.TrimPrefix(rest, "artifactory/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

func (apkCmd *ApkUploadCommand) collectApkDependencies(pkgName, filePath, uploadedPkgID string) ([]entities.Dependency, error) {
	specs, err := depsFromApkInfoCommand(pkgName)
	if err != nil || len(specs) == 0 {
		log.Debug("apk info -a not available for", pkgName, "— falling back to .PKGINFO parsing:", err)
		specs, err = depsFromEmbeddedPkgInfo(filePath)
		if err != nil {
			return nil, err
		}
	}

	providers, installedByName := apkInstalledProviderIndex()

	deps := make([]entities.Dependency, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		id := resolveDepIDWithProviders(spec, providers, installedByName)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		var requestedBy [][]string
		if uploadedPkgID != "" {
			requestedBy = [][]string{{uploadedPkgID}}
		}
		deps = append(deps, entities.Dependency{
			Id:          id,
			Scopes:      []string{bibuild.AlpineScopeProd},
			RequestedBy: bibuild.FlattenRequestedBy(requestedBy),
		})
	}

	deps = enrichDepsFromLocalCache(deps)
	if apkCmd.serverDetails != nil && apkCmd.repoKey != "" {
		deps = apkCmd.enrichUploadDepsFromAQL(deps)
	}
	return deps, nil
}

func (apkCmd *ApkUploadCommand) enrichUploadDepsFromAQL(deps []entities.Dependency) []entities.Dependency {
	var missing []int
	for i, dep := range deps {
		if dep.Sha256 == "" && dep.Sha1 == "" {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return deps
	}
	sm, err := artutils.CreateServiceManager(apkCmd.serverDetails, defaultHTTPRetries, defaultHTTPRetryWaitMilliSecs, false)
	if err != nil {
		log.Debug("Could not create Artifactory service manager for upload AQL enrichment:", err)
		return deps
	}
	return enrichDepsChecksumsFromAQL(deps, missing, sm, apkCmd.repoKey)
}

func enrichDepsFromLocalCache(deps []entities.Dependency) []entities.Dependency {
	cacheDirs := []string{}
	if envCache := os.Getenv("APKCACHE"); envCache != "" {
		cacheDirs = append(cacheDirs, envCache)
	}
	cacheDirs = append(cacheDirs, "/var/cache/apk")

	for i, dep := range deps {
		if dep.Sha1 != "" || dep.Sha256 != "" {
			continue
		}
		pkg := alpinePackageFromDepID(dep.Id)
		for _, dir := range cacheDirs {
			checksums, err := biUtils.ChecksumsFromCache(pkg, dir)
			if err != nil || len(checksums) == 0 {
				continue
			}
			deps[i].Sha1 = checksums[crypto.SHA1]
			deps[i].Sha256 = checksums[crypto.SHA256]
			deps[i].Md5 = checksums[crypto.MD5]
			break
		}
	}
	return deps
}

func alpinePackageFromDepID(id string) biUtils.AlpinePackage {
	if isApkProviderToken(id) {
		return biUtils.AlpinePackage{Name: id}
	}
	if name, ver, ok := strings.Cut(id, ":"); ok && ver != "" {
		return biUtils.AlpinePackage{Name: name, Version: ver}
	}
	if m := apkFilenameNoArchPattern.FindStringSubmatch(id + ".apk"); m != nil {
		return biUtils.AlpinePackage{Name: m[1], Version: m[2]}
	}
	return biUtils.AlpinePackage{Name: id}
}

func isApkProviderToken(id string) bool {
	for _, prefix := range []string{"so:", "cmd:", "pc:"} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func validateArtifactoryPathSegment(flag, value string) error {
	if value == "" {
		return errorutils.CheckErrorf("%s must not be empty", flag)
	}
	if strings.ContainsAny(value, `/\`) || value == "." || value == ".." || strings.Contains(value, "..") {
		return errorutils.CheckErrorf("invalid %s %q: must not contain path separators or '..'", flag, value)
	}
	return nil
}

// depsFromApkInfoCommand runs `apk info -a <pkgName>` and parses the "depends on:" section.
// This works only when the package is installed on the local system.
func depsFromApkInfoCommand(pkgName string) ([]string, error) {
	out, err := exec.Command("apk", "info", "-a", pkgName).Output()
	if err != nil {
		return nil, fmt.Errorf("apk info -a %q: %w", pkgName, err)
	}
	return parseDependsSection(string(out)), nil
}

// parseDependsSection extracts dependency specs from the "depends on:" block
// produced by `apk info -a`.
//
// Example block:
//
//	zlib-1.3.1-r2 depends on:
//	so:libc.musl-x86_64.so.1
//	<blank line>
func parseDependsSection(output string) []string {
	var specs []string
	inSection := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, " depends on:") {
			inSection = true
			continue
		}
		if inSection {
			if trimmed == "" {
				break
			}
			specs = append(specs, trimmed)
		}
	}
	return specs
}

// depsFromEmbeddedPkgInfo opens the .apk archive and reads `depend = ...` lines
// from the .PKGINFO metadata file embedded in its first tar+gzip stream.
func depsFromEmbeddedPkgInfo(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader for %q: %w", filePath, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar from %q: %w", filePath, err)
		}
		if hdr.Name != ".PKGINFO" {
			continue
		}
		return parsePkgInfoDepends(tr), nil
	}
	return nil, nil
}

// parsePkgInfoDepends reads `depend = <spec>` lines from a .PKGINFO stream.
func parsePkgInfoDepends(r io.Reader) []string {
	var specs []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		after, found := strings.CutPrefix(line, "depend = ")
		if found {
			specs = append(specs, strings.TrimSpace(after))
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug("Failed reading .PKGINFO depend lines:", err)
	}
	return specs
}

// archFromEmbeddedPkgInfo opens the .apk archive and returns the value of the `arch = `
// line from the embedded .PKGINFO metadata — the package's true target architecture.
// Returns an empty string (and nil error) when the field is absent.
func archFromEmbeddedPkgInfo(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader for %q: %w", filePath, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar from %q: %w", filePath, err)
		}
		if hdr.Name != ".PKGINFO" {
			continue
		}
		return parsePkgInfoArch(tr), nil
	}
	return "", nil
}

// parsePkgInfoArch reads the `arch = <value>` line from a .PKGINFO stream.
func parsePkgInfoArch(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if after, found := strings.CutPrefix(scanner.Text(), "arch = "); found {
			return strings.TrimSpace(after)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug("Failed reading the .PKGINFO arch line:", err)
	}
	return ""
}

// detectSystemArch returns the Alpine architecture of the current system via
// `apk --print-arch`, which yields Alpine naming (e.g. x86_64, aarch64).
// Returns an empty string when apk is unavailable or the command fails.
func detectSystemArch() string {
	out, err := exec.Command("apk", "--print-arch").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectSystemAlpineVersion reads /etc/alpine-release and returns the major.minor version
// (e.g. "3.21", normalized later to "v3.21"). Returns "" when the file is absent or
// unparseable (i.e. not running on Alpine).
func detectSystemAlpineVersion() string {
	data, err := os.ReadFile("/etc/alpine-release")
	if err != nil {
		return ""
	}
	ver := strings.TrimSpace(string(data))
	ver = strings.TrimPrefix(ver, "v")
	parts := strings.Split(ver, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func resolveDepID(spec string) (string, error) {
	name := stripVersionConstraint(spec)

	out, err := exec.Command("apk", "info", name).Output()
	if err != nil {
		return "", fmt.Errorf("apk info %q: %w", name, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return strings.TrimSuffix(line, " description:"), nil
		}
	}
	return "", fmt.Errorf("empty output from apk info %q", name)
}

func apkInstalledProviderIndex() (providers map[string]string, byName map[string]biUtils.AlpinePackage) {
	pkgs, err := biUtils.ListInstalledPackages()
	if err != nil {
		log.Debug("Could not read the Alpine installed package database:", err)
		return nil, nil
	}
	byName = make(map[string]biUtils.AlpinePackage, len(pkgs))
	for _, pkg := range pkgs {
		byName[pkg.Name] = pkg
	}
	return biUtils.BuildProviderIndex(pkgs), byName
}

func resolveDepIDWithProviders(spec string, providers map[string]string, byName map[string]biUtils.AlpinePackage) string {
	token := stripVersionConstraint(spec)
	if name := biUtils.ResolveDependencyProvider(token, providers); name != "" {
		if pkg, ok := byName[name]; ok && pkg.Version != "" {
			return pkg.ID()
		}
		return name
	}
	id, err := resolveDepID(spec)
	if err != nil {
		log.Debug("Could not resolve APK dependency", spec, "-", err)
		return token
	}
	return id
}

// stripVersionConstraint removes trailing version constraints (>=, ~=, =, !=) from a dep spec,
// leaving just the package or provider name.
func stripVersionConstraint(spec string) string {
	for _, op := range []string{">=", "<=", "~=", "!=", ">", "<", "="} {
		if idx := strings.Index(spec, op); idx != -1 {
			return strings.TrimSpace(spec[:idx])
		}
	}
	return strings.TrimSpace(spec)
}
