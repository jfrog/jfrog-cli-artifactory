package ruby

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/gofrog/crypto"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// Supported native tools.
const (
	toolGem    = "gem"
	toolBundle = "bundle"
)

// Run executes the native gem/bundle command with Artifactory auth injection and,
// when build parameters are supplied, collects build info.
func (rc *RubyCommand) Run() error {
	if rc.nativeTool == "" {
		rc.nativeTool = toolGem
	}
	if rc.nativeTool != toolGem && rc.nativeTool != toolBundle {
		return fmt.Errorf("unsupported ruby tool %q: expected 'gem' or 'bundle'", rc.nativeTool)
	}

	subCommand := ""
	if len(rc.args) > 0 {
		subCommand = rc.args[0]
	}

	// Help requests must bypass auth injection entirely so credentials are never
	// printed in help output (same rationale as the UV native command).
	if isRubyHelpRequest(subCommand, rc.args) {
		return runRubyBinary(rc.nativeTool, rc.args, nil)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	serverDetails, srvErr := rc.ServerDetails()
	if srvErr != nil {
		log.Warn("Ruby auth: could not load jf server config — " + srvErr.Error())
		serverDetails = nil
	}

	// Discover the Artifactory gem source the project points at, then inject auth.
	sourceURL, repoKey := rc.resolveRepo(workingDir)
	var extraEnv []string
	if serverDetails != nil {
		extraEnv = rc.injectAuth(serverDetails, sourceURL)
	}

	log.Info(fmt.Sprintf("Running %s %s.", rc.nativeTool, subCommand))
	if runErr := runRubyBinary(rc.nativeTool, rc.args, extraEnv); runErr != nil {
		return fmt.Errorf("%s %s failed: %w", rc.nativeTool, subCommand, runErr)
	}

	if rc.buildConfiguration != nil {
		buildName, nameErr := rc.buildConfiguration.GetBuildName()
		if nameErr == nil && buildName != "" {
			if biErr := rc.collectBuildInfo(workingDir, subCommand, repoKey, serverDetails); biErr != nil {
				log.Warn("Failed to collect Ruby build info: " + biErr.Error())
			}
		}
	}
	return nil
}

// runRubyBinary executes gem/bundle with stdio pass-through and optional extra env vars.
func runRubyBinary(tool string, args, extraEnv []string) error {
	cmd := exec.Command(tool, args...) // #nosec G204 -- tool is restricted to gem/bundle; args come from the user's own command line
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}

// isRubyHelpRequest reports whether the invocation is purely a help request.
func isRubyHelpRequest(subCommand string, args []string) bool {
	if subCommand == "help" || subCommand == "" {
		return true
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// rubyResolveServerDetails resolves the jf server config for the given server ID,
// falling back to the default server when empty.
func rubyResolveServerDetails(serverID string) (*coreConfig.ServerDetails, error) {
	if serverID == "" {
		return coreConfig.GetDefaultServerConf()
	}
	return coreConfig.GetSpecificConfig(serverID, true, true)
}

// ── Authentication ───────────────────────────────────────────────────────────

// injectAuth returns the additional environment variables required to authenticate
// the native tool against Artifactory. It is non-destructive: a credential is only
// injected when the user has not already configured one natively (env var, embedded
// URL credentials, ~/.gem/credentials, or .bundle/config), mirroring the UV flow.
//
// Bundler  → BUNDLE_<HOST_KEY>="user:password" (Bundler's per-host credential env var).
// RubyGems → GEM_HOST_API_KEY="user:password" (used by `gem push`/`gem fetch`).
func (rc *RubyCommand) injectAuth(serverDetails *coreConfig.ServerDetails, sourceURL string) []string {
	user, pass := rubyCredentials(serverDetails)
	if user == "" || pass == "" {
		log.Debug("Ruby auth: no username/password/token available in server config; relying on native configuration")
		return nil
	}

	// Determine the host to authenticate. Prefer the discovered source URL host;
	// otherwise fall back to the Artifactory server host.
	host := rubyHostOf(sourceURL)
	if host == "" {
		host = rubyHostOf(serverDetails.ArtifactoryUrl)
	}
	// Without --server-id, only inject when the source host matches the jf server
	// host to avoid leaking credentials to an unrelated registry.
	if rc.serverID == "" && sourceURL != "" && !rubyHostMatchesServer(sourceURL, serverDetails.ArtifactoryUrl) {
		log.Warn(fmt.Sprintf(
			"Ruby auth: gem source host (%s) differs from jf server config host (%s) — "+
				"skipping credential injection. Use --server-id to authenticate explicitly, "+
				"or configure credentials with `bundle config set` / ~/.gem/credentials.",
			host, rubyHostOf(serverDetails.ArtifactoryUrl)))
		return nil
	}

	var extraEnv []string
	switch rc.nativeTool {
	case toolBundle:
		key := bundleEnvKeyForHost(host)
		if os.Getenv(key) != "" {
			log.Info(fmt.Sprintf("Ruby auth [bundle]: %s already set — respecting existing credentials", key))
		} else {
			extraEnv = append(extraEnv, fmt.Sprintf("%s=%s:%s", key, user, pass))
			log.Info(fmt.Sprintf("Ruby auth [bundle]: injecting credentials via %s", key))
		}
	case toolGem:
		if os.Getenv("GEM_HOST_API_KEY") != "" {
			log.Info("Ruby auth [gem]: GEM_HOST_API_KEY already set — respecting existing credentials")
		} else {
			extraEnv = append(extraEnv, fmt.Sprintf("GEM_HOST_API_KEY=%s:%s", user, pass))
			log.Info("Ruby auth [gem]: injecting credentials via GEM_HOST_API_KEY")
		}
	}
	return extraEnv
}

// rubyCredentials extracts the effective username/password, handling access tokens.
func rubyCredentials(serverDetails *coreConfig.ServerDetails) (user, pass string) {
	user = serverDetails.GetUser()
	pass = serverDetails.GetPassword()
	if serverDetails.GetAccessToken() != "" {
		if user == "" {
			user = auth.ExtractUsernameFromAccessToken(serverDetails.GetAccessToken())
		}
		pass = serverDetails.GetAccessToken()
	}
	return user, pass
}

// bundleEnvKeyForHost converts a host into Bundler's per-host credential env var name,
// following Bundler's key normalization: uppercase, "." → "__", "-" → "___", and any
// remaining non-alphanumeric character → "_", prefixed with "BUNDLE_".
//
//	"mycompany.jfrog.io" → "BUNDLE_MYCOMPANY__JFROG__IO"
func bundleEnvKeyForHost(host string) string {
	key := strings.ToUpper(host)
	key = strings.ReplaceAll(key, ".", "__")
	key = strings.ReplaceAll(key, "-", "___")
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return "BUNDLE_" + b.String()
}

// ── Repository discovery ───────────────────────────────────────────────────────

// resolveRepo discovers the Artifactory gem source URL and repo key the project uses.
// Precedence: explicit --repo override > --source/--host/--clear-sources arg >
// Gemfile `source` line > `gem sources` list. Returns empty strings when none is found.
func (rc *RubyCommand) resolveRepo(workingDir string) (sourceURL, repoKey string) {
	if rc.repository != "" {
		return "", rc.repository
	}
	// 1. Inspect the command args for an explicit source/host URL.
	if u := rubySourceFromArgs(rc.args); u != "" {
		return u, rubyExtractRepoKeyFromURL(u)
	}
	// 2. Gemfile `source "<url>"` pointing at /api/gems/.
	if u := rubySourceFromGemfile(workingDir); u != "" {
		return u, rubyExtractRepoKeyFromURL(u)
	}
	// 3. Configured gem sources.
	if u := rubySourceFromGemSources(); u != "" {
		return u, rubyExtractRepoKeyFromURL(u)
	}
	return "", ""
}

// rubySourceFromArgs returns the URL following --source/-s/--host/--clear-sources flags,
// or an inline "--source=<url>" form.
func rubySourceFromArgs(args []string) string {
	for i, a := range args {
		switch {
		case strings.HasPrefix(a, "--source="):
			return strings.TrimPrefix(a, "--source=")
		case strings.HasPrefix(a, "--host="):
			return strings.TrimPrefix(a, "--host=")
		case a == "--source" || a == "-s" || a == "--host":
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

// rubySourceFromGemfile scans the project's Gemfile for a `source "<url>"` directive
// that points at an Artifactory gems repository.
func rubySourceFromGemfile(workingDir string) string {
	gemfile := filepath.Join(workingDir, "Gemfile")
	data, err := os.ReadFile(gemfile)
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "source") {
			continue
		}
		if u := extractQuotedURL(line); u != "" && strings.Contains(u, "/api/gems/") {
			return u
		}
	}
	return ""
}

// rubySourceFromGemSources runs `gem sources --list` and returns the first Artifactory
// gems URL it finds. Best-effort: returns empty on any error.
func rubySourceFromGemSources() string {
	out, err := exec.Command("gem", "sources", "--list").Output()
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "/api/gems/") && (strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")) {
			return line
		}
	}
	return ""
}

// extractQuotedURL pulls the first single- or double-quoted token from a line.
func extractQuotedURL(line string) string {
	for _, q := range []byte{'"', '\''} {
		start := strings.IndexByte(line, q)
		if start == -1 {
			continue
		}
		end := strings.IndexByte(line[start+1:], q)
		if end == -1 {
			continue
		}
		return line[start+1 : start+1+end]
	}
	return ""
}

// rubyExtractRepoKeyFromURL returns the repo key from a full Artifactory URL
// (".../api/gems/<repo>/...") or returns the input unchanged when it is a bare key.
func rubyExtractRepoKeyFromURL(repoOrURL string) string {
	if repoOrURL == "" {
		return ""
	}
	if !strings.HasPrefix(repoOrURL, "http://") && !strings.HasPrefix(repoOrURL, "https://") {
		return repoOrURL
	}
	parsed, err := url.Parse(repoOrURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "gems" && i+1 < len(segments) {
			return segments[i+1]
		}
		// Also handle "/api/gems/<repo>".
		if seg == "api" && i+2 < len(segments) && segments[i+1] == "gems" {
			return segments[i+2]
		}
	}
	return ""
}

// rubyHostOf returns the host[:port] of a URL, or "" when not parseable.
func rubyHostOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// rubyHostMatchesServer reports whether rawURL has the same host as the Artifactory URL.
func rubyHostMatchesServer(rawURL, artifactoryURL string) bool {
	h := rubyHostOf(rawURL)
	return h != "" && h == rubyHostOf(artifactoryURL)
}

// ── Build info ─────────────────────────────────────────────────────────────────

// collectBuildInfo dispatches build-info collection based on the native tool/sub-command.
func (rc *RubyCommand) collectBuildInfo(workingDir, subCommand, repoKey string, serverDetails *coreConfig.ServerDetails) error {
	switch {
	case rc.nativeTool == toolGem && (subCommand == "build" || subCommand == "push"):
		return rc.collectGemArtifactBuildInfo(workingDir, subCommand, repoKey, serverDetails)
	case rc.collectsDependencies(subCommand):
		return rc.collectDependencyBuildInfo(workingDir, subCommand, repoKey, serverDetails)
	default:
		log.Debug(fmt.Sprintf("Ruby build-info: no collection for '%s %s'", rc.nativeTool, subCommand))
		return nil
	}
}

// collectsDependencies reports whether the sub-command resolves a dependency tree
// (i.e. produces/uses a Gemfile.lock we can read).
func (rc *RubyCommand) collectsDependencies(subCommand string) bool {
	if rc.nativeTool == toolBundle {
		switch subCommand {
		case "install", "update", "lock", "add":
			return true
		}
	}
	if rc.nativeTool == toolGem {
		// `gem install`/`gem fetch` only yield a Gemfile.lock-style tree inside a
		// bundler project; collected opportunistically when a lock file exists.
		switch subCommand {
		case "install", "fetch":
			return true
		}
	}
	return false
}

// collectDependencyBuildInfo parses Gemfile.lock and records dependencies, enriching
// checksums from Artifactory.
func (rc *RubyCommand) collectDependencyBuildInfo(workingDir, subCommand, repoKey string, serverDetails *coreConfig.ServerDetails) error {
	buildName, err := rc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := rc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	gemConfig := flexpack.GemConfig{WorkingDirectory: workingDir}
	// For bundler, use `bundle list` as the ground-truth installed set so group
	// filtering (--without/--with) is reflected accurately.
	if rc.nativeTool == toolBundle {
		gemConfig.InstalledPackages = bundleInstalledPackages(workingDir)
	}

	collector, err := flexpack.NewRubygemsFlexPack(gemConfig)
	if err != nil {
		return fmt.Errorf("failed to create RubyGems FlexPack collector: %w", err)
	}
	bi, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect RubyGems build info: %w", err)
	}

	if customModule := rc.buildConfiguration.GetModule(); customModule != "" && len(bi.Modules) > 0 {
		bi.Modules[0].Id = customModule
	}

	if len(bi.Modules) > 0 && len(bi.Modules[0].Dependencies) > 0 && repoKey != "" && serverDetails != nil {
		directURLDeps := collector.GetDirectURLDeps()
		rubyEnrichDepsFromArtifactory(bi.Modules[0].Dependencies, repoKey, directURLDeps, serverDetails)
	} else if repoKey == "" {
		log.Info("Ruby build-info: no Artifactory gems repo discovered — dependency checksum enrichment skipped. " +
			"Point your Gemfile/gem source at an Artifactory gems repository or pass --server-id.")
	}

	if err := rubySaveBuildInfo(bi, rc.buildConfiguration); err != nil {
		return fmt.Errorf("failed to save RubyGems build info: %w", err)
	}
	log.Info(fmt.Sprintf("RubyGems build info collected. Use 'jf rt bp %s %s' to publish.", buildName, buildNumber))
	return nil
}

// collectGemArtifactBuildInfo records the .gem artifact produced by `gem build`/`gem push`.
func (rc *RubyCommand) collectGemArtifactBuildInfo(workingDir, subCommand, repoKey string, serverDetails *coreConfig.ServerDetails) error {
	buildName, err := rc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := rc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	artifacts, err := rubyCollectGemArtifacts(workingDir, rc.args)
	if err != nil {
		return fmt.Errorf("failed to collect gem artifacts: %w", err)
	}
	if len(artifacts) == 0 {
		log.Debug("Ruby build-info: no .gem artifacts found to record")
		return nil
	}

	moduleID := rc.gemModuleID(workingDir)
	if customModule := rc.buildConfiguration.GetModule(); customModule != "" {
		moduleID = customModule
	}

	bi := &buildinfo.BuildInfo{
		Name:       buildName,
		Number:     buildNumber,
		Agent:      &buildinfo.Agent{Name: "gem"},
		BuildAgent: &buildinfo.Agent{Name: "Generic", Version: "1.0"},
		Modules: []buildinfo.Module{{
			Id:        moduleID,
			Type:      buildinfo.Gem,
			Artifacts: artifacts,
		}},
	}

	// On push, set build properties on the uploaded artifacts in Artifactory.
	if subCommand == "push" && repoKey != "" && serverDetails != nil {
		if propErr := rubySetBuildProperties(serverDetails, repoKey, buildName, buildNumber, rc.buildConfiguration.GetProject(), bi); propErr != nil {
			log.Warn("Failed to set build properties on gem artifacts: " + propErr.Error())
		}
	}

	if err := rubySaveBuildInfo(bi, rc.buildConfiguration); err != nil {
		return fmt.Errorf("failed to save RubyGems build info: %w", err)
	}
	log.Info(fmt.Sprintf("RubyGems build info collected. Use 'jf rt bp %s %s' to publish.", buildName, buildNumber))
	return nil
}

// gemModuleID derives a module ID for gem build/push from the gemspec/dir name.
func (rc *RubyCommand) gemModuleID(workingDir string) string {
	name := filepath.Base(workingDir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "ruby-project"
	}
	return name
}

// rubyCollectGemArtifacts locates .gem files (build output or explicit push target)
// and computes their checksums for build-info.
func rubyCollectGemArtifacts(workingDir string, args []string) ([]buildinfo.Artifact, error) {
	gemFiles := make(map[string]bool)

	// Explicit .gem path on a `gem push <file>` command.
	for _, a := range args {
		if strings.HasSuffix(a, ".gem") {
			p := a
			if !filepath.IsAbs(p) {
				p = filepath.Join(workingDir, a)
			}
			gemFiles[p] = true
		}
	}

	// `gem build` writes <name>-<version>.gem into the working dir and pkg/.
	for _, dir := range []string{workingDir, filepath.Join(workingDir, "pkg")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".gem") {
				gemFiles[filepath.Join(dir, e.Name())] = true
			}
		}
	}

	var artifacts []buildinfo.Artifact
	for path := range gemFiles {
		checksum, err := rubyFileChecksums(path)
		if err != nil {
			log.Warn(fmt.Sprintf("Could not compute checksums for %s: %v", path, err))
			continue
		}
		artifacts = append(artifacts, buildinfo.Artifact{
			Name:     filepath.Base(path),
			Type:     gemDepArtifactType,
			Path:     filepath.Base(path),
			Checksum: checksum,
		})
	}
	return artifacts, nil
}

