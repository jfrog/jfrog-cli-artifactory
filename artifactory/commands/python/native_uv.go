package python

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NativeUvCommand runs `uv` directly (no config file required) and collects build info.
type NativeUvCommand struct {
	commandName        string
	args               []string
	serverID           string
	deployerRepo       string
	buildConfiguration *buildUtils.BuildConfiguration
}

// NewNativeUvCommand creates a new NativeUvCommand instance.
func NewNativeUvCommand() *NativeUvCommand {
	return &NativeUvCommand{}
}

func (c *NativeUvCommand) SetCommandName(name string) *NativeUvCommand {
	c.commandName = name
	return c
}

func (c *NativeUvCommand) SetArgs(args []string) *NativeUvCommand {
	c.args = args
	return c
}

func (c *NativeUvCommand) SetServerID(serverID string) *NativeUvCommand {
	c.serverID = serverID
	return c
}

func (c *NativeUvCommand) SetDeployerRepo(deployerRepo string) *NativeUvCommand {
	c.deployerRepo = deployerRepo
	return c
}

func (c *NativeUvCommand) SetBuildConfiguration(bc *buildUtils.BuildConfiguration) *NativeUvCommand {
	c.buildConfiguration = bc
	return c
}

func (c *NativeUvCommand) CommandName() string {
	return "rt_uv_native"
}

func (c *NativeUvCommand) ServerDetails() (*coreConfig.ServerDetails, error) {
	return uvResolveServerDetails(c.serverID)
}

// Run executes the UV command with auth injection and optional build info collection.
func (c *NativeUvCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Resolve publish URL: explicit arg > pyproject.toml
	deployerRepo := c.deployerRepo
	if c.commandName == "publish" && deployerRepo == "" {
		if tomlURL := uvPublishURLFromToml(workingDir); tomlURL != "" {
			deployerRepo = tomlURL
			c.args = append(c.args, "--publish-url", tomlURL)
			log.Info("Using publish URL from pyproject.toml [tool.uv]: " + tomlURL)
		}
	}

	serverDetails, credErr := uvResolveServerDetails(c.serverID)
	if credErr != nil {
		log.Warn("UV auth: could not load jf server config — " + credErr.Error())
	} else if serverDetails != nil {
		c.injectCredentials(workingDir, deployerRepo, serverDetails)
	}

	log.Info(fmt.Sprintf("Running UV %s.", c.commandName))
	if err := runUvBinary(append([]string{c.commandName}, c.args...)); err != nil {
		return fmt.Errorf("uv %s failed: %w", c.commandName, err)
	}

	if c.buildConfiguration != nil {
		buildName, err := c.buildConfiguration.GetBuildName()
		if err == nil && buildName != "" {
			if biErr := uvGetBuildInfo(workingDir, c.buildConfiguration, deployerRepo, c.commandName, serverDetails); biErr != nil {
				log.Warn("Failed to collect UV build info: " + biErr.Error())
			}
		}
	}
	return nil
}

