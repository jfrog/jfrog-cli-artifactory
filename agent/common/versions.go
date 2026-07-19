package common

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// DeleteVersion deletes the entire version directory for a package (plugin, skill, etc.)
// at {artURL}{repoKey}/{slug}/{version}/ using an HTTP DELETE request.
func DeleteVersion(serverDetails *config.ServerDetails, repoKey, slug, version string) error {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create service manager for deletion: %w", err)
	}
	artURL := clientutils.AddTrailingSlashIfNeeded(sm.GetConfig().GetServiceDetails().GetUrl())
	deletePath := fmt.Sprintf("%s%s/%s/%s/", artURL, repoKey, slug, version)
	httpDetails := sm.GetConfig().GetServiceDetails().CreateHttpClientDetails()
	resp, body, err := sm.Client().SendDelete(deletePath, nil, &httpDetails)
	if err != nil {
		return fmt.Errorf("failed to delete %s/%s/%s: %w", repoKey, slug, version, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to delete %s/%s/%s: HTTP %d — %s", repoKey, slug, version, resp.StatusCode, string(body))
	}
	return nil
}

// ErrVersionExistenceUnknown indicates the version path could not be checked (e.g. network
// error or an Artifactory response whose HTTP status could not be read). Callers should not
// treat this as "version does not exist".
var ErrVersionExistenceUnknown = errors.New("package version existence could not be determined")

// jfrogClientResponseErrPrefix is the prefix used by errorutils.GenerateResponseError in jfrog-client-go.
// There is no exported *ResponseError type in jfrog-client-go today; this prefix is the stable fallback.
const jfrogClientResponseErrPrefix = "server response: "

// httpStatusCoder may be implemented by jfrog-client-go response errors when exposed.
type httpStatusCoder interface {
	StatusCode() int
}

// PackageVersionExists reports whether {repoKey}/{slug}/{version}/ exists in Artifactory
// using the generic storage API. A 404 on the path is reported as "does not exist".
// When the HTTP status cannot be determined, returns ErrVersionExistenceUnknown.
func PackageVersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	_, err = serviceManager.FolderInfo(fmt.Sprintf("%s/%s/%s", repoKey, slug, version))
	if err == nil {
		return true, nil
	}
	if statusCode, hasStatusCode := jfrogClientHTTPStatusCode(err); hasStatusCode {
		if statusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return false, fmt.Errorf("%w: %w", ErrVersionExistenceUnknown, err)
}

// IsHTTPNotFound reports whether err is an HTTP 404 from jfrog-client-go (typed StatusCode or
// errorutils.GenerateResponseError). Returns false when the status cannot be determined.
func IsHTTPNotFound(err error) bool {
	code, ok := jfrogClientHTTPStatusCode(err)
	return ok && code == http.StatusNotFound
}

// jfrogClientHTTPStatusCode extracts an HTTP status from jfrog-client-go errors by walking the chain.
// It prefers errors.As against types that implement StatusCode(). When jfrog-client-go does not expose
// a typed response error, it falls back to parsing messages from errorutils.GenerateResponseError only.
// Remove statusFromJFrogGenerateResponseError once jfrog-client-go exports a response error with StatusCode().
// If neither applies, ok is false and callers must not infer 404 from the error text.
func jfrogClientHTTPStatusCode(err error) (int, bool) {
	for chainedErr := err; chainedErr != nil; chainedErr = errors.Unwrap(chainedErr) {
		var statusCoder httpStatusCoder
		if errors.As(chainedErr, &statusCoder) {
			if code := statusCoder.StatusCode(); isValidHTTPStatusCode(code) {
				return code, true
			}
		}
		if code, hasStatusCode := statusFromJFrogGenerateResponseError(chainedErr); hasStatusCode {
			return code, true
		}
	}
	return 0, false
}

// statusFromJFrogGenerateResponseError parses errors produced by errorutils.GenerateResponseError.
// Accepted example: "server response: 404 Not Found"
// Accepted with body: "server response: 404 Not Found\n{\"errors\":[]}"
// Not accepted: "failed to access repo 404-something", "connection refused"
func statusFromJFrogGenerateResponseError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, jfrogClientResponseErrPrefix) {
		return 0, false
	}
	statusLine := strings.TrimSpace(msg[len(jfrogClientResponseErrPrefix):])
	if newline := strings.IndexByte(statusLine, '\n'); newline >= 0 {
		statusLine = statusLine[:newline]
	}
	codeStr, _, found := strings.Cut(statusLine, " ")
	if !found {
		codeStr = statusLine
	}
	statusCode, convErr := strconv.Atoi(codeStr)
	if convErr != nil || !isValidHTTPStatusCode(statusCode) {
		return 0, false
	}
	return statusCode, true
}