// rubyFileChecksums calculates SHA1, SHA256 and MD5 for a file.
func rubyFileChecksums(filePath string) (buildinfo.Checksum, error) {
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

// gemDepArtifactType is the build-info artifact/dependency type for gem files.
const gemDepArtifactType = "gem"

// rubySaveBuildInfo persists the build info locally for a later `jf rt bp`.
func rubySaveBuildInfo(bi *buildinfo.BuildInfo, buildConfiguration *buildUtils.BuildConfiguration) error {
	service := buildUtils.CreateBuildInfoService()
	bld, err := service.GetOrCreateBuildWithProject(bi.Name, bi.Number, buildConfiguration.GetProject())
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return bld.SaveBuildInfo(bi)
}

// bundleInstalledPackages runs `bundle list` and returns the installed gems as
// name → version. Returns nil on error (caller falls back to including the full lock).
func bundleInstalledPackages(workingDir string) map[string]string {
	cmd := exec.Command("bundle", "list")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Debug(fmt.Sprintf("bundle list failed, using full Gemfile.lock for build-info: %v", err))
		return nil
	}
	installed := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		// Lines look like: "  * rake (13.0.6)"
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimPrefix(line, "* ")
		name, version := parseBundleListLine(line)
		if name != "" {
			installed[name] = version
		}
	}
	if len(installed) == 0 {
		return nil
	}
	return installed
}