// injectCredentials sets UV_INDEX_* and UV_PUBLISH_* env vars from jf config,
// only when native UV mechanisms (env vars, embedded URL, netrc) don't already cover the host.
func (c *NativeUvCommand) injectCredentials(workingDir, deployerRepo string, serverDetails *coreConfig.ServerDetails) {
	user := serverDetails.User
	pass := serverDetails.Password
	if pass == "" {
		pass = serverDetails.AccessToken
	}
	if user == "" || pass == "" {
		return
	}

	injectedAny := false

	if os.Getenv("UV_INDEX_URL") != "" {
		log.Info("UV auth: UV_INDEX_URL is set globally (native)")
	}

	for _, idx := range uvReadIndexesFromToml(workingDir) {
		envName := uvIndexEnvName(idx.Name)
		userKey := "UV_INDEX_" + envName + "_USERNAME"
		switch {
		case os.Getenv(userKey) != "":
			log.Info(fmt.Sprintf("UV auth [index %q]: using env var %s (native, step 2)", idx.Name, userKey))
		case uvURLHasEmbeddedCredentials(idx.URL):
			log.Info(fmt.Sprintf("UV auth [index %q]: using credentials embedded in URL (native, step 3)", idx.Name))
		case uvNetrcHasCredentials(idx.URL):
			log.Info(fmt.Sprintf("UV auth [index %q]: using %s (native, step 4)", idx.Name, uvNetrcPath()))
		default:
			if uvHostMatchesServer(idx.URL, serverDetails.ArtifactoryUrl) {
				os.Setenv(userKey, user)
				os.Setenv("UV_INDEX_"+envName+"_PASSWORD", pass)
				injectedAny = true
				log.Info(fmt.Sprintf("UV auth [index %q]: using jf server config (user: %s, fallback)", idx.Name, user))
			} else {
				log.Warn(fmt.Sprintf(
					"UV auth [index %q]: index host (%s) differs from jf server config host (%s) — "+
						"set UV_INDEX_%s_USERNAME/PASSWORD, embed credentials in the URL, or add ~/.netrc entry for %s",
					idx.Name, uvHostOf(idx.URL), uvHostOf(serverDetails.ArtifactoryUrl),
					envName, uvHostOf(idx.URL)))
			}
		}
	}

	if injectedAny {
		os.Setenv("UV_KEYRING_PROVIDER", "disabled")
	}

	if c.commandName == "publish" {
		uvApplyPublishAuth(deployerRepo, workingDir, serverDetails, user, pass)
	}
}

// runUvBinary executes the uv binary with stdio pass-through.
func runUvBinary(args []string) error {
	cmd := exec.Command("uv", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// uvGetCommandName strips the first non-flag argument from a slice and returns
// (commandName, remainingArgs).
func uvGetCommandName(orgArgs []string) (string, []string) {
	cmdArgs := make([]string, len(orgArgs))
	copy(cmdArgs, orgArgs)
	for i, arg := range cmdArgs {
		if !strings.HasPrefix(arg, "-") {
			return arg, append(cmdArgs[:i], cmdArgs[i+1:]...)
		}
	}
	return "", cmdArgs
}

// ── TOML types ───────────────────────────────────────────────────────────────

type uvIndexEntry struct {
	Name string
	URL  string
}

type uvToolUv struct {
	PublishURL string         `toml:"publish-url"`
	Index      []uvIndexEntry `toml:"index"`
}

type uvPyprojectToml struct {
	Tool struct {
		Uv uvToolUv `toml:"uv"`
	} `toml:"tool"`
}

// parseUvPyproject reads and parses pyproject.toml from workingDir.
// Returns a zero-value struct on any error (missing file is normal).
func parseUvPyproject(workingDir string) uvPyprojectToml {
	data, err := os.ReadFile(filepath.Join(workingDir, "pyproject.toml"))
	if err != nil {
		return uvPyprojectToml{}
	}
	var p uvPyprojectToml
	if err := toml.Unmarshal(data, &p); err != nil {
		return uvPyprojectToml{}
	}
	return p
}

func uvPublishURLFromToml(workingDir string) string {
	return parseUvPyproject(workingDir).Tool.Uv.PublishURL
}

func uvReadIndexesFromToml(workingDir string) []uvIndexEntry {
	p := parseUvPyproject(workingDir)
	var entries []uvIndexEntry
	for _, idx := range p.Tool.Uv.Index {
		if idx.Name != "" {
			entries = append(entries, idx)
		}
	}
	return entries
}

// ── Credential helpers ────────────────────────────────────────────────────────

// uvIndexHasNativeCredentials returns true when any native UV mechanism
// (env var, embedded URL, netrc) already provides credentials for the index,
// so jf config injection should be skipped.
func uvIndexHasNativeCredentials(indexURL, envVarUsername string) bool {
	if os.Getenv(envVarUsername) != "" {
		return true
	}
	if uvURLHasEmbeddedCredentials(indexURL) {
		return true
	}
	return uvNetrcHasCredentials(indexURL)
}

// uvHostOf returns the hostname from a URL, or empty string on parse error.
func uvHostOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// uvHostMatchesServer returns true when publishURL's hostname equals the Artifactory
// server's hostname, preventing credential injection onto a different instance.
func uvHostMatchesServer(publishURL, serverURL string) bool {
	return uvHostOf(publishURL) != "" && uvHostOf(publishURL) == uvHostOf(serverURL)
}

// uvMatchingIndexCredentials looks for a [[tool.uv.index]] whose URL shares the
// same hostname as publishURL and returns credentials from the first matching source.
func uvMatchingIndexCredentials(publishURL, workingDir string) (username, password string) {
	publishHost := uvHostOf(publishURL)
	if publishHost == "" {
		return "", ""
	}
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if uvHostOf(idx.URL) != publishHost {
			continue
		}
		envName := uvIndexEnvName(idx.Name)
		u := os.Getenv("UV_INDEX_" + envName + "_USERNAME")
		p := os.Getenv("UV_INDEX_" + envName + "_PASSWORD")
		if u != "" {
			return u, p
		}
		if parsed, err := url.Parse(idx.URL); err == nil && parsed.User != nil {
			u = parsed.User.Username()
			p, _ = parsed.User.Password()
			if u != "" {
				return u, p
			}
		}
	}
	return "", ""
}

// uvURLHasEmbeddedCredentials returns true when the URL contains a userinfo component.
func uvURLHasEmbeddedCredentials(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.User != nil && parsed.User.Username() != ""
}

// uvNetrcPath returns the effective netrc file path (respecting the NETRC env var).
func uvNetrcPath() string {
	if custom := os.Getenv("NETRC"); custom != "" {
		return custom
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".netrc")
}

// uvNetrcHasCredentials returns true when the netrc file contains a `machine <host>`
// entry for the hostname of rawURL.
func uvNetrcHasCredentials(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()

	data, err := os.ReadFile(uvNetrcPath())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "machine" && fields[1] == host {
			return true
		}
	}
	return false
}

