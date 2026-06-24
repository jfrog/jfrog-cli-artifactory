package alpine

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jfrog/build-info-go/entities"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
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
	if apkCmd.branch == "" {
		apkCmd.branch = "main"
	}

	filename := filepath.Base(apkCmd.filePath)
	pkgName, pkgVersion, arch, err := parseApkFilename(filename, apkCmd.arch)
	if err != nil {
		return err
	}

	fileDetails, err := crypto.GetFileDetails(apkCmd.filePath, true)
	if err != nil {
		return errorutils.CheckErrorf("failed to compute checksums for %s: %w", apkCmd.filePath, err)
	}

	if apkCmd.serverDetails == nil {
		return errorutils.CheckErrorf("no JFrog server configured — run 'jf c add' first or pass --server-id")
	}

	username, password := resolveCredentials(apkCmd.serverDetails, apkCmd.username, apkCmd.password)

	rtURL := apkCmd.serverDetails.GetArtifactoryUrl()
	target := fmt.Sprintf("%s/%s/%s/%s", apkCmd.repoKey, apkCmd.alpineVersion, arch, filename)
	uploadURL := rtURL + target

	log.Info(fmt.Sprintf("Uploading %s → %s", filename, target))

	if err := apkCmd.uploadFile(uploadURL, username, password, fileDetails); err != nil {
		return err
	}
	log.Info("Upload successful.")

	if err := apkCmd.setProperties(target, pkgName, pkgVersion, arch); err != nil {
		log.Warn("Failed to set artifact properties:", err)
	}

	collectBuildInfo, err := apkCmd.buildConfiguration.IsCollectBuildInfo()
	if err != nil {
		return err
	}
	if collectBuildInfo {
		if err := apkCmd.recordBuildInfoArtifact(filename, pkgName, pkgVersion, arch, fileDetails.Checksum); err != nil {
			log.Warn("Build Info artifact recording failed:", err)
		}
	}
	return nil
}