// parseBundleListLine parses "rake (13.0.6)" → name, version.
func parseBundleListLine(line string) (name, version string) {
	open := strings.Index(line, " (")
	if open == -1 {
		return "", ""
	}
	name = strings.TrimSpace(line[:open])
	rest := line[open+2:]
	if closeIdx := strings.IndexByte(rest, ')'); closeIdx != -1 {
		version = strings.TrimSpace(rest[:closeIdx])
	}
	return name, version
}

// rubyEnrichDepsFromArtifactory fetches sha1/sha256/md5 for registry-based dependencies
// in a single batched AQL call, matching .gem filenames by "<name>-<version>" prefix.
// GIT/PATH deps (in directURLDeps) are skipped since they are not stored in Artifactory.
func rubyEnrichDepsFromArtifactory(deps []buildinfo.Dependency, repoKey string, directURLDeps map[string]string, serverDetails *coreConfig.ServerDetails) {
	if len(deps) == 0 {
		return
	}
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

	type depEntry struct {
		idx    int
		prefix string // "<name>-<version>" used to match the .gem filename
	}
	var entries []depEntry
	for i, dep := range deps {
		if dep.Id == "" {
			continue
		}
		if _, isDirect := directURLDeps[dep.Id]; isDirect {
			continue
		}
		colonIdx := strings.LastIndex(dep.Id, ":")
		if colonIdx < 0 {
			continue
		}
		name, version := dep.Id[:colonIdx], dep.Id[colonIdx+1:]
		entries = append(entries, depEntry{i, name + "-" + version})
	}
	if len(entries) == 0 {
		return
	}

	var orClauses []string
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.prefix] {
			continue
		}
		seen[e.prefix] = true
		// Match "<name>-<version>.gem" and platform-specific "<name>-<version>-<platform>.gem".
		orClauses = append(orClauses, fmt.Sprintf(`{"name":{"$match":%q}}`, e.prefix+"*.gem"))
	}
	aqlQuery := fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("name","actual_sha1","actual_md5","sha256")`,
		searchRepo, strings.Join(orClauses, ","),
	)

	stream, err := servicesManager.Aql(aqlQuery)
	if err != nil {
		log.Debug(fmt.Sprintf("Batch AQL enrichment failed for repo %s: %v", searchRepo, err))
		return
	}
	raw, _ := io.ReadAll(stream)
	_ = stream.Close()

	var aqlResult struct {
		Results []struct {
			Name       string `json:"name"`
			ActualSha1 string `json:"actual_sha1"`
			ActualMd5  string `json:"actual_md5"`
			Sha256     string `json:"sha256"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &aqlResult); err != nil {
		log.Debug(fmt.Sprintf("Failed to parse AQL enrichment response: %v", err))
		return
	}

	enriched := 0
	for _, r := range aqlResult.Results {
		if r.ActualSha1 == "" {
			continue
		}
		for _, e := range entries {
			if deps[e.idx].Sha1 != "" {
				continue
			}
			// "<name>-<version>.gem" or "<name>-<version>-<platform>.gem"
			if r.Name == e.prefix+".gem" || strings.HasPrefix(r.Name, e.prefix+"-") {
				deps[e.idx].Sha1 = r.ActualSha1
				deps[e.idx].Md5 = r.ActualMd5
				if r.Sha256 != "" && deps[e.idx].Sha256 == "" {
					deps[e.idx].Sha256 = r.Sha256
				}
				enriched++
				break
			}
		}
	}

	if enriched > 0 {
		log.Info(fmt.Sprintf("Enriched %d/%d RubyGems dependencies with Artifactory checksums (repo: %s)", enriched, len(deps), searchRepo))
	} else {
		log.Debug(fmt.Sprintf("No RubyGems dependencies enriched from repo %s — gems may not be cached yet", searchRepo))
	}
}

