package flexpack

// Find the Gradle deploy repository key by scanning properties, build/settings scripts, and init scripts.

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	buildinfoflexpack "github.com/jfrog/build-info-go/flexpack/gradle"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// It does not check for specific environment variables
func getGradleDeployRepository(workingDir, rootDir, version string) (string, error) {
	if err := ValidateWorkingDirectory(workingDir); err != nil {
		return "", err
	}

	isSnapshot := strings.Contains(strings.ToLower(version), keywordSnapshot)
	props := collectAllGradleProperties(workingDir)

	// Add default properties for path resolution, ex: ${rootDir}
	if _, ok := props[rootDirProp]; !ok {
		if rootDir != "" {
			props[rootDirProp] = rootDir
		} else {
			props[rootDirProp] = workingDir
		}
	}
	if _, ok := props[projectDirProp]; !ok {
		props[projectDirProp] = workingDir
	}

	if repo, err := findRepoInProperties(props, isSnapshot); err == nil && repo != "" {
		log.Debug("Found repository from properties: " + repo)
		return repo, nil
	}

	buildGradlePath, _, err := buildinfoflexpack.FindGradleFile(workingDir, "build")
	if err == nil {
		if repo, err := checkGradleScript(buildGradlePath, isSnapshot, props); err == nil && repo != "" {
			log.Debug("Found repository from publishing configuration in " + filepath.Base(buildGradlePath) + ": " + repo)
			return repo, nil
		}
	}

	// Check settings.gradle or settings.gradle.kts
	settingsGradlePath, _, err := buildinfoflexpack.FindGradleFile(workingDir, "settings")
	if err == nil {
		if repo, err := checkGradleScript(settingsGradlePath, isSnapshot, props); err == nil && repo != "" {
			log.Debug("Found repository from " + filepath.Base(settingsGradlePath) + ": " + repo)
			return repo, nil
		}
	}

	// Check init scripts in GRADLE_USER_HOME
	gradleUserHome := buildinfoflexpack.GetGradleUserHome()
	if gradleUserHome != "" {
		if repoKey, err := checkInitScripts(gradleUserHome, isSnapshot, props); err == nil && repoKey != "" {
			log.Debug("Found repository from init scripts: " + repoKey)
			return repoKey, nil
		}
	}
	return "", fmt.Errorf("no deployment repository found in Gradle configuration or environment")
}

// It works only if the key has "repo", "url", or "deploy" in the name
func findRepoInProperties(props map[string]string, isSnapshot bool) (string, error) {
	var candidates []string
	seen := make(map[string]bool)

	// Sort keys for deterministic behavior, in case of multiple keys with same name
	var keys []string
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := props[key]
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" || val == "" {
			continue
		}

		keyLower := strings.ToLower(key)
		if strings.Contains(keyLower, keywordRepo) || strings.Contains(keyLower, keywordUrl) ||
			strings.Contains(keyLower, keywordDeploy) {

			if _, err := strconv.ParseBool(val); err == nil {
				continue
			}

			// Filter based on version type snapshot/release
			if isSnapshot && strings.Contains(keyLower, keywordRelease) && !strings.Contains(keyLower, keywordSnapshot) {
				continue
			}
			if !isSnapshot && strings.Contains(keyLower, keywordSnapshot) && !strings.Contains(keyLower, keywordRelease) {
				continue
			}

			if repoKey, err := extractRepoKeyCandidate(val); err == nil && repoKey != "" {
				if !seen[repoKey] {
					candidates = append(candidates, repoKey)
					seen[repoKey] = true
				}
			}
		}
	}

	return selectBestRepo(candidates, isSnapshot)
}

func extractRepoKeyCandidate(val string) (string, error) {
	if strings.Contains(val, "://") || strings.HasPrefix(val, "/") {
		if strings.HasPrefix(val, "./") || strings.HasPrefix(val, "../") {
			return "", nil
		}
		return extractRepoKeyFromArtifactoryUrl(val)
	}
	if !strings.Contains(val, "/") && !strings.Contains(val, ":") {
		return val, nil
	}
	return "", nil
}