// uploadFile PUTs the .apk file to Artifactory with checksum headers.
func (apkCmd *ApkUploadCommand) uploadFile(uploadURL, username, password string, fileDetails *crypto.FileDetails) error {
	f, err := os.Open(apkCmd.filePath)
	if err != nil {
		return errorutils.CheckErrorf("failed to open %s: %w", apkCmd.filePath, err)
	}
	defer func() { _ = f.Close() }()

	req, err := http.NewRequest(http.MethodPut, uploadURL, f)
	if err != nil {
		return errorutils.CheckErrorf("failed to build upload request: %w", err)
	}
	req.ContentLength = fileDetails.Size
	req.SetBasicAuth(username, password)
	req.Header.Set("X-Checksum-Sha1", fileDetails.Checksum.Sha1)
	req.Header.Set("X-Checksum-Md5", fileDetails.Checksum.Md5)
	req.Header.Set("X-Checksum", fileDetails.Checksum.Sha256)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errorutils.CheckErrorf("upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return errorutils.CheckErrorf("upload failed with HTTP %d: %s", resp.StatusCode, string(body))
}

// setProperties sets Alpine package properties on the uploaded artifact via a single SetProps call.
func (apkCmd *ApkUploadCommand) setProperties(target, pkgName, pkgVersion, arch string) error {
	servicesManager, err := artutils.CreateServiceManager(apkCmd.serverDetails, -1, 0, false)
	if err != nil {
		return errorutils.CheckErrorf("failed to create Artifactory service manager: %w", err)
	}

	propsStr := fmt.Sprintf("os.name=alpine;os.version=%s;os.arch=%s;apk.name=%s;apk.version=%s",
		apkCmd.alpineVersion, arch, pkgName, pkgVersion)

	if buildProps, _ := apkCmd.getBuildProps(); buildProps != "" {
		propsStr += ";" + buildProps
	}

	searchReader, err := servicesManager.SearchFiles(services.SearchParams{
		CommonParams: &specutils.CommonParams{Pattern: target},
	})
	if err != nil {
		return errorutils.CheckErrorf("failed to search for uploaded artifact: %w", err)
	}

	_, err = servicesManager.SetProps(services.PropsParams{
		Reader: searchReader,
		Props:  propsStr,
	})
	return err
}

// getBuildProps returns build.name/number/timestamp properties when build configuration is set.
func (apkCmd *ApkUploadCommand) getBuildProps() (string, error) {
	buildName, err := apkCmd.buildConfiguration.GetBuildName()
	if err != nil {
		return "", err
	}
	if buildName == "" {
		return "", nil
	}
	buildNumber, err := apkCmd.buildConfiguration.GetBuildNumber()
	if err != nil {
		return "", err
	}
	if buildNumber == "" {
		return "", nil
	}
	if err := buildUtils.SaveBuildGeneralDetails(buildName, buildNumber, apkCmd.buildConfiguration.GetProject()); err != nil {
		return "", err
	}
	return buildUtils.CreateBuildProperties(buildName, buildNumber, apkCmd.buildConfiguration.GetProject())
}

// recordBuildInfoArtifact saves the uploaded artifact (and its dependencies) to the local Build Info cache.
func (apkCmd *ApkUploadCommand) recordBuildInfoArtifact(filename, pkgName, pkgVersion, arch string, checksum crypto.Checksum) error {
	buildObj, err := buildUtils.PrepareBuildPrerequisites(apkCmd.buildConfiguration)
	if err != nil {
		return err
	}

	moduleID := apkCmd.buildConfiguration.GetModule()
	if moduleID == "" {
		moduleID = fmt.Sprintf("%s:alpine", apkCmd.repoKey)
	}

	httpAuth := ""
	if apkCmd.serverDetails != nil {
		u, p := resolveHTTPAuthCredentials(apkCmd.serverDetails, apkCmd.username, apkCmd.password)
		if u != "" && p != "" {
			if auth, authErr := buildHTTPAuth(apkCmd.serverDetails.GetArtifactoryUrl(), u, p); authErr == nil {
				httpAuth = auth
			}
		}
	}

	deps, err := collectApkDependencies(pkgName, apkCmd.filePath, httpAuth)
	if err != nil {
		log.Warn("Failed to collect APK dependencies for build info:", err)
	}

	module := entities.Module{
		Id:   moduleID,
		Type: entities.Alpine,
		Artifacts: []entities.Artifact{{
			Name: fmt.Sprintf("%s:%s:%s", pkgName, pkgVersion, arch),
			Path: fmt.Sprintf("%s/%s/%s/%s", apkCmd.repoKey, apkCmd.alpineVersion, arch, filename),
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

// parseApkFilename extracts name, version, and arch from an Alpine package filename.
// archOverride is applied when the filename does not contain an architecture field,
// as is the case for files downloaded with `apk fetch` (e.g. "zlib-1.3.2-r0.apk").
// If archOverride is also empty in that situation, an error is returned.
func parseApkFilename(filename, archOverride string) (name, version, arch string, err error) {
	if m := apkFilenamePattern.FindStringSubmatch(filename); m != nil {
		arch = m[3]
		if archOverride != "" {
			arch = archOverride
		}
		return m[1], m[2], arch, nil
	}
	if m := apkFilenameNoArchPattern.FindStringSubmatch(filename); m != nil {
		if archOverride == "" {
			return "", "", "", errorutils.CheckErrorf(
				"cannot determine architecture from filename %q (no arch suffix) — pass --arch <arch> explicitly, e.g. --arch aarch64",
				filename,
			)
		}
		return m[1], m[2], archOverride, nil
	}
	return "", "", "", errorutils.CheckErrorf(
		"cannot parse Alpine package filename %q — expected <name>-<ver>-<rel>.<arch>.apk or <name>-<ver>-<rel>.apk",
		filename,
	)
}

// collectApkDependencies returns the runtime dependencies for pkgName to include in build info.
// It first tries `apk info -a <pkgName>` (requires the package to be installed on the system).
// If that yields no results, it falls back to parsing the `depend =` lines from the embedded
// .PKGINFO metadata inside the .apk archive itself.
// httpAuth is the HTTP_AUTH env value injected into apk fetch subprocesses for private repos.
func collectApkDependencies(pkgName, filePath, httpAuth string) ([]entities.Dependency, error) {
	specs, err := depsFromApkInfoCommand(pkgName)
	if err != nil || len(specs) == 0 {
		log.Debug("apk info -a not available for", pkgName, "— falling back to .PKGINFO parsing:", err)
		specs, err = depsFromEmbeddedPkgInfo(filePath)
		if err != nil {
			return nil, err
		}
	}

	deps := make([]entities.Dependency, 0, len(specs))
	for _, spec := range specs {
		id, checksum, resolveErr := resolveDepInfo(spec, httpAuth)
		if resolveErr != nil {
			log.Debug("Could not resolve APK dependency", spec, "-", resolveErr)
			id = stripVersionConstraint(spec)
		}
		deps = append(deps, entities.Dependency{
			Id:   id,
			Type: "apk",
			Checksum: entities.Checksum{
				Sha1:   checksum.Sha1,
				Sha256: checksum.Sha256,
				Md5:    checksum.Md5,
			},
		})
	}
	return deps, nil
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
	return specs
}

// resolveDepInfo maps a raw dep spec (e.g. "musl>=1.2", "so:libc.musl-x86_64.so.1") to a
// fully qualified package ID (e.g. "musl-1.2.5-r3") and computes the SHA1/SHA256/MD5
// checksums of its .apk file by fetching it into a temp directory.
// httpAuth is injected as HTTP_AUTH into the apk fetch subprocess for private Artifactory repos.
func resolveDepInfo(spec, httpAuth string) (id string, checksum entities.Checksum, err error) {
	name := stripVersionConstraint(spec)

	out, err := exec.Command("apk", "info", name).Output()
	if err != nil {
		return "", entities.Checksum{}, fmt.Errorf("apk info %q: %w", name, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			id = strings.TrimSuffix(line, " description:")
			break
		}
	}
	if id == "" {
		return "", entities.Checksum{}, fmt.Errorf("empty output from apk info %q", name)
	}

	tmpDir, err := os.MkdirTemp("", "apk-dep-chk-*")
	if err != nil {
		return id, entities.Checksum{}, nil
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	fetchCmd := exec.Command("apk", "fetch", "--output", tmpDir, name)
	if httpAuth != "" {
		fetchCmd.Env = append(os.Environ(), "HTTP_AUTH="+httpAuth)
	}
	if fetchErr := fetchCmd.Run(); fetchErr != nil {
		log.Debug("apk fetch failed for dep", name, "-", fetchErr, "; checksum will be empty")
		return id, entities.Checksum{}, nil
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		return id, entities.Checksum{}, nil
	}

	fileDetails, err := crypto.GetFileDetails(filepath.Join(tmpDir, entries[0].Name()), true)
	if err != nil {
		return id, entities.Checksum{}, nil
	}

	return id, entities.Checksum{
		Sha1:   fileDetails.Checksum.Sha1,
		Sha256: fileDetails.Checksum.Sha256,
		Md5:    fileDetails.Checksum.Md5,
	}, nil
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
