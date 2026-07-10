package ruby

import (
	"bufio"
	"encoding/base64"
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

	// Bug 3 fix: explicit no-args check before help bypass so we don't
	// silently fall into gem help when no subcommand is given.
	if len(rc.args) == 0 {
		return fmt.Errorf("no subcommand provided for '%s'. Usage: jf ruby %s <subcommand> [args...]", rc.nativeTool, rc.nativeTool)
	}

	subCommand := rc.args[0]

	// Help requests bypass auth injection entirely so credentials are never
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
	sourceURL, repoKey := rc.resolveRepo(workingDir, serverDetails)

	// When --repo constructed the URL and no --source/--host was provided in args,
	// inject the source/host arg into the native command so the tool knows where to point.
	if rc.repository != "" && sourceURL != "" && rubySourceFromArgs(rc.args) == "" {
		rc.args = rubyInjectSourceArg(rc.nativeTool, subCommand, rc.args, sourceURL)
	}

	var extraEnv []string
	var credCleanup func()
	// gem build is a pure local operation — skip auth injection entirely.
	if rc.nativeTool == toolGem && subCommand == "build" {
		log.Debug("Ruby auth: skipping credential injection for gem build (local-only operation)")
	} else if serverDetails != nil && sourceURL != "" {
		extraEnv = rc.injectAuth(serverDetails, sourceURL)
		if rc.nativeTool == toolGem {
			switch subCommand {
			case "install", "fetch":
				// Embed credentials in --source URL for index downloads (specs.4.8.gz).
				rc.args = rubyEmbedCredsInSourceArg(rc.args, serverDetails)
				log.Debug("Ruby auth [gem install/fetch]: embedded credentials in --source URL for index downloads")
			case "push":
				// Strip trailing slash from --host in args (whether user-provided or injected).
				// RubyGems' push_command.rb builds URLs as "#{host}/api/v1/gems" — a trailing
				// slash on host produces a double-slash that Artifactory rejects with 405.
				rc.args = rubyStripHostTrailingSlash(rc.args)
				// Write temporary ~/.gem/credentials for the target host.
				// CRITICAL: the credentials key MUST exactly match the --host value that
				// gets passed to the native command (no trailing slash).
				pushHost := strings.TrimRight(sourceURL, "/")
				cleanup, credErr := rubyWriteTempGemCredentials(pushHost, serverDetails)
				if credErr != nil {
					log.Warn("Ruby auth [gem push]: failed to write temporary credentials: " + credErr.Error())
				} else {
					credCleanup = cleanup
					log.Debug("Ruby auth [gem push]: wrote temporary ~/.gem/credentials entry for " + pushHost)
				}
			}
		}
	} else if serverDetails != nil && sourceURL == "" {
		log.Debug("Ruby auth: no Artifactory gem source discovered in args/Gemfile/gem-sources — skipping credential injection")
	}
	defer func() {
		if credCleanup != nil {
			credCleanup()
		}
	}()

	log.Info(fmt.Sprintf("Running %s %s.", rc.nativeTool, subCommand))
	// For gem install/fetch, capture stdout to parse "Successfully installed"/"Downloaded" lines.
	var capturedOutput string
	needsCapture := rc.nativeTool == toolGem && (subCommand == "install" || subCommand == "fetch")
	if needsCapture {
		var runErr error
		capturedOutput, runErr = runRubyBinaryCapture(rc.nativeTool, rc.args, extraEnv)
		if runErr != nil {
			return fmt.Errorf("%s %s failed: %w", rc.nativeTool, subCommand, runErr)
		}
	} else {
		if runErr := runRubyBinary(rc.nativeTool, rc.args, extraEnv); runErr != nil {
			return fmt.Errorf("%s %s failed: %w", rc.nativeTool, subCommand, runErr)
		}
	}

	if rc.buildConfiguration != nil {
		buildName, nameErr := rc.buildConfiguration.GetBuildName()
		if nameErr == nil && buildName != "" {
			if biErr := rc.collectBuildInfo(workingDir, subCommand, repoKey, serverDetails, capturedOutput); biErr != nil {
				log.Warn("Failed to collect Ruby build info: " + biErr.Error())
			}
		}
	}
	return nil
}