// uvIndexEnvName converts a UV index name to the env var suffix UV expects.
// e.g. "agrasth-uv-local" → "AGRASTH_UV_LOCAL"
func uvIndexEnvName(name string) string {
	upper := strings.ToUpper(name)
	return strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(upper)
}

// uvResolveServerDetails returns server details for the given server ID.
// If serverID is empty, the default configured server is used.
func uvResolveServerDetails(serverID string) (*coreConfig.ServerDetails, error) {
	if serverID == "" {
		return coreConfig.GetDefaultServerConf()
	}
	return coreConfig.GetSpecificConfig(serverID, true, true)
}

// uvApplyPublishAuth selects and applies publish credentials following UV's priority chain.
func uvApplyPublishAuth(publishURL, workingDir string, serverDetails *coreConfig.ServerDetails, user, pass string) {
	switch {
	case os.Getenv("UV_PUBLISH_TOKEN") != "":
		log.Info("UV auth [publish]: using UV_PUBLISH_TOKEN (native, step 2a)")
	case os.Getenv("UV_PUBLISH_USERNAME") != "" || os.Getenv("UV_PUBLISH_PASSWORD") != "":
		log.Info("UV auth [publish]: using UV_PUBLISH_USERNAME/PASSWORD (native, step 2b)")
	case uvURLHasEmbeddedCredentials(publishURL):
		log.Info("UV auth [publish]: using credentials embedded in publish URL (native, step 3)")
	case uvNetrcHasCredentials(publishURL):
		log.Info(fmt.Sprintf("UV auth [publish]: using %s (native, step 4)", uvNetrcPath()))
	default:
		uvInjectPublishCredentials(publishURL, workingDir, serverDetails, user, pass)
	}
}