// rubyAqlQueryForSearch builds an AQL ItemsFind expression matching a file by name.
func rubyAqlQueryForSearch(repo, file string) string {
	return fmt.Sprintf(
		`{"repo": %q, "$or": [{"$and": [{"path": {"$match": "*"}, "name": {"$match": %q}}]}]}`,
		repo, file,
	)
}

// rubySetBuildProperties tags uploaded .gem artifacts with build.name/number properties
// so they are linked to the build in Artifactory.
func rubySetBuildProperties(serverDetails *coreConfig.ServerDetails, repoKey, buildName, buildNumber, project string, bi *buildinfo.BuildInfo) error {
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}
	searchRepo, err := utils.GetRepoNameForDependenciesSearch(repoKey, servicesManager)
	if err != nil {
		searchRepo = repoKey
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
					ItemsFind: rubyAqlQueryForSearch(searchRepo, artifact.Name),
				},
			},
		}
		searchReader, searchErr := servicesManager.SearchFiles(searchParams)
		if searchErr != nil {
			log.Warn(fmt.Sprintf("Failed to find artifact %s: %v", artifact.Name, searchErr))
			continue
		}
		_, setErr := servicesManager.SetProps(services.PropsParams{Reader: searchReader, Props: buildProps})
		if closeErr := searchReader.Close(); closeErr != nil {
			log.Warn("Failed to close search reader:", closeErr)
		}
		if setErr != nil {
			log.Warn(fmt.Sprintf("Failed to set properties on artifact %s: %v", artifact.Name, setErr))
		}
	}
	log.Info(fmt.Sprintf("Successfully set build properties on %d artifacts", len(bi.Modules[0].Artifacts)))
	return nil
}
