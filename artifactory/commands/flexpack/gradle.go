package flexpack

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/build"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/flexpack"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	gradleTaskPublish             = "publish"
	gradleTaskPublishToMavenLocal = "publishToMavenLocal"
	gradlePropertiesTimeout       = 1 * time.Minute
	artifactSearchClockSkewBuffer = 1 * time.Minute
	gradleEnvPrefixLen            = 19
)

func CollectGradleBuildInfoWithFlexPack(workingDir, buildName, buildNumber string, tasks []string, buildConfiguration *buildUtils.BuildConfiguration) error {
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for working directory: %w", err)
	}
	workingDir = absWorkingDir

	startTime := time.Now()
	config := flexpack.GradleConfig{
		WorkingDirectory:        workingDir,
		IncludeTestDependencies: true,
	}

	gradleFlex, err := flexpack.NewGradleFlexPack(config)
	if err != nil {
		return fmt.Errorf("failed to create Gradle FlexPack: %w", err)
	}

	isPublishCommand := wasPublishCommand(tasks)
	gradleFlex.WasPublishCommand = isPublishCommand

	buildInfo, err := gradleFlex.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info with FlexPack: %w", err)
	}

	if err := saveGradleFlexPackBuildInfo(buildInfo); err != nil {
		log.Warn("Failed to save build info for jfrog-cli compatibility: " + err.Error())
	} else {
		log.Info("Build info saved locally. Use 'jf rt bp " + buildName + " " + buildNumber + "' to publish it to Artifactory.")
	}

	if isPublishCommand {
		if err := setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber, buildConfiguration, buildInfo, startTime); err != nil {
			log.Warn("Failed to set build properties on deployed artifacts: " + err.Error())
		}
	}
	return nil
}

func wasPublishCommand(tasks []string) bool {
	for _, task := range tasks {
		// Handle tasks with project paths (e.g., ":subproject:publish")
		if idx := strings.LastIndex(task, ":"); idx != -1 {
			task = task[idx+1:]
		}

		// Match common Gradle publish tasks
		if task == gradleTaskPublish {
			return true
		}
		if strings.HasPrefix(task, "publishTo") && task != gradleTaskPublishToMavenLocal {
			return true
		}
		// Pattern: publish<Publication>To<Repository>
		if strings.HasPrefix(task, "publish") && strings.Contains(task, "To") {
			if !strings.HasSuffix(task, "Local") {
				return true
			}
		}
	}
	return false
}

func saveGradleFlexPackBuildInfo(buildInfo *entities.BuildInfo) error {
	service := build.NewBuildInfoService()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, "")
	if err != nil {
		return fmt.Errorf("failed to create build: %w", err)
	}
	return buildInstance.SaveBuildInfo(buildInfo)
}

func setGradleBuildPropertiesOnArtifacts(workingDir, buildName, buildNumber string, buildArgs *buildUtils.BuildConfiguration, buildInfo *entities.BuildInfo, startTime time.Time) error {
	serverDetails, err := getGradleServerDetails()
	if err != nil {
		return fmt.Errorf("failed to get server details: %w", err)
	}
	if serverDetails == nil {
		log.Warn("No server details configured, skipping build properties")
		return nil
	}

	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to create services manager: %w", err)
	}

	projectKey := buildArgs.GetProject()
	recentArtifacts, err := searchRecentArtifacts(servicesManager, buildInfo, startTime, workingDir)
	if err != nil {
		return err
	}

	if len(recentArtifacts) == 0 {
		log.Warn("No recently deployed artifacts found")
		return nil
	}

	timestamp := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
	buildProps := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	if projectKey != "" {
		buildProps += fmt.Sprintf(";build.project=%s", projectKey)
	}

	writer, err := content.NewContentWriter(content.DefaultKey, true, false)
	if err != nil {
		return fmt.Errorf("failed to create content writer: %w", err)
	}

	for _, artifact := range recentArtifacts {
		writer.Write(artifact)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close content writer: %w", err)
	}

	reader := content.NewContentReader(writer.GetFilePath(), content.DefaultKey)
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close reader: %s", closeErr))
		}
	}()

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  buildProps,
	}

	_, err = servicesManager.SetProps(propsParams)
	if err != nil {
		return fmt.Errorf("failed to set properties on artifacts: %w", err)
	}

	log.Info("Successfully set build properties on deployed Gradle artifacts")
	return nil
}