// rubyEmbedCredsInSourceArg rewrites --source/--host URL args to embed credentials
// for gem install/fetch. RubyGems 3.x uses embedded URL credentials for index downloads
// (specs.4.8.gz) but does NOT use GEM_HOST_API_KEY for those requests.
func rubyEmbedCredsInSourceArg(args []string, serverDetails *coreConfig.ServerDetails) []string {
	user, pass := rubyCredentials(serverDetails)
	if user == "" || pass == "" {
		return args
	}
	result := make([]string, len(args))
	copy(result, args)
	for i, a := range result {
		var rawURL string
		var prefix string
		switch {
		case strings.HasPrefix(a, "--source="):
			prefix = "--source="
			rawURL = strings.TrimPrefix(a, prefix)
		case strings.HasPrefix(a, "--host="):
			prefix = "--host="
			rawURL = strings.TrimPrefix(a, prefix)
		case (a == "--source" || a == "-s" || a == "--host") && i+1 < len(result):
			parsed, err := url.Parse(result[i+1])
			if err != nil || parsed.User != nil {
				continue
			}
			parsed.User = url.UserPassword(user, pass)
			result[i+1] = parsed.String()
			continue
		default:
			continue
		}
		if rawURL == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.User != nil {
			continue
		}
		parsed.User = url.UserPassword(user, pass)
		result[i] = prefix + parsed.String()
	}
	return result
}

// rubyEmbedCredsInHostArg embeds credentials in the --host URL for gem push.
// This is the fallback for RubyGems <= 3.0.x which does NOT respect GEM_HOST_API_KEY.
// Uses the same logic as rubyEmbedCredsInSourceArg but only targets --host.
func rubyEmbedCredsInHostArg(args []string, serverDetails *coreConfig.ServerDetails) []string {
	user, pass := rubyCredentials(serverDetails)
	if user == "" || pass == "" {
		return args
	}
	result := make([]string, len(args))
	copy(result, args)
	for i, a := range result {
		var rawURL string
		var prefix string
		switch {
		case strings.HasPrefix(a, "--host="):
			prefix = "--host="
			rawURL = strings.TrimPrefix(a, prefix)
		case a == "--host" && i+1 < len(result):
			parsed, err := url.Parse(result[i+1])
			if err != nil || parsed.User != nil {
				continue
			}
			parsed.User = url.UserPassword(user, pass)
			result[i+1] = parsed.String()
			continue
		default:
			continue
		}
		if rawURL == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.User != nil {
			continue
		}
		parsed.User = url.UserPassword(user, pass)
		result[i] = prefix + parsed.String()
	}
	return result
}