// uvInjectPublishCredentials injects publish credentials from same-host index credentials
// or the jf server config (in that priority order).
func uvInjectPublishCredentials(publishURL, workingDir string, serverDetails *coreConfig.ServerDetails, user, pass string) {
	if idxUser, idxPass := uvMatchingIndexCredentials(publishURL, workingDir); idxUser != "" {
		os.Setenv("UV_PUBLISH_USERNAME", idxUser)
		os.Setenv("UV_PUBLISH_PASSWORD", idxPass)
		os.Setenv("UV_KEYRING_PROVIDER", "disabled")
		log.Info("UV auth [publish]: using same-host index credentials (native, step 4b)")
		return
	}
	if uvHostMatchesServer(publishURL, serverDetails.ArtifactoryUrl) {
		os.Setenv("UV_PUBLISH_USERNAME", user)
		os.Setenv("UV_PUBLISH_PASSWORD", pass)
		os.Setenv("UV_KEYRING_PROVIDER", "disabled")
		log.Info(fmt.Sprintf("UV auth [publish]: using jf server config (user: %s, fallback)", user))
		return
	}
	log.Warn(fmt.Sprintf(
		"UV auth [publish]: publish URL host (%s) does not match jf server config host (%s) — "+
			"set UV_PUBLISH_USERNAME/UV_PUBLISH_PASSWORD or UV_PUBLISH_TOKEN to authenticate",
		uvHostOf(publishURL), uvHostOf(serverDetails.ArtifactoryUrl)))
}

// ── Build info collection ────────────────────────────────────────────────────