func getGradleServerDetails() (*config.ServerDetails, error) {
	serverDetails, err := config.GetDefaultServerConf()
	if err != nil {
		return nil, fmt.Errorf("failed to get server details: %w", err)
	}
	return serverDetails, nil
}

func searchRecentArtifacts(servicesManager artifactory.ArtifactoryServicesManager, buildInfo *entities.BuildInfo, startTime time.Time, workingDir string) ([]specutils.ResultItem, error) {
	var recentArtifacts []specutils.ResultItem
	// The repository typically depends on whether the version is a snapshot or release.
	repoCache := make(map[bool]string)

	for _, module := range buildInfo.Modules {
		if len(module.Artifacts) == 0 {
			continue
		}
		// We assume all artifacts in a module go to the same repo structure
		artifact := module.Artifacts[0]
		parts := strings.Split(module.Id, ":")
		if len(parts) < 3 {
			log.Warn("Skipping module with invalid ID format: " + module.Id)
			continue
		}
		version := parts[2]

		isSnapshot := strings.Contains(strings.ToLower(version), "snapshot")
		targetRepo, ok := repoCache[isSnapshot]
		if !ok {
			var deployErr error
			targetRepo, deployErr = getGradleDeployRepository(workingDir, version)
			if deployErr != nil {
				log.Warn(fmt.Sprintf("Could not determine Gradle deploy repository for module %s: %v", module.Id, deployErr))
				continue
			}
			repoCache[isSnapshot] = targetRepo
		}

		var artifactPath string
		if artifact.Path != "" {
			artifactPath = fmt.Sprintf("%s/%s", targetRepo, artifact.Path)
		} else {
			groupId := parts[0]
			artifactId := parts[1]
			artifactPath = fmt.Sprintf("%s/%s/%s/%s/%s-*",
				targetRepo,
				strings.ReplaceAll(groupId, ".", "/"), artifactId, version, artifactId)
		}

		// Let's use the directory of the artifact to find all related artifacts (jars, poms, etc)
		artifactDir := path.Dir(artifactPath)
		searchPattern := fmt.Sprintf("%s/*", artifactDir)

		searchParams := services.SearchParams{
			CommonParams: &specutils.CommonParams{
				Pattern: searchPattern,
			},
		}
		searchReader, err := servicesManager.SearchFiles(searchParams)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to search for deployed artifacts for module %s: %v", module.Id, err))
			continue
		}

		// Filter to only artifacts modified after the build started
		for item := new(specutils.ResultItem); searchReader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
			modTime, err := parseArtifactModifiedTime(item.Modified)
			if err != nil {
				log.Debug("Could not parse modified time for " + item.Name + ": " + err.Error())
				continue
			}

			// Allow a small buffer for clock skew between build machine and Artifactory server.
			if modTime.After(startTime.Add(-artifactSearchClockSkewBuffer)) {
				recentArtifacts = append(recentArtifacts, *item)
			}
		}
		if closeErr := searchReader.Close(); closeErr != nil {
			log.Debug(fmt.Sprintf("Failed to close search reader: %s", closeErr))
		}
	}
	return recentArtifacts, nil
}