// rubyWriteTempGemCredentials writes a temporary entry to ~/.gem/credentials for gem push.
// This is the only auth mechanism that works across ALL RubyGems versions (3.0.x through current).
// RubyGems' push_command.rb ALWAYS checks ~/.gem/credentials keyed by host URL.
// Returns a cleanup function that restores the original file (or removes the added entry).
func rubyWriteTempGemCredentials(hostURL string, serverDetails *coreConfig.ServerDetails) (cleanup func(), err error) {
	user, pass := rubyCredentials(serverDetails)
	if user == "" || pass == "" {
		return nil, fmt.Errorf("no credentials available")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine home directory: %w", err)
	}
	gemDir := filepath.Join(homeDir, ".gem")
	credFile := filepath.Join(gemDir, "credentials")

	// Ensure ~/.gem directory exists.
	if err := os.MkdirAll(gemDir, 0700); err != nil {
		return nil, fmt.Errorf("could not create ~/.gem directory: %w", err)
	}

	// Read existing credentials file (if any) to preserve and restore later.
	var originalContent []byte
	var originalExists bool
	if data, readErr := os.ReadFile(credFile); readErr == nil {
		originalContent = data
		originalExists = true
	}

	// The credential value: Basic auth encoded (what Artifactory expects).
	credValue := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))

	// CRITICAL: Use the host URL EXACTLY as-is (including trailing slash if present).
	// RubyGems' Gem::GemcutterUtilities#api_key does an exact string match against
	// the --host value. If we strip the trailing slash but --host has one, the lookup misses.
	credKey := hostURL

	// Build new credentials content: preserve existing + add our entry.
	var newContent string
	if originalExists {
		newContent = string(originalContent)
		// Remove existing entry for same host if present (we'll re-add it).
		// Check both with and without trailing slash to avoid duplicates.
		keyWithSlash := strings.TrimRight(hostURL, "/") + "/"
		keyWithoutSlash := strings.TrimRight(hostURL, "/")
		lines := strings.Split(newContent, "\n")
		var filtered []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, keyWithSlash+":") || strings.HasPrefix(trimmed, keyWithoutSlash+":") {
				continue
			}
			filtered = append(filtered, line)
		}
		newContent = strings.Join(filtered, "\n")
	} else {
		newContent = "---\n"
	}

	// Add our entry keyed by the EXACT host URL (preserving trailing slash).
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += fmt.Sprintf("%s: %s\n", credKey, credValue)

	// Write the credentials file with restricted permissions (0600 required by RubyGems).
	if err := os.WriteFile(credFile, []byte(newContent), 0600); err != nil {
		return nil, fmt.Errorf("could not write credentials file: %w", err)
	}

	// Return cleanup function.
	cleanup = func() {
		if originalExists {
			_ = os.WriteFile(credFile, originalContent, 0600)
		} else {
			_ = os.Remove(credFile)
		}
		log.Debug("Ruby auth [gem push]: cleaned up temporary ~/.gem/credentials entry")
	}
	return cleanup, nil
}

// rubyInjectSourceArg appends the appropriate source/host flag to native args when
// --repo was used to construct the URL and the user didn't provide one in their command.
// For gem push → --host; for gem install/fetch → --source; for bundle → no arg needed
// (Bundler uses env-var-based auth and reads from Gemfile, so the Gemfile must point
// at Artifactory — --repo only helps with credential injection for bundle).
func rubyInjectSourceArg(tool, subCommand string, args []string, sourceURL string) []string {
	if tool == toolGem {
		switch subCommand {
		case "push":
			// Strip trailing slash for gem push: RubyGems' push_command.rb builds
			// the request URL as "#{host}/api/v1/gems" — if host already ends with /,
			// the resulting URL has a double slash which Artifactory rejects with 405.
			hostForPush := strings.TrimRight(sourceURL, "/")
			return append(args, "--host", hostForPush)
		case "install", "fetch":
			return append(args, "--source", sourceURL)
		}
	}
	return args
}