// uvGetBuildInfo collects build info for UV projects using the FlexPack native implementation.
func uvGetBuildInfo(workingDir string, buildConfiguration *buildUtils.BuildConfiguration, deployerRepo, cmdName string, serverDetails *coreConfig.ServerDetails) error {
	log.Debug(fmt.Sprintf("Collecting UV build info for command '%s' in: %s", cmdName, workingDir))

	buildName, err := buildConfiguration.GetBuildName()
	if err != nil {
		return fmt.Errorf("GetBuildName failed: %w", err)
	}
	buildNumber, err := buildConfiguration.GetBuildNumber()
	if err != nil {
		return fmt.Errorf("GetBuildNumber failed: %w", err)
	}

	uvConfig := flexpack.UvConfig{
		WorkingDirectory:       workingDir,
		IncludeDevDependencies: false,
	}
	collector, err := flexpack.NewUvFlexPack(uvConfig)
	if err != nil {
		return fmt.Errorf("failed to create UV FlexPack collector: %w", err)
	}

	bi, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect UV build info: %w", err)
	}

	if customModule := buildConfiguration.GetModule(); customModule != "" && len(bi.Modules) > 0 {
		bi.Modules[0].Id = customModule
	}

	switch cmdName {
	case "sync", "install", "lock", "add", "remove", "run":
		if len(bi.Modules) > 0 && len(bi.Modules[0].Dependencies) > 0 {
			if repoKey := uvResolverRepoFromToml(workingDir); repoKey != "" {
				sd := serverDetails
				if sd == nil {
					sd, _ = coreConfig.GetDefaultServerConf()
				}
				if sd != nil {
					if indexURL := uvIndexURLFromToml(workingDir); indexURL != "" && !uvServerHostMatches(indexURL, sd.ArtifactoryUrl) {
						log.Warn(fmt.Sprintf(
							"UV build-info: jf server config host (%s) differs from index URL host (%s) — "+
								"dependency checksum enrichment (sha1/md5) will be skipped. "+
								"Use --server-id to specify the Artifactory instance that hosts your uv packages.",
							uvHostOf(sd.ArtifactoryUrl), uvHostOf(indexURL)))
					}
					uvEnrichDepsFromArtifactory(bi.Modules[0].Dependencies, repoKey, sd)
				}
			}
		}
	}

	switch cmdName {
	case "build":
		if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(artifacts) > 0 {
			if len(bi.Modules) > 0 {
				bi.Modules[0].Artifacts = artifacts
				log.Info(fmt.Sprintf("Collected %d artifact(s) from dist/", len(artifacts)))
			}
		} else if scanErr != nil {
			log.Warn("Could not scan dist/ for artifacts: " + scanErr.Error())
		}
	case "publish":
		repoKey := uvExtractRepoKeyFromURL(deployerRepo)
		sd := serverDetails
		if sd == nil {
			var sdErr error
			sd, sdErr = coreConfig.GetDefaultServerConf()
			if sdErr != nil {
				log.Warn("Could not load server config for artifact lookup: " + sdErr.Error())
				sd = nil
			}
		}
		if sd == nil {
			if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(bi.Modules) > 0 {
				bi.Modules[0].Artifacts = artifacts
			}
			break
		}
		if repoKey != "" {
			if deployerRepo != "" && !uvServerHostMatches(deployerRepo, sd.ArtifactoryUrl) {
				log.Warn(fmt.Sprintf(
					"UV build-info: jf server config host (%s) differs from publish URL host (%s) — "+
						"artifact lookup and build property setting will be skipped. "+
						"Use --server-id to specify the Artifactory instance that hosts your uv packages.",
					uvHostOf(sd.ArtifactoryUrl), uvHostOf(deployerRepo)))
			} else {
				if artErr := uvAddArtifactsToBuildInfo(bi, sd, repoKey, workingDir); artErr != nil {
					log.Warn("Could not look up artifact repo paths, using local checksums: " + artErr.Error())
					if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(bi.Modules) > 0 {
						bi.Modules[0].Artifacts = artifacts
					}
				}
				if propErr := uvSetBuildProperties(sd, repoKey, buildName, buildNumber, buildConfiguration.GetProject(), bi); propErr != nil {
					log.Warn("Failed to set build properties on artifacts: " + propErr.Error())
				}
			}
		} else {
			if artifacts, scanErr := uvCollectDistArtifacts(workingDir); scanErr == nil && len(artifacts) > 0 {
				if len(bi.Modules) > 0 {
					bi.Modules[0].Artifacts = artifacts
					log.Info(fmt.Sprintf("Collected %d artifact(s) from dist/ (no deployer repo set)", len(artifacts)))
				}
			}
		}
	}

	if err = uvSaveBuildInfo(bi, buildConfiguration); err != nil {
		return fmt.Errorf("failed to save UV build info: %w", err)
	}

	log.Info(fmt.Sprintf("UV build info collected. Use 'jf rt bp %s %s' to publish.", buildName, buildNumber))
	return nil
}

// uvResolverRepoFromToml extracts the Artifactory repo key from the first
// [[tool.uv.index]] URL in pyproject.toml.
func uvResolverRepoFromToml(workingDir string) string {
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if idx.URL != "" {
			return uvExtractRepoKeyFromURL(idx.URL)
		}
	}
	return ""
}

// uvIndexURLFromToml returns the raw URL of the first [[tool.uv.index]] entry.
func uvIndexURLFromToml(workingDir string) string {
	for _, idx := range uvReadIndexesFromToml(workingDir) {
		if idx.URL != "" {
			return idx.URL
		}
	}
	return ""
}

// uvServerHostMatches returns true when rawURL's hostname matches the Artifactory server URL hostname.
func uvServerHostMatches(rawURL, serverURL string) bool {
	h := uvHostOf(rawURL)
	return h != "" && h == uvHostOf(serverURL)
}