func parseArtifactModifiedTime(modified string) (time.Time, error) {
	// Try common Artifactory time formats
	formats := []string{
		time.RFC3339Nano,                // 2006-01-02T15:04:05.999999999Z07:00
		time.RFC3339,                    // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05.999Z",      // ISO 8601 with milliseconds and Z
		"2006-01-02T15:04:05.000-0700",  // Build info format
		"2006-01-02T15:04:05.999-07:00", // ISO 8601 with milliseconds and timezone
	}

	for _, format := range formats {
		if t, err := time.Parse(format, modified); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse time: %s", modified)
}

// It does not check for specific environment variables
func getGradleDeployRepository(workingDir, version string) (string, error) {
	// Validate working directory
	if workingDir == "" {
		return "", fmt.Errorf("working directory cannot be empty")
	}
	if info, err := os.Stat(workingDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid working directory: %s", workingDir)
	}

	isSnapshot := strings.Contains(strings.ToLower(version), "snapshot")
	props := collectAllGradleProperties(workingDir)

	// Add default properties for path resolution, ex: ${rootDir}
	if _, ok := props["rootDir"]; !ok {
		props["rootDir"] = workingDir
	}
	if _, ok := props["projectDir"]; !ok {
		props["projectDir"] = workingDir
	}

	if repo, err := findRepoInProperties(props, isSnapshot); err == nil && repo != "" {
		log.Debug("Found repository from properties: " + repo)
		return repo, nil
	}

	buildGradlePath := filepath.Join(workingDir, "build.gradle")
	isBuildGradleKts := false
	if _, err := os.Stat(buildGradlePath); os.IsNotExist(err) {
		buildGradlePath = filepath.Join(workingDir, "build.gradle.kts")
		isBuildGradleKts = true
	}

	if content, err := os.ReadFile(buildGradlePath); err == nil {
		if repo, err := findRepoInGradleScript(content, isBuildGradleKts, props, isSnapshot, buildGradlePath); err == nil && repo != "" {
			log.Debug("Found repository from publishing configuration in " + filepath.Base(buildGradlePath) + ": " + repo)
			return repo, nil
		}
	}

	// Check settings.gradle or settings.gradle.kts
	settingsGradlePath := filepath.Join(workingDir, "settings.gradle")
	isSettingsGradleKts := false
	if _, err := os.Stat(settingsGradlePath); os.IsNotExist(err) {
		settingsGradlePath = filepath.Join(workingDir, "settings.gradle.kts")
		isSettingsGradleKts = true
	}

	if content, err := os.ReadFile(settingsGradlePath); err == nil {
		if repo, err := findRepoInGradleScript(content, isSettingsGradleKts, props, isSnapshot, settingsGradlePath); err == nil && repo != "" {
			log.Debug("Found repository from " + filepath.Base(settingsGradlePath) + ": " + repo)
			return repo, nil
		}
	}

	// Check init scripts in GRADLE_USER_HOME
	gradleUserHome := getGradleUserHome()
	if gradleUserHome != "" {
		if repoKey, err := checkInitScripts(gradleUserHome, isSnapshot, props); err == nil && repoKey != "" {
			log.Debug("Found repository from init scripts: " + repoKey)
			return repoKey, nil
		}
	}

	return "", fmt.Errorf("no deployment repository found in Gradle configuration or environment")
}

func collectAllGradleProperties(workingDir string) map[string]string {
	props := make(map[string]string)

	// Helper to merge maps (later sources override earlier ones)
	merge := func(source map[string]string) {
		for k, v := range source {
			if v != "" {
				props[k] = v
			}
		}
	}

	// 1. GRADLE_USER_HOME gradle.properties (lowest priority)
	if home := getGradleUserHome(); home != "" {
		merge(readPropertiesFile(filepath.Join(home, "gradle.properties")))
	}

	// 2. Project gradle.properties
	propsFile := filepath.Join(workingDir, "gradle.properties")
	if _, err := os.Stat(propsFile); err == nil {
		merge(readPropertiesFile(propsFile))
	}

	// 3. get project properties from environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "ORG_GRADLE_PROJECT_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				// Extract key after the prefix
				key := strings.TrimSpace(parts[0][gradleEnvPrefixLen:])
				val := strings.TrimSpace(parts[1])
				if key != "" && val != "" {
					props[key] = val
				}
			}
		}
	}

	// 4. CLI Arguments and Options (-P, -D) (highest priority for command line)
	merge(parsePropertiesFromArgs(os.Args))

	// Parse GRADLE_OPTS and JAVA_OPTS
	if opts := os.Getenv("GRADLE_OPTS"); opts != "" {
		merge(parsePropertiesFromOpts(opts))
	}
	if opts := os.Getenv("JAVA_OPTS"); opts != "" {
		merge(parsePropertiesFromOpts(opts))
	}

	return props
}

func getGradleUserHome() string {
	if home := os.Getenv("GRADLE_USER_HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gradle")
	}
	return ""
}