func isValidHTTPStatusCode(code int) bool {
	return code >= 100 && code <= 599
}

// PluginVersion holds metadata about a published plugin version.
type PluginVersion struct {
	Version string
}

// ListPluginVersions lists all versions of a plugin in Artifactory by reading the directory structure.
// It returns an empty slice if the plugin has no versions. Errors reading the directory are logged but not fatal.
func ListPluginVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]PluginVersion, error) {
	serviceManager, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create service manager: %w", err)
	}

	folderInfo, err := serviceManager.FolderInfo(fmt.Sprintf("%s/%s", repoKey, slug))
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for %s/%s: %w", repoKey, slug, err)
	}

	var versions []PluginVersion
	for _, child := range folderInfo.Children {
		if child.Folder {
			versions = append(versions, PluginVersion{Version: child.Uri[1:]}) // Remove leading slash
		}
	}

	return versions, nil
}

// PublishableVersion holds metadata about any published version (plugin, skill, etc).
type PublishableVersion struct {
	Version string
}

// ListVersionsFunc is a function type that lists available versions for a package.
type ListVersionsFunc func(serverDetails *config.ServerDetails, repoKey, slug string) ([]PublishableVersion, error)

// ResolveMissingVersionOpts configures version resolution for publish commands.
type ResolveMissingVersionOpts struct {
	ServerDetails *config.ServerDetails
	RepoKey       string
	Slug          string
	Quiet         bool
	ListVersions  ListVersionsFunc // Fetches available versions from Artifactory
}

// ResolveMissingVersion resolves a version when neither --version flag nor manifest provides one.
// - Interactive mode: fetches and shows existing versions, then prompts user for a new version
// - Quiet/CI mode: auto-increments the next minor version OR defaults to 0.1.0
// This function is the single source of truth for publish version resolution across all package types.
func ResolveMissingVersion(opts ResolveMissingVersionOpts) (string, error) {
	versionStrs := fetchExistingVersionStrings(opts)

	if opts.Quiet {
		return resolveQuietDefaultVersion(versionStrs)
	}
	printVersionResolutionBanner(opts.Slug, versionStrs)
	return promptForNewVersion()
}

// fetchExistingVersionStrings fetches published versions for the slug. A lookup failure is
// logged, not fatal, and yields an empty slice (treated the same as "nothing published yet").
func fetchExistingVersionStrings(opts ResolveMissingVersionOpts) []string {
	versions, err := opts.ListVersions(opts.ServerDetails, opts.RepoKey, opts.Slug)
	if err != nil {
		log.Debug("Could not fetch existing versions:", err.Error())
	}
	versionStrs := make([]string, len(versions))
	for index, version := range versions {
		versionStrs[index] = version.Version
	}
	return versionStrs
}

// latestOrFallback returns the latest semver in versionStrs, or its last element if the
// versions can't be parsed as semver. Requires len(versionStrs) > 0.
func latestOrFallback(versionStrs []string) string {
	if len(versionStrs) == 0 {
		return ""
	}
	latest, err := LatestVersion(versionStrs)
	if err != nil {
		log.Debug("Could not determine latest version:", err.Error())
		return versionStrs[len(versionStrs)-1]
	}
	return latest
}

// resolveQuietDefaultVersion picks a version automatically for --quiet/CI mode: the next
// minor version after the latest existing one, or 0.1.0 when nothing is published yet.
func resolveQuietDefaultVersion(versionStrs []string) (string, error) {
	if len(versionStrs) == 0 {
		log.Info("No version specified and no existing versions found. Defaulting to 0.1.0")
		return "0.1.0", nil
	}
	latest := latestOrFallback(versionStrs)
	next, err := NextMinorVersion(latest)
	if err != nil {
		return "", fmt.Errorf("failed to compute next version from '%s': %w", latest, err)
	}
	log.Info(fmt.Sprintf("No version specified. Auto-incrementing to %s", next))
	return next, nil
}

// printVersionResolutionBanner prints the interactive-mode "no version specified" banner,
// listing existing versions when any were found.
func printVersionResolutionBanner(slug string, versionStrs []string) {
	log.Info("No version specified in manifest or --version flag.")
	if len(versionStrs) == 0 {
		log.Info(fmt.Sprintf("No existing versions found for '%s'.", slug))
		return
	}
	log.Info(fmt.Sprintf("Existing versions: %v  (latest: %s)", versionStrs, latestOrFallback(versionStrs)))
}

// promptForNewVersion asks the user to type a version to publish; empty input aborts.
func promptForNewVersion() (string, error) {
	newVersion, err := PromptLine("Enter version to publish: ")
	if err != nil {
		return "", err
	}
	if newVersion == "" {
		return "", fmt.Errorf("no version provided, aborting")
	}
	return newVersion, nil
}