// uvEnrichDepsFromArtifactory searches each dependency by filename in the given Artifactory repo
// and updates the dependency's sha1 and md5 checksums.
func uvEnrichDepsFromArtifactory(deps []buildinfo.Dependency, repoKey string, serverDetails *coreConfig.ServerDetails) {
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		log.Warn("Could not create services manager for dependency enrichment: " + err.Error())
		return
	}
	searchRepo, err := utils.GetRepoNameForDependenciesSearch(repoKey, servicesManager)
	if err != nil {
		log.Warn("Could not resolve repo for dependency search, using as-is: " + err.Error())
		searchRepo = repoKey
	}

	enriched := 0
	for i, dep := range deps {
		if dep.Id == "" {
			continue
		}
		aqlQuery := specutils.CreateAqlQueryForPypi(searchRepo, dep.Id)
		stream, err := servicesManager.Aql(aqlQuery)
		if err != nil {
			log.Debug(fmt.Sprintf("AQL search failed for %s: %v", dep.Id, err))
			continue
		}
		result, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			continue
		}
		var aql struct {
			Results []struct {
				Actual_Sha1 string `json:"actual_sha1"`
				Actual_Md5  string `json:"actual_md5"`
				Sha256      string `json:"sha256"`
			} `json:"results"`
		}
		if err := json.Unmarshal(result, &aql); err != nil || len(aql.Results) == 0 {
			log.Debug(fmt.Sprintf("Dependency %s not found in repo %s", dep.Id, searchRepo))
			continue
		}
		r := aql.Results[0]
		deps[i].Checksum.Sha1 = r.Actual_Sha1
		deps[i].Checksum.Md5 = r.Actual_Md5
		if r.Sha256 != "" {
			deps[i].Checksum.Sha256 = r.Sha256
		}
		enriched++
		log.Debug(fmt.Sprintf("Enriched %s with sha1=%s md5=%s", dep.Id, r.Actual_Sha1, r.Actual_Md5))
	}
	if enriched > 0 {
		log.Info(fmt.Sprintf("Enriched %d/%d UV dependencies with Artifactory checksums (repo: %s)", enriched, len(deps), searchRepo))
	}
}

// uvExtractRepoKeyFromURL returns just the repo key from a full Artifactory URL or a bare key.
func uvExtractRepoKeyFromURL(repoOrURL string) string {
	if repoOrURL == "" {
		return ""
	}
	if !strings.HasPrefix(repoOrURL, "http://") && !strings.HasPrefix(repoOrURL, "https://") {
		return repoOrURL
	}
	parsed, err := url.Parse(repoOrURL)
	if err != nil {
		return repoOrURL
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "api" && i+2 < len(segments) {
			return segments[i+2]
		}
	}
	for i := len(segments) - 1; i >= 0; i-- {
		if segments[i] != "" && segments[i] != "simple" {
			return segments[i]
		}
	}
	return repoOrURL
}