func readPropertiesFile(path string) map[string]string {
	m := make(map[string]string)
	content, err := os.ReadFile(path)
	if err != nil {
		return m
	}

	re := regexp.MustCompile(`(?m)^\s*([^#=\s:]+)\s*[:=]\s*(.*)$`)
	matches := re.FindAllSubmatch(content, -1)
	for _, match := range matches {
		if len(match) == 3 {
			key := strings.TrimSpace(string(match[1]))
			val := strings.TrimSpace(string(match[2]))
			// Remove quotes if present
			if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
				val = val[1 : len(val)-1]
			}
			if key != "" && val != "" {
				m[key] = val
			}
		}
	}
	return m
}

func parsePropertiesFromArgs(args []string) map[string]string {
	m := make(map[string]string)

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if (strings.HasPrefix(arg, "-P") || strings.HasPrefix(arg, "-D")) && len(arg) > 2 {
			pair := arg[2:]
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				// Remove quotes if present
				if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
					val = val[1 : len(val)-1]
				}
				if key != "" && val != "" {
					m[key] = val
				}
			}
		}
	}
	return m
}

func parsePropertiesFromOpts(opts string) map[string]string {
	args := strings.Fields(opts)
	return parsePropertiesFromArgs(args)
}

// It works only if the key has "repo", "artifactory", "url", or "deploy" in the name
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
		if strings.Contains(keyLower, "repo") || strings.Contains(keyLower, "artifactory") ||
			strings.Contains(keyLower, "url") || strings.Contains(keyLower, "deploy") {

			valLower := strings.ToLower(val)
			if valLower == "true" || valLower == "false" {
				continue
			}

			// Filter based on version type snapshot/release
			if isSnapshot && strings.Contains(keyLower, "release") && !strings.Contains(keyLower, "snapshot") {
				continue
			}
			if !isSnapshot && strings.Contains(keyLower, "snapshot") && !strings.Contains(keyLower, "release") {
				continue
			}

			// Try to extract repo key from value (URL)
			if strings.Contains(val, "://") || strings.HasPrefix(val, "/") {
				// Looks like a URL - validate it's not a relative path
				if strings.HasPrefix(val, "./") || strings.HasPrefix(val, "../") {
					continue
				}
				if repoKey, err := extractRepoKeyFromArtifactoryUrl(val); err == nil && repoKey != "" {
					if !seen[repoKey] {
						candidates = append(candidates, repoKey)
						seen[repoKey] = true
					}
				}
			} else if !strings.Contains(val, "/") && !strings.Contains(val, ":") {
				// Assume repo key
				if !seen[val] {
					candidates = append(candidates, val)
					seen[val] = true
				}
			}
		}
	}

	if len(candidates) > 0 {
		// Pick best candidate (prefer snapshot/release matching)
		for _, c := range candidates {
			cLower := strings.ToLower(c)
			if isSnapshot && strings.Contains(cLower, "snapshot") {
				return c, nil
			}
			if !isSnapshot && strings.Contains(cLower, "release") {
				return c, nil
			}
		}
		return candidates[0], nil
	}

	return "", fmt.Errorf("no repository found in properties")
}