// rubyStripHostTrailingSlash removes the trailing slash from any --host value in args.
// For gem push, RubyGems builds URLs as "#{host}/api/v1/gems" — a trailing slash on
// the host creates a double-slash that Artifactory rejects with 405.
func rubyStripHostTrailingSlash(args []string) []string {
	result := make([]string, len(args))
	copy(result, args)
	for i, a := range result {
		switch {
		case strings.HasPrefix(a, "--host="):
			val := strings.TrimPrefix(a, "--host=")
			result[i] = "--host=" + strings.TrimRight(val, "/")
		case a == "--host" && i+1 < len(result):
			result[i+1] = strings.TrimRight(result[i+1], "/")
		}
	}
	return result
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

// runRubyBinaryCapture executes gem/bundle capturing stdout while still printing it.
// Used for `gem install`/`fetch` to parse installed/downloaded gem names from output.
func runRubyBinaryCapture(tool string, args, extraEnv []string) (string, error) {
	cmd := exec.Command(tool, args...) // #nosec G204
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	// Capture stdout while also printing it to the user's terminal.
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	err := cmd.Run()
	return buf.String(), err
}


// isRubyHelpRequest reports whether the invocation is purely a help request.
func isRubyHelpRequest(subCommand string, args []string) bool {
	if subCommand == "help" {
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
			basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
			extraEnv = append(extraEnv, fmt.Sprintf("GEM_HOST_API_KEY=%s", basicAuth))
			log.Info("Ruby auth [gem]: injecting GEM_HOST_API_KEY (used by gem push on RubyGems >= 3.1; URL-embedded credentials used as primary auth for install/fetch/push)")
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
// Precedence: explicit --repo override (URL constructed from server config) >
// --source/--host/--clear-sources arg > Gemfile `source` line > `gem sources` list.
// Returns empty strings when none is found.
func (rc *RubyCommand) resolveRepo(workingDir string, serverDetails *coreConfig.ServerDetails) (sourceURL, repoKey string) {
	// When --repo is provided, construct the full Artifactory gems URL from server config.
	if rc.repository != "" {
		if serverDetails == nil {
			log.Warn("Ruby: --repo specified but no server details available; using repo key only")
			return "", rc.repository
		}
		repoURL, err := rubyConstructRepoURL(serverDetails, rc.repository)
		if err != nil {
			log.Warn(fmt.Sprintf("Ruby: failed to construct repo URL from server config: %v; using repo key only", err))
			return "", rc.repository
		}
		log.Info(fmt.Sprintf("Ruby: using --repo %q → %s", rc.repository, repoURL))
		return repoURL, rc.repository
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

// rubyConstructRepoURL builds the Artifactory gems API URL from server details and repo name.
// Example: serverURL "https://my.jfrog.io/artifactory/" + repo "gems-virtual"
// → "https://my.jfrog.io/artifactory/api/gems/gems-virtual/"
func rubyConstructRepoURL(serverDetails *coreConfig.ServerDetails, repoName string) (string, error) {
	baseURL := serverDetails.GetArtifactoryUrl()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed = parsed.JoinPath("api/gems", repoName)
	result := parsed.String()
	if !strings.HasSuffix(result, "/") {
		result += "/"
	}
	return result, nil
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
// capturedOutput is the captured stdout from gem commands (empty for bundle commands).
func (rc *RubyCommand) collectBuildInfo(workingDir, subCommand, repoKey string, serverDetails *coreConfig.ServerDetails, capturedOutput string) error {
	switch {
	case rc.nativeTool == toolGem && subCommand == "push":
		// Only gem push records artifacts — it's the point where the .gem enters Artifactory.
		// gem build is local-only; the artifact has no Artifactory path until pushed.
		return rc.collectGemArtifactBuildInfo(workingDir, repoKey, serverDetails)
	case rc.collectsDependencies(subCommand):
		return rc.collectDependencyBuildInfo(workingDir, subCommand, repoKey, serverDetails, capturedOutput)
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

// collectDependencyBuildInfo records dependencies in build-info. For bundle commands,
// this parses Gemfile.lock via FlexPack. For gem install/fetch, it records the specific
// gems that were actually installed/fetched (parsed from the native tool's stdout).
func (rc *RubyCommand) collectDependencyBuildInfo(workingDir, subCommand, repoKey string, serverDetails *coreConfig.ServerDetails, capturedOutput string) error {
	buildName, err := rc.buildConfiguration.GetBuildName()
	if err != nil {
		return err
	}
	buildNumber, err := rc.buildConfiguration.GetBuildNumber()
	if err != nil {
		return err
	}

	// For gem install/fetch: parse stdout to determine exactly what was installed/fetched.
	if rc.nativeTool == toolGem {
		return rc.collectGemInstallDependencies(workingDir, subCommand, buildName, buildNumber, repoKey, serverDetails, capturedOutput)
	}

	// For bundle install/update/lock/add: use the full FlexPack lock-file parser.
	gemConfig := flexpack.GemConfig{WorkingDirectory: workingDir}
	gemConfig.GemGroups = parseGemfileGroups(workingDir)
	gemConfig.InstalledPackages = bundleInstalledPackages(workingDir)

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
		rubyEnrichDepsChecksums(bi.Modules[0].Dependencies, repoKey, directURLDeps, serverDetails)
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

// collectGemInstallDependencies records the gems that were actually installed/fetched.
// Primary mechanism: parse stdout ("Successfully installed X-Y" / "Downloaded X-Y.gem").
// Fallback: explicit -v/--version arg + gem name from args, or gem list query.
func (rc *RubyCommand) collectGemInstallDependencies(workingDir, subCommand, buildName, buildNumber, repoKey string, serverDetails *coreConfig.ServerDetails, capturedOutput string) error {
	// Primary: parse the captured stdout for definitive name:version pairs.
	deps := parseGemCommandOutput(capturedOutput, subCommand)

	// Fallback: if stdout parsing yielded nothing, try extracting from args + gem list.
	if len(deps) == 0 {
		explicitVersion := extractVersionFromArgs(rc.args)
		gemNames := extractGemNamesFromArgs(rc.args)
		for _, name := range gemNames {
			version := explicitVersion
			if version == "" {
				version = queryInstalledGemVersion(name)
			}
			if version == "" {
				log.Debug(fmt.Sprintf("Ruby build-info [gem %s]: could not determine version for %q — skipping", subCommand, name))
				continue
			}
			deps = append(deps, buildinfo.Dependency{
				Id:   fmt.Sprintf("%s:%s", name, version),
				Type: gemDepArtifactType,
			})
		}
	}

	if len(deps) == 0 {
		log.Debug(fmt.Sprintf("Ruby build-info [gem %s]: no gems detected in output or args — empty build-info", subCommand))
		return nil
	}

	moduleID := "gem-" + subCommand
	if customModule := rc.buildConfiguration.GetModule(); customModule != "" {
		moduleID = customModule
	}

	bi := &buildinfo.BuildInfo{
		Name:       buildName,
		Number:     buildNumber,
		Agent:      &buildinfo.Agent{Name: "gem"},
		BuildAgent: &buildinfo.Agent{Name: "Generic", Version: "1.0"},
		Modules: []buildinfo.Module{{
			Id:           moduleID,
			Type:         buildinfo.Gem,
			Dependencies: deps,
		}},
	}

	// Enrich checksums from Artifactory.
	if repoKey != "" && serverDetails != nil {
		rubyEnrichDepsChecksums(bi.Modules[0].Dependencies, repoKey, nil, serverDetails)
	}

	if err := rubySaveBuildInfo(bi, rc.buildConfiguration); err != nil {
		return fmt.Errorf("failed to save RubyGems build info: %w", err)
	}
	log.Info(fmt.Sprintf("RubyGems build info collected (%d gem(s)). Use 'jf rt bp %s %s' to publish.", len(deps), buildName, buildNumber))
	return nil
}

// extractGemNamesFromArgs parses gem names from `gem install/fetch` command args.
// Skips flags (--source, --version, etc.) and their values.
func extractGemNamesFromArgs(args []string) []string {
	var names []string
	skipNext := false
	for i, a := range args {
		if i == 0 {
			continue // skip the subcommand itself (install/fetch)
		}
		if skipNext {
			skipNext = false
			continue
		}
		// Skip flags and their values.
		if strings.HasPrefix(a, "-") {
			// Flags that take a value argument.
			switch a {
			case "--source", "-s", "--host", "--version", "-v", "--platform", "-i",
				"--install-dir", "--bindir", "-n", "--document", "--build-root":
				skipNext = true
			}
			continue
		}
		// Skip anything that looks like a path or URL (not a gem name).
		if strings.Contains(a, "/") || strings.Contains(a, "\\") {
			continue
		}
		names = append(names, a)
	}
	return names
}

// parseGemCommandOutput parses gem install/fetch stdout to extract name:version pairs.
//
// gem install prints: "Successfully installed <name>-<version>"
// gem fetch prints:   "Downloaded <name>-<version>.gem" or "Fetching: <name>-<version>.gem"
//
// This is the primary (most accurate) mechanism — it reflects what actually happened.
func parseGemCommandOutput(output, subCommand string) []buildinfo.Dependency {
	if output == "" {
		return nil
	}
	seen := make(map[string]bool)
	var deps []buildinfo.Dependency
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		var nameVersion string
		switch {
		case strings.HasPrefix(line, "Successfully installed "):
			// "Successfully installed colorize-1.1.0"
			nameVersion = strings.TrimPrefix(line, "Successfully installed ")
		case strings.HasPrefix(line, "Downloaded "):
			// "Downloaded colorize-1.1.0.gem"
			nameVersion = strings.TrimSuffix(strings.TrimPrefix(line, "Downloaded "), ".gem")
		case strings.HasPrefix(line, "Fetching: "):
			// "Fetching: colorize-1.1.0.gem (100%)" — older gem versions
			nameVersion = strings.TrimPrefix(line, "Fetching: ")
			if idx := strings.Index(nameVersion, ".gem"); idx > 0 {
				nameVersion = nameVersion[:idx]
			}
		default:
			continue
		}
		name, version := splitGemNameVersion(nameVersion)
		if name == "" || version == "" {
			continue
		}
		depID := fmt.Sprintf("%s:%s", name, version)
		if seen[depID] {
			continue
		}
		seen[depID] = true
		deps = append(deps, buildinfo.Dependency{
			Id:   depID,
			Type: gemDepArtifactType,
		})
	}
	return deps
}

// splitGemNameVersion splits "colorize-1.1.0" into ("colorize", "1.1.0").
// Gem names can contain hyphens (e.g., "rspec-core"), so we split on the LAST
// hyphen that is followed by a digit.
func splitGemNameVersion(s string) (name, version string) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			return s[:i], s[i+1:]
		}
	}
	return "", ""
}

// extractVersionFromArgs extracts an explicit version from -v/--version flags in args.
func extractVersionFromArgs(args []string) string {
	for i, a := range args {
		switch {
		case (a == "-v" || a == "--version") && i+1 < len(args):
			return strings.TrimSpace(args[i+1])
		case strings.HasPrefix(a, "--version="):
			return strings.TrimSpace(strings.TrimPrefix(a, "--version="))
		case strings.HasPrefix(a, "-v") && len(a) > 2 && a[2] != '-':
			// -v1.0.0 form (unusual but valid)
			return strings.TrimSpace(a[2:])
		}
	}
	return ""
}

// queryInstalledGemVersion queries the installed version of a gem via `gem list --exact <name>`.
// Used as a fallback when stdout parsing doesn't yield results.
// Returns the latest installed version or empty string if not found.
func queryInstalledGemVersion(name string) string {
	cmd := exec.Command("gem", "list", "--exact", name)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output format: "colorize (1.1.0, 0.8.1)" or "colorize (1.1.0)"
	line := strings.TrimSpace(string(out))
	openParen := strings.Index(line, "(")
	closeParen := strings.Index(line, ")")
	if openParen == -1 || closeParen == -1 || closeParen <= openParen {
		return ""
	}
	versions := line[openParen+1 : closeParen]
	parts := strings.Split(versions, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

// collectGemArtifactBuildInfo records the .gem artifact uploaded by `gem push`.
func (rc *RubyCommand) collectGemArtifactBuildInfo(workingDir, repoKey string, serverDetails *coreConfig.ServerDetails) error {
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

	if repoKey != "" && serverDetails != nil {
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

// rubyCollectGemArtifacts locates the .gem file from the `gem push` command args.
func rubyCollectGemArtifacts(workingDir string, args []string) ([]buildinfo.Artifact, error) {
	var gemFiles []string
	for _, a := range args {
		if strings.HasSuffix(a, ".gem") && !strings.HasPrefix(a, "-") {
			p := a
			if !filepath.IsAbs(p) {
				p = filepath.Join(workingDir, a)
			}
			gemFiles = append(gemFiles, p)
		}
	}

	var artifacts []buildinfo.Artifact
	for _, path := range gemFiles {
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

// parseGemfileGroups parses the Gemfile to extract gem → group mappings.
// Returns a map where keys are gem names and values are their Bundler groups.
// Gems outside any group block get ["production"]. Gems inside `group :dev do...end`
// get ["development"], etc. Gems in multiple groups get all of them.
func parseGemfileGroups(workingDir string) map[string][]string {
	gemfilePath := filepath.Join(workingDir, "Gemfile")
	data, err := os.ReadFile(gemfilePath)
	if err != nil {
		return nil
	}

	groups := make(map[string][]string)
	var currentGroups []string // nil = top level (production)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect `group :development do` or `group :development, :test do`
		if strings.HasPrefix(line, "group") && strings.HasSuffix(line, "do") {
			currentGroups = parseGroupNames(line)
			continue
		}

		// Detect `end` closing a group block.
		if line == "end" && currentGroups != nil {
			currentGroups = nil
			continue
		}

		// Detect `gem "name"` declarations.
		gemName := parseGemDeclaration(line)
		if gemName == "" {
			continue
		}

		// Inline group: `gem "rspec", group: :test` or `gem "rspec", groups: [:test, :development]`
		if inlineGroups := parseInlineGroups(line); len(inlineGroups) > 0 {
			groups[gemName] = inlineGroups
		} else if currentGroups != nil {
			groups[gemName] = currentGroups
		} else {
			groups[gemName] = []string{"production"}
		}
	}

	if len(groups) == 0 {
		return nil
	}
	return groups
}

// parseGroupNames extracts group names from `group :dev, :test do`.
func parseGroupNames(line string) []string {
	// Strip "group " prefix and " do" suffix.
	line = strings.TrimPrefix(line, "group")
	line = strings.TrimSuffix(line, "do")
	line = strings.TrimSpace(line)

	var result []string
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, ":")
		part = strings.Trim(part, `"'`)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// parseGemDeclaration extracts the gem name from `gem "name"` or `gem 'name'`.
func parseGemDeclaration(line string) string {
	if !strings.HasPrefix(line, "gem ") && !strings.HasPrefix(line, "gem\t") {
		return ""
	}
	rest := strings.TrimPrefix(line, "gem")
	rest = strings.TrimSpace(rest)
	// Extract quoted name.
	if len(rest) < 3 {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	endIdx := strings.IndexByte(rest[1:], quote)
	if endIdx == -1 {
		return ""
	}
	return rest[1 : endIdx+1]
}

// parseInlineGroups handles `gem "x", group: :test` or `gem "x", groups: [:dev, :test]`.
func parseInlineGroups(line string) []string {
	// Look for group: or groups: in the line.
	idx := strings.Index(line, "group:")
	if idx == -1 {
		idx = strings.Index(line, "groups:")
		if idx == -1 {
			return nil
		}
	}
	rest := line[idx:]
	colonIdx := strings.IndexByte(rest, ':')
	if colonIdx == -1 {
		return nil
	}
	rest = strings.TrimSpace(rest[colonIdx+1:])

	// Handle array form: [:dev, :test]
	if strings.HasPrefix(rest, "[") {
		rest = strings.TrimPrefix(rest, "[")
		rest = strings.TrimSuffix(strings.TrimSpace(rest), "]")
		// Remove trailing stuff after the bracket
		if closeIdx := strings.IndexByte(rest, ']'); closeIdx != -1 {
			rest = rest[:closeIdx]
		}
	}

	var result []string
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, ":")
		part = strings.Trim(part, `"'`)
		// Remove trailing non-alphanumeric (e.g., closing bracket remnants)
		part = strings.TrimRight(part, " \t])")
		if part != "" && !strings.Contains(part, " ") {
			result = append(result, part)
		}
	}
	return result
}

// rubyDepEntry associates a dependency index with its gem filename prefix for enrichment.
type rubyDepEntry struct {
	idx    int
	prefix string // "<name>-<version>" used to match the .gem filename
}

// rubyEnrichDepsChecksums enriches dependency checksums using a hybrid approach:
// 1. Try local gem cache first (fast, no network)
// 2. Fall back to AQL for any that couldn't be resolved locally
// GIT/PATH deps (in directURLDeps) are skipped since they are not stored in Artifactory.
func rubyEnrichDepsChecksums(deps []buildinfo.Dependency, repoKey string, directURLDeps map[string]string, serverDetails *coreConfig.ServerDetails) {
	if len(deps) == 0 {
		return
	}

	var entries []rubyDepEntry
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
		entries = append(entries, rubyDepEntry{i, name + "-" + version})
	}
	if len(entries) == 0 {
		return
	}

	// Phase 1: Try local gem cache.
	cacheDir := rubyGemCacheDir()
	localHits := 0
	var needsAQL []rubyDepEntry
	for _, e := range entries {
		if cacheDir == "" {
			needsAQL = append(needsAQL, e)
			continue
		}
		gemFile := filepath.Join(cacheDir, e.prefix+".gem")
		checksum, err := rubyFileChecksums(gemFile)
		if err == nil {
			deps[e.idx].Sha1 = checksum.Sha1
			deps[e.idx].Md5 = checksum.Md5
			deps[e.idx].Sha256 = checksum.Sha256
			localHits++
			log.Debug(fmt.Sprintf("Checksum from local cache: %s", e.prefix))
		} else {
			needsAQL = append(needsAQL, e)
		}
	}
	if localHits > 0 {
		log.Info(fmt.Sprintf("Resolved %d/%d dependency checksums from local gem cache", localHits, len(entries)))
	}

	// Phase 2: AQL fallback for remaining deps (also provides repo path).
	if len(needsAQL) == 0 && repoKey == "" {
		return
	}
	// Even locally-resolved deps need repo path from AQL if we have a repo key.
	entriesToQuery := needsAQL
	if repoKey != "" {
		entriesToQuery = entries
	}
	if len(entriesToQuery) == 0 || serverDetails == nil {
		return
	}
	rubyEnrichDepsViaAQL(deps, entriesToQuery, repoKey, serverDetails)
}

// rubyGemCacheDir returns the local RubyGems cache directory.
// Typically ~/.local/share/gem/ruby/<version>/cache or from `gem env gemdir`/cache.
func rubyGemCacheDir() string {
	out, err := exec.Command("gem", "env", "gemdir").Output()
	if err != nil {
		log.Debug("Could not determine gem cache dir: " + err.Error())
		return ""
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "cache")
	if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
		return dir
	}
	return ""
}

// rubyEnrichDepsViaAQL fetches checksums and repo paths from Artifactory via batched AQL.
func rubyEnrichDepsViaAQL(deps []buildinfo.Dependency, entries []rubyDepEntry, repoKey string, serverDetails *coreConfig.ServerDetails) {
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

	var orClauses []string
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.prefix] {
			continue
		}
		seen[e.prefix] = true
		orClauses = append(orClauses, fmt.Sprintf(`{"name":{"$match":%q}}`, e.prefix+"*.gem"))
	}
	aqlQuery := fmt.Sprintf(
		`items.find({"repo":%q,"$or":[%s]}).include("name","path","actual_sha1","actual_md5","sha256")`,
		searchRepo, strings.Join(orClauses, ","),
	)
	log.Debug(fmt.Sprintf("AQL fallback query for %d deps (repo: %s)", len(entries), searchRepo))

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
			Path       string `json:"path"`
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
			if r.Name == e.prefix+".gem" || strings.HasPrefix(r.Name, e.prefix+"-") {
				if deps[e.idx].Sha1 == "" {
					deps[e.idx].Sha1 = r.ActualSha1
					deps[e.idx].Md5 = r.ActualMd5
					if r.Sha256 != "" && deps[e.idx].Sha256 == "" {
						deps[e.idx].Sha256 = r.Sha256
					}
				}
				if r.Path != "" && r.Path != "." {
					deps[e.idx].Repository = searchRepo + "/" + r.Path + "/" + r.Name
				} else {
					deps[e.idx].Repository = searchRepo + "/" + r.Name
				}
				enriched++
				break
			}
		}
	}

	if enriched > 0 {
		log.Info(fmt.Sprintf("Enriched %d/%d dependencies via AQL (repo: %s)", enriched, len(entries), searchRepo))
	} else {
		log.Debug(fmt.Sprintf("No dependencies enriched via AQL from repo %s — gems may not be cached yet", searchRepo))
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