func extractRepoKeyFromArtifactoryUrl(repoUrl string) (string, error) {
	repoUrl = strings.TrimSpace(repoUrl)

	u, err := url.Parse(repoUrl)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Path: /artifactory/api/<type>/REPO-KEY
	if len(segments) >= 4 && segments[len(segments)-3] == keywordApi {
		apiType := segments[len(segments)-2]
		if apiType == keywordMaven || apiType == keywordGradle || apiType == keywordIvy {
			return segments[len(segments)-1], nil
		}
	}

	// Standard format: /artifactory/REPO-KEY
	if len(segments) >= 2 {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
		}
	}
	return "", fmt.Errorf("unable to extract repository key from URL: %s (check repository URL format)", repoUrl)
}

func checkInitScripts(gradleUserHome string, isSnapshot bool, props map[string]string) (string, error) {
	// Sanitize gradle user home path - returns a new untainted value
	sanitizedGradleHome, err := buildinfoflexpack.SanitizePath(gradleUserHome)
	if err != nil {
		return "", fmt.Errorf("invalid gradle user home path: %w", err)
	}

	// 1. Check init.gradle or init.gradle.kts
	// FindGradleFile already sanitizes and validates paths
	initGradlePath, _, err := buildinfoflexpack.FindGradleFile(sanitizedGradleHome, "init")
	if err == nil {
		if repo, err := checkGradleScript(initGradlePath, isSnapshot, props); err == nil {
			return repo, nil
		}
	}

	// 2. Check init.d directory - sanitize and validate the path
	initDDirPath := filepath.Join(sanitizedGradleHome, initDDirName)
	sanitizedInitDDir, err := buildinfoflexpack.SanitizeAndValidatePath(initDDirPath, sanitizedGradleHome)
	if err != nil {
		return "", fmt.Errorf("invalid init.d directory path: %w", err)
	}

	entries, err := os.ReadDir(sanitizedInitDDir)
	if err == nil {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		// Iterate backwards to find the highest precedence script that has a repo
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".gradle") || strings.HasSuffix(entry.Name(), ".gradle.kts")) {
				// Validate entry name doesn't contain path separators
				if strings.ContainsAny(entry.Name(), `/\`) {
					continue
				}
				// Sanitize and validate script path
				scriptPath := filepath.Join(sanitizedInitDDir, entry.Name())
				sanitizedScriptPath, err := buildinfoflexpack.SanitizeAndValidatePath(scriptPath, sanitizedInitDDir)
				if err != nil {
					log.Debug("Skipping invalid script path: " + scriptPath)
					continue
				}
				if repo, err := checkGradleScript(sanitizedScriptPath, isSnapshot, props); err == nil {
					return repo, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no repository found in init scripts")
}

func checkGradleScript(path string, isSnapshot bool, props map[string]string) (string, error) {
	// Sanitize the path before reading
	cleanPath, err := buildinfoflexpack.SanitizePath(path)
	if err != nil {
		return "", fmt.Errorf("invalid script path: %w", err)
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	isKts := strings.HasSuffix(cleanPath, ".kts")
	return findRepoInGradleScript(content, isKts, props, isSnapshot, cleanPath)
}

func findRepoInGradleScript(content []byte, isKts bool, props map[string]string, isSnapshot bool, scriptPath string) (string, error) {
	visited := make(map[string]bool)
	if scriptPath != "" {
		if absPath, err := buildinfoflexpack.SanitizePath(scriptPath); err == nil {
			visited[absPath] = true
		}
	}
	return findRepoInGradleScriptRecursive(content, isKts, props, isSnapshot, scriptPath, visited)
}

func findRepoInGradleScriptRecursive(content []byte, isKts bool, props map[string]string, isSnapshot bool, scriptPath string, visited map[string]bool) (string, error) {
	localProps := extractPropertiesFromScript(string(content))
	combinedProps := make(map[string]string)
	for k, v := range props {
		combinedProps[k] = v
	}
	for k, v := range localProps {
		if _, exists := combinedProps[k]; !exists {
			combinedProps[k] = v
		}
	}

	matches := findUrlsInGradleScript(content, isKts)
	var resolvedUrls []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			raw := strings.TrimSpace(string(match[1]))
			if raw == "" {
				continue
			}
			resolvedStr := resolveGradleProperty(raw, combinedProps)
			if strings.Contains(resolvedStr, "${") {
				log.Debug("Skipping unresolved property reference: " + resolvedStr)
				continue
			}
			if strings.HasPrefix(resolvedStr, "./") || strings.HasPrefix(resolvedStr, "../") {
				// Skip relative paths as they're not Artifactory URLs
				continue
			}
			if seen[resolvedStr] {
				continue
			}
			seen[resolvedStr] = true
			resolvedUrls = append(resolvedUrls, resolvedStr)
		}
	}

	if len(resolvedUrls) > 0 {
		return findRepositoryKeyFromMatches(resolvedUrls, scriptPath, isSnapshot)
	}

	appliedScripts := collectAppliedScripts(content, isKts, combinedProps, scriptPath)
	for _, appliedScript := range appliedScripts {
		// Sanitize the applied script path
		absPath, err := buildinfoflexpack.SanitizePath(appliedScript)
		if err != nil {
			log.Debug("Skipping invalid applied script path: " + appliedScript)
			continue
		}

		if visited[absPath] {
			log.Debug("Skipping already visited script: " + appliedScript)
			continue
		}
		visited[absPath] = true

		appliedContent, err := os.ReadFile(absPath)
		if err != nil {
			log.Debug("Failed to read applied script " + appliedScript + ": " + err.Error())
			continue
		}

		isAppliedKts := strings.HasSuffix(absPath, ".kts")
		repo, err := findRepoInGradleScriptRecursive(appliedContent, isAppliedKts, combinedProps, isSnapshot, absPath, visited)
		if err == nil && repo != "" {
			log.Debug("Found repository in applied script " + filepath.Base(absPath) + ": " + repo)
			return repo, nil
		}
	}
	return "", fmt.Errorf("no repository found in %s", scriptPath)
}

func findRepositoryKeyFromMatches(repoUrls []string, sourceName string, isSnapshot bool) (string, error) {
	var candidates []string

	for _, repoValue := range repoUrls {
		repoValue = strings.TrimSpace(repoValue)
		if repoValue == "" {
			continue
		}

		repoKey, err := extractRepoKeyCandidate(repoValue)
		if err != nil {
			log.Debug("Failed to extract repo key from: " + repoValue + " - " + err.Error())
			continue
		}

		if repoKey != "" {
			candidates = append(candidates, repoKey)
		}
	}

	if best, err := selectBestRepo(candidates, isSnapshot); err == nil && best != "" {
		log.Debug("Selected repository from " + sourceName + ": " + best)
		return best, nil
	}
	return "", fmt.Errorf("no matching repository found in %s (isSnapshot: %v)", sourceName, isSnapshot)
}

func selectBestRepo(candidates []string, isSnapshot bool) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no candidates provided")
	}
	var snapshotCandidates, releaseCandidates, generalCandidates []string
	seen := make(map[string]bool)

	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true

		cLower := strings.ToLower(c)
		switch {
		case strings.Contains(cLower, "snapshot"):
			snapshotCandidates = append(snapshotCandidates, c)
		case strings.Contains(cLower, "release"):
			releaseCandidates = append(releaseCandidates, c)
		default:
			generalCandidates = append(generalCandidates, c)
		}
	}

	if isSnapshot {
		if len(snapshotCandidates) > 0 {
			return snapshotCandidates[0], nil
		}
		if len(generalCandidates) > 0 {
			return generalCandidates[0], nil
		}
	} else {
		if len(releaseCandidates) > 0 {
			return releaseCandidates[0], nil
		}
		if len(generalCandidates) > 0 {
			return generalCandidates[0], nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return "", fmt.Errorf("no suitable repository found")
}