func extractRepoKeyFromArtifactoryUrl(repoUrl string) (string, error) {
	repoUrl = strings.TrimSpace(repoUrl)

	u, err := url.Parse(repoUrl)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Path: /artifactory/api/maven/REPO-KEY
	if len(segments) >= 4 && segments[len(segments)-3] == "api" && segments[len(segments)-2] == "maven" {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
		}
	}

	// Path: /artifactory/api/gradle/REPO-KEY
	if len(segments) >= 4 && segments[len(segments)-3] == "api" && segments[len(segments)-2] == "gradle" {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
		}
	}

	// Path: /artifactory/api/ivy/REPO-KEY
	if len(segments) >= 4 && segments[len(segments)-3] == "api" && segments[len(segments)-2] == "ivy" {
		repoKey := segments[len(segments)-1]
		if repoKey != "" {
			return repoKey, nil
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

func findRepoInGradleScript(content []byte, isKts bool, props map[string]string, isSnapshot bool, scriptPath string) (string, error) {
	visited := make(map[string]bool)
	if scriptPath != "" {
		if absPath, err := filepath.Abs(scriptPath); err == nil {
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
		absPath, err := filepath.Abs(appliedScript)
		if err != nil {
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

		isAppliedKts := strings.HasSuffix(appliedScript, ".kts")
		repo, err := findRepoInGradleScriptRecursive(appliedContent, isAppliedKts, combinedProps, isSnapshot, appliedScript, visited)
		if err == nil && repo != "" {
			log.Debug("Found repository in applied script " + filepath.Base(appliedScript) + ": " + repo)
			return repo, nil
		}
	}

	return "", fmt.Errorf("no repository found in %s", scriptPath)
}

func extractPropertiesFromScript(contentStr string) map[string]string {
	props := make(map[string]string)
	// 1. Extract ext { ... } blocks to define the property value
	extBlocks := extractAllGradleBlocks(contentStr, "ext")
	for _, block := range extBlocks {
		re := regexp.MustCompile(`(?m)^\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*=\s*['"]([^'"]+)['"]`)
		matches := re.FindAllStringSubmatch(block, -1)
		for _, match := range matches {
			if len(match) > 2 {
				props[strings.TrimSpace(match[1])] = match[2]
			}
		}
	}

	// 2. Extract ext.key = "value" or project.ext.key = "value" to define the property key
	re := regexp.MustCompile(`(?m)^\s*(?:project\.)?ext\.([a-zA-Z_][a-zA-Z0-9_.]*)\s*=\s*['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(contentStr, -1)
	for _, match := range matches {
		if len(match) > 2 {
			props[strings.TrimSpace(match[1])] = match[2]
		}
	}
	return props
}

func extractAllGradleBlocks(content, keyword string) []string {
	var blocks []string
	idx := 0
	for {
		block, nextIdx := extractNextGradleBlock(content, keyword, idx)
		if nextIdx == -1 {
			break
		}
		if block != "" {
			blocks = append(blocks, block)
		}
		idx = nextIdx
	}
	return blocks
}

func extractNextGradleBlock(content, keyword string, startIndex int) (string, int) {
	// inString, stringChar: Tracks if we are currently inside a string literal (single ' or double " quotes)
	inString := false
	stringChar := byte(0)
	// inLineComment, inBlockComment: Tracks if we are inside a comment
	inLineComment := false
	inBlockComment := false

	keywordLen := len(keyword)
	mode := 0
	braceStartIdx := -1
	depth := 0

	for i := startIndex; i < len(content); i++ {
		char := content[i]

		if inLineComment {
			if char == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if char == '*' && i+1 < len(content) && content[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		// ex: "Repo\"Name\""
		if inString {
			if char == stringChar && (i == 0 || content[i-1] != '\\') {
				inString = false
			}
			continue
		}

		switch char {
		case '/':
			if i+1 < len(content) {
				if content[i+1] == '/' {
					inLineComment = true
					i++
				}
				if content[i+1] == '*' {
					inBlockComment = true
					i++
				}
			}
		case '"', '\'':
			inString = true
			stringChar = char
			continue
		}

		// Process based on mode
		switch mode {
		case 0: // Search for keyword
			if char == keyword[0] {
				if i+keywordLen <= len(content) && content[i:i+keywordLen] == keyword {
					// Check boundaries
					validStart := (i == 0) || isDelimiter(content[i-1])
					validEnd := (i+keywordLen == len(content)) || isDelimiter(content[i+keywordLen])

					if validStart && validEnd {
						mode = 1
						i += keywordLen - 1
					}
				}
			}
		case 1: // Search for opening brace
			switch char {
			case '{':
				mode = 2
				depth = 1
				braceStartIdx = i
			default:
				if !isWhitespace(char) {
					// Found unexpected char before {, reset
					mode = 0
				}
			}
		case 2: // Search for closing brace
			switch char {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return content[braceStartIdx+1 : i], i + 1
				}
			}
		}
	}

	return "", -1
}

func isDelimiter(b byte) bool {
	switch b {
	case '{', '}', '(', ')', ';', ',':
		return true
	}
	return isWhitespace(b)
}

func isWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// It attempts to locate the publishing { repositories { ... } } block and find the urls in it
// Covers: url = "..."  ,  url "..."  ,  url = uri("...")  ,  url.set(uri("..."))  ,  maven { url = ... }
func findUrlsInGradleScript(content []byte, isKts bool) [][][]byte {
	contentStr := string(content)
	var combinedRepos string

	collectRepos := func(parentKeyword string) {
		blocks := extractAllGradleBlocks(contentStr, parentKeyword)
		for _, block := range blocks {
			repoBlocks := extractAllGradleBlocks(block, "repositories")
			for _, repoBlock := range repoBlocks {
				combinedRepos += repoBlock + "\n"
			}
		}
	}

	collectRepos("publishing")
	// 2. Extract from uploadArchives blocks (legacy maven)
	collectRepos("uploadArchives")
	// 3. Extract from dependencyResolutionManagement blocks (Gradle 7.0+)
	collectRepos("dependencyResolutionManagement")

	if combinedRepos == "" {
		return nil
	}
	var re *regexp.Regexp
	if isKts {
		// Kotlin DSL patterns
		re = regexp.MustCompile(`(?m)url(?:\.set)?\s*(?:\(\s*|\s*=\s*)(?:uri\s*\(\s*)?['"]([^'"]+)['"]`)
	} else {
		// Groovy DSL patterns
		re = regexp.MustCompile(`(?m)url\s*(?:[:=]?\s*|[:=]\s*uri\s*\(\s*)['"]([^'"]+)['"]`)
	}
	return re.FindAllSubmatch([]byte(combinedRepos), -1)
}

func resolveGradleProperty(val string, props map[string]string) string {
	if val == "" {
		return val
	}
	// Prevent infinite recursion with a max depth (in case of circular references)
	const maxDepth = 10
	var resolve func(s string, depth int) string
	resolve = func(s string, depth int) string {
		if depth > maxDepth {
			log.Debug("Max recursion depth reached in property resolution for: " + s)
			return s
		}

		// 1. Replace ${key} with value from props
		re := regexp.MustCompile(`\$\{([^}]+)\}`)
		result := re.ReplaceAllStringFunc(s, func(match string) string {
			// strip ${ and }
			key := match[2 : len(match)-1]
			key = strings.TrimSpace(key)

			if key == "" {
				return match
			}

			switch {
			case strings.HasPrefix(key, "project."):
				// Remove "project." prefix
				key = key[8:]
			case strings.HasPrefix(key, "rootProject."):
				// Remove "rootProject." prefix
				key = key[12:]
			}

			// Also handle property accessors like findProperty("key")
			switch {
			case strings.HasPrefix(key, `findProperty("`) && strings.HasSuffix(key, `")`):
				// Remove findProperty(" and ")
				key = key[14 : len(key)-2]
			case strings.HasPrefix(key, `findProperty('`) && strings.HasSuffix(key, `')`):
				// Remove findProperty(' and ')
				key = key[14 : len(key)-2]
			}

			// Handle escaped ${} - if key starts with $, it might be escaped
			if strings.HasPrefix(key, "$") {
				return match
			}

			if v, ok := props[key]; ok && v != "" {
				if v == match {
					log.Debug("Circular property reference detected for: " + key)
					return match
				}
				return resolve(v, depth+1)
			}
			return match
		})

		// 2. Replace $key with value from props (simple variable syntax)
		// Matches $var where var can contain dots (e.g. $project.version or $host.com)
		reSimple := regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)`)
		result = reSimple.ReplaceAllStringFunc(result, func(match string) string {
			// Remove $
			fullKey := match[1:]

			// 1. Try exact match
			if v, ok := props[fullKey]; ok && v != "" {
				if v == match {
					return match
				}
				return resolve(v, depth+1)
			}

			// This handles cases like "$host.com" where 'host' is the property
			parts := strings.Split(fullKey, ".")
			for i := len(parts) - 1; i >= 1; i-- {
				prefix := strings.Join(parts[:i], ".")
				// Check if prefix is a known property
				if v, ok := props[prefix]; ok && v != "" {
					if v == "$"+prefix {
						continue
					}
					suffix := "." + strings.Join(parts[i:], ".")
					return resolve(v, depth+1) + suffix
				}
			}
			return match
		})
		return result
	}
	return resolve(val, 0)
}

func findRepositoryKeyFromMatches(repoUrls []string, sourceName string, isSnapshot bool) (string, error) {
	var snapshotCandidates, releaseCandidates, generalCandidates []string

	for _, repoValue := range repoUrls {
		repoValue = strings.TrimSpace(repoValue)
		if repoValue == "" {
			continue
		}
		repoValueLower := strings.ToLower(repoValue)

		var repoKey string
		var err error
		if strings.Contains(repoValue, "://") || strings.HasPrefix(repoValue, "/") {
			repoKey, err = extractRepoKeyFromArtifactoryUrl(repoValue)
			if err != nil {
				log.Debug("Failed to extract repo key from URL: " + repoValue + " - " + err.Error())
				continue
			}
		}

		if repoKey == "" {
			log.Debug("Empty repository key extracted from: " + repoValue)
			continue
		}

		switch {
		case strings.Contains(repoValueLower, "snapshot"):
			snapshotCandidates = append(snapshotCandidates, repoKey)
		case strings.Contains(repoValueLower, "release"):
			releaseCandidates = append(releaseCandidates, repoKey)
		default:
			generalCandidates = append(generalCandidates, repoKey)
		}
	}

	var selected []string
	if isSnapshot {
		if len(snapshotCandidates) > 0 {
			selected = snapshotCandidates
		} else {
			selected = generalCandidates
		}
	} else {
		if len(releaseCandidates) > 0 {
			selected = releaseCandidates
		} else {
			selected = generalCandidates
		}
	}

	if len(selected) == 0 {
		return "", fmt.Errorf("no matching repository found in %s (isSnapshot: %v)", sourceName, isSnapshot)
	}

	best := selected[0]
	log.Debug("Selected repository from " + sourceName + ": " + best)
	return best, nil
}

func collectAppliedScripts(content []byte, isKts bool, props map[string]string, currentScriptPath string) []string {
	var paths []string
	contentStr := string(content)

	var matches [][]string
	if isKts {
		re := regexp.MustCompile(`(?m)apply\s*\(\s*from\s*=\s*['"]([^'"]+)['"]`)
		matches = re.FindAllStringSubmatch(contentStr, -1)
	} else {
		// Groovy: apply from: "..."
		re := regexp.MustCompile(`(?m)apply\s+from\s*:\s*['"]([^'"]+)['"]`)
		matches = re.FindAllStringSubmatch(contentStr, -1)
	}

	scriptDir := ""
	if currentScriptPath != "" {
		scriptDir = filepath.Dir(currentScriptPath)
	}

	for _, match := range matches {
		if len(match) > 1 {
			path := match[1]
			path = resolveGradleProperty(path, props)

			if strings.Contains(path, "://") {
				log.Debug("Skipping remote script: " + path)
				continue
			}

			if !filepath.IsAbs(path) && scriptDir != "" {
				path = filepath.Join(scriptDir, path)
			}
			paths = append(paths, path)
		}
	}

	return paths
}

func checkInitScripts(gradleUserHome string, isSnapshot bool, props map[string]string) (string, error) {
	// 1. Check init.gradle.kts
	initGradleKtsPath := filepath.Join(gradleUserHome, "init.gradle.kts")
	if repo, err := checkGradleScript(initGradleKtsPath, isSnapshot, props); err == nil {
		return repo, nil
	}

	// 2. Check init.gradle
	initGradlePath := filepath.Join(gradleUserHome, "init.gradle")
	if repo, err := checkGradleScript(initGradlePath, isSnapshot, props); err == nil {
		return repo, nil
	}

	// 3. Check init.d directory
	initDDir := filepath.Join(gradleUserHome, "init.d")
	entries, err := os.ReadDir(initDDir)
	if err == nil {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		// Iterate backwards to find the highest precedence script that has a repo
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".gradle") || strings.HasSuffix(entry.Name(), ".gradle.kts")) {
				if repo, err := checkGradleScript(filepath.Join(initDDir, entry.Name()), isSnapshot, props); err == nil {
					return repo, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no repository found in init scripts")
}

func checkGradleScript(path string, isSnapshot bool, props map[string]string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	isKts := strings.HasSuffix(path, ".kts")
	return findRepoInGradleScript(content, isKts, props, isSnapshot, path)
}