// uvCollectDistArtifacts collects wheel/sdist artifacts from the dist/ directory.
func uvCollectDistArtifacts(workingDir string) ([]buildinfo.Artifact, error) {
	distDir := filepath.Join(workingDir, "dist")
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("dist directory not found: %s", distDir)
	}
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dist directory: %w", err)
	}

	var artifacts []buildinfo.Artifact
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if !strings.HasSuffix(filename, ".whl") && !strings.HasSuffix(filename, ".tar.gz") {
			continue
		}
		artifact := buildinfo.Artifact{
			Name: filename,
			Path: ".",
			Type: uvArtifactType(filename),
		}
		if checksums, csErr := uvFileChecksums(filepath.Join(distDir, filename)); csErr == nil {
			artifact.Checksum = checksums
		} else {
			log.Warn(fmt.Sprintf("Failed to calculate checksums for %s: %v", filename, csErr))
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// uvFileChecksums calculates SHA1, SHA256, and MD5 checksums for a file.
func uvFileChecksums(filePath string) (buildinfo.Checksum, error) {
	fileDetails, err := crypto.GetFileDetails(filePath, true)
	if err != nil {
		return buildinfo.Checksum{}, fmt.Errorf("failed to calculate checksums: %w", err)
	}
	return buildinfo.Checksum{
		Sha1:   fileDetails.Checksum.Sha1,
		Sha256: fileDetails.Checksum.Sha256,
		Md5:    fileDetails.Checksum.Md5,
	}, nil
}

func uvArtifactType(filename string) string {
	if strings.HasSuffix(filename, ".whl") {
		return "wheel"
	}
	if strings.HasSuffix(filename, ".tar.gz") {
		return "sdist"
	}
	return "unknown"
}

// uvAddArtifactsToBuildInfo looks up uploaded artifacts in Artifactory and adds them
// to the build info module.
func uvAddArtifactsToBuildInfo(bi *buildinfo.BuildInfo, serverDetails *coreConfig.ServerDetails, targetRepo, workingDir string) error {
	if len(bi.Modules) == 0 {
		return fmt.Errorf("no modules found in build info")
	}
	localArtifacts, err := uvCollectDistArtifacts(workingDir)
	if err != nil {
		return fmt.Errorf("failed to get local artifacts: %w", err)
	}
	if len(localArtifacts) == 0 {
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	var artifacts []buildinfo.Artifact
	for _, localArtifact := range localArtifacts {
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Aql: specutils.Aql{
					ItemsFind: uvAqlQueryForSearch(targetRepo, localArtifact.Name),
				},
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to search for artifact %s: %v", localArtifact.Name, err))
			continue
		}
		for result := new(specutils.ResultItem); searchReader.NextRecord(result) == nil; result = new(specutils.ResultItem) {
			artifacts = append(artifacts, buildinfo.Artifact{
				Name:     result.Name,
				Path:     result.Path,
				Type:     uvArtifactType(result.Name),
				Checksum: localArtifact.Checksum,
			})
			break
		}
		if err := searchReader.Close(); err != nil {
			log.Warn("Failed to close search reader:", err)
		}
	}

	bi.Modules[0].Artifacts = artifacts
	log.Info(fmt.Sprintf("Added %d artifacts to build info", len(artifacts)))
	return nil
}

// uvSetBuildProperties sets build.name / build.number properties on uploaded Python dist artifacts.
func uvSetBuildProperties(serverDetails *coreConfig.ServerDetails, targetRepo, buildName, buildNumber, project string, bi *buildinfo.BuildInfo) error {
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	if err := buildUtils.SaveBuildGeneralDetails(buildName, buildNumber, project); err != nil {
		return fmt.Errorf("SaveBuildGeneralDetails failed: %w", err)
	}
	buildProps, err := buildUtils.CreateBuildProperties(buildName, buildNumber, project)
	if err != nil {
		return fmt.Errorf("CreateBuildProperties failed: %w", err)
	}

	if len(bi.Modules) == 0 || len(bi.Modules[0].Artifacts) == 0 {
		return nil
	}
	for _, artifact := range bi.Modules[0].Artifacts {
		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Aql: specutils.Aql{
					ItemsFind: uvAqlQueryForSearch(targetRepo, artifact.Name),
				},
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to find artifact %s: %v", artifact.Name, err))
			continue
		}
		_, err = servicesManager.SetProps(services.PropsParams{Reader: searchReader, Props: buildProps})
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to set properties on artifact %s: %v", artifact.Name, err))
		}
	}
	log.Info(fmt.Sprintf("Successfully set build properties on %d artifacts", len(bi.Modules[0].Artifacts)))
	return nil
}

// uvSaveBuildInfo saves the build info locally for later publishing with 'jf rt bp'.
func uvSaveBuildInfo(bi *buildinfo.BuildInfo, buildConfiguration *buildUtils.BuildConfiguration) error {
	service := buildUtils.CreateBuildInfoService()
	bld, err := service.GetOrCreateBuildWithProject(bi.Name, bi.Number, buildConfiguration.GetProject())
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return bld.SaveBuildInfo(bi)
}

// uvAqlQueryForSearch returns an AQL items.find query for a file in a repo.
func uvAqlQueryForSearch(repo, file string) string {
	return fmt.Sprintf(
		`{"repo": %q, "$or": [{"$and": [{"path": {"$match": "*"}, "name": {"$match": %q}}]}]}`,
		repo, file,
	)
}
