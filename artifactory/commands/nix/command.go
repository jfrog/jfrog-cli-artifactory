package nix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	nixflex "github.com/jfrog/build-info-go/flexpack/nix"
	gofrogcmd "github.com/jfrog/gofrog/io"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NixCommand represents a Nix CLI command with build info support.
// Follows the FlexPack native-first pattern:
//   - No config command (like Conan, unlike npm/maven)
//   - User configures Nix substituters to point to Artifactory (one-time setup)
//   - JFrog CLI runs native nix commands and collects build info afterward
//
// Flow:
//
//	jf nix flake lock  → native nix flake lock  → collect deps from flake.lock
//	jf nix build       → native nix build        → collect deps (flake.lock + runtime closure)
//	jf nix copy        → native nix copy --to    → set build properties on uploaded artifacts
//	jf rt bp X Y       → publish build-info
type NixCommand struct {
	commandName        string
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
	repo               string // Target repo for substituter and property tagging (--repo flag)
	netrcPath          string // Temp netrc file, cleaned up after Run
	nixConfPath        string // Path to user nix.conf (for restore)
	originalNixConf    string // Original nix.conf content (for restore)
}

func NewNixCommand() *NixCommand {
	return &NixCommand{}
}

func (c *NixCommand) SetCommandName(name string) *NixCommand {
	c.commandName = name
	return c
}

func (c *NixCommand) SetArgs(args []string) *NixCommand {
	c.args = args
	return c
}

func (c *NixCommand) SetServerDetails(details *config.ServerDetails) *NixCommand {
	c.serverDetails = details
	return c
}

func (c *NixCommand) SetBuildConfiguration(config *buildUtils.BuildConfiguration) *NixCommand {
	c.buildConfiguration = config
	return c
}

func (c *NixCommand) SetRepo(repo string) *NixCommand {
	c.repo = repo
	return c
}

// Commands that resolve dependencies → collect build-info from flake.lock
var commandsCollectingDeps = []string{
	"build",
	"develop",
	"flake",
	"run",
	"shell",
	"profile",
}

func shouldCollectDeps(cmd string) bool {
	for _, c := range commandsCollectingDeps {
		if c == cmd {
			return true
		}
	}
	return false
}

// Commands that produce ./result → also collect runtime closure deps
var commandsWithRuntimeClosure = []string{
	"build",
}

func hasRuntimeClosure(cmd string) bool {
	for _, c := range commandsWithRuntimeClosure {
		if c == cmd {
			return true
		}
	}
	return false
}

// Commands that upload to a remote → set build properties on uploaded artifacts
// "copy" is the native nix command for uploading NARs to a binary cache
var commandsUploading = []string{
	"copy",
}

func isUploadCommand(cmd string) bool {
	for _, c := range commandsUploading {
		if c == cmd {
			return true
		}
	}
	return false
}

func (c *NixCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// If --repo is not set, try to parse it from --to URL in the args.
	// e.g. --to "https://server/artifactory/api/nix/nix-local" → repo = "nix-local"
	if c.repo == "" {
		c.repo = c.parseRepoFromToArg()
	}

	// Set up auth for Artifactory access (substituter for resolve, netrc for copy)
	if c.serverDetails != nil && c.repo != "" {
		c.setupSubstituterAndAuth()
		defer func() {
			c.restoreNixConf()
			if c.netrcPath != "" {
				os.Remove(c.netrcPath)
			}
		}()
	} else if c.serverDetails != nil && c.repo == "" && isUploadCommand(c.commandName) {
		// Even without --repo, create netrc for auth if --to points to Artifactory
		c.createNetrcFile()
		defer func() {
			if c.netrcPath != "" {
				os.Remove(c.netrcPath)
			}
		}()
	}

	log.Info(fmt.Sprintf("Running Nix %s", c.commandName))

	// Run the native nix command
	if err := gofrogcmd.RunCmd(c); err != nil {
		return fmt.Errorf("nix %s failed: %w", c.commandName, err)
	}

	// After native command completes, collect build info or set properties
	if c.buildConfiguration != nil {
		if shouldCollectDeps(c.commandName) {
			return c.collectAndSaveBuildInfo()
		}
		if isUploadCommand(c.commandName) {
			return c.tagUploadedArtifacts()
		}
	}

	return nil
}

// collectAndSaveBuildInfo collects dependencies from flake.lock and optionally
// from the runtime closure (after nix build), then saves build-info locally.
func (c *NixCommand) collectAndSaveBuildInfo() error {
	buildName, buildNumber, err := c.getBuildNameAndNumber()
	if err != nil || buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Collecting build info for Nix project: %s/%s", buildName, buildNumber))

	collector, err := nixflex.NewNixFlexPack(nixflex.NixConfig{
		WorkingDirectory: c.workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create Nix FlexPack: %w", err)
	}

	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect Nix build info: %w", err)
	}

	// For build commands, also collect runtime closure dependencies
	if hasRuntimeClosure(c.commandName) {
		resultPath := filepath.Join(c.workingDir, "result")
		if _, err := os.Lstat(resultPath); err == nil {
			runtimeDeps, err := c.collectRuntimeClosure(resultPath)
			if err != nil {
				log.Warn("Failed to collect runtime closure: " + err.Error())
			} else if len(runtimeDeps) > 0 && len(buildInfo.Modules) > 0 {
				buildInfo.Modules[0].Dependencies = append(buildInfo.Modules[0].Dependencies, runtimeDeps...)
				log.Info(fmt.Sprintf("Added %d runtime dependency(ies) from build closure", len(runtimeDeps)))
			}
		}
	}

	// Apply --module override if specified
	moduleOverride := c.buildConfiguration.GetModule()
	if moduleOverride != "" && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Id = moduleOverride
	}

	projectKey := c.buildConfiguration.GetProject()
	if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}

	log.Info(fmt.Sprintf("Nix build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// nixPathInfo represents the JSON output of "nix path-info --json -r"
type nixPathInfo struct {
	Path       string   `json:"path"`
	NarHash    string   `json:"narHash"`
	NarSize    int64    `json:"narSize"`
	References []string `json:"references"`
	Deriver    string   `json:"deriver,omitempty"`
}

// collectRuntimeClosure runs "nix path-info --json -r ./result" and returns
// the runtime closure as build-info dependencies with scope "runtime".
func (c *NixCommand) collectRuntimeClosure(resultPath string) ([]entities.Dependency, error) {
	storePath, err := os.Readlink(resultPath)
	if err != nil {
		return nil, fmt.Errorf("read result symlink: %w", err)
	}

	cmd := exec.Command("nix", "path-info", "--json", "--recursive", storePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nix path-info failed: %w", err)
	}

	// nix path-info --json outputs a map: { "/nix/store/...": { narHash, narSize, references, ... }, ... }
	var pathInfoMap map[string]nixPathInfo
	if err := json.Unmarshal(output, &pathInfoMap); err != nil {
		return nil, fmt.Errorf("parse path-info output: %w", err)
	}
	// Fill in the Path field from the map key
	for path, info := range pathInfoMap {
		info.Path = path
		pathInfoMap[path] = info
	}

	// Build requestedBy from references (invert the graph)
	requestedBy := make(map[string][]string)
	for parentPath, info := range pathInfoMap {
		parentID := storePathToDepID(parentPath)
		for _, refPath := range info.References {
			if refPath == parentPath {
				continue // skip self-references
			}
			childID := storePathToDepID(refPath)
			requestedBy[childID] = append(requestedBy[childID], parentID)
		}
	}

	// Convert to build-info dependencies, skip the root package itself.
	rootID := storePathToDepID(storePath)
	var deps []entities.Dependency
	for path, info := range pathInfoMap {
		depID := storePathToDepID(path)
		if depID == rootID {
			continue // skip root — it's the project, not a dep
		}

		dep := entities.Dependency{
			Id:     depID,
			Scopes: []string{"runtime"},
			Checksum: entities.Checksum{
				Sha256: info.NarHash,
			},
		}

		if parents, ok := requestedBy[depID]; ok && len(parents) > 0 {
			dep.RequestedBy = [][]string{parents}
		}

		deps = append(deps, dep)
	}

	return deps, nil
}

// storePathToDepID converts "/nix/store/<hash>-name-version" to "name:version"
// e.g. "/nix/store/yalw...-hello-2.12.3" → "hello:2.12.3"
func storePathToDepID(storePath string) string {
	base := filepath.Base(storePath)
	// Store path format: <hash>-<name>[-<version>]
	if idx := strings.Index(base, "-"); idx != -1 {
		nameVersion := base[idx+1:]
		// Try to split name-version at the last dash before a digit
		for i := len(nameVersion) - 1; i >= 0; i-- {
			if nameVersion[i] == '-' && i+1 < len(nameVersion) && nameVersion[i+1] >= '0' && nameVersion[i+1] <= '9' {
				return nameVersion[:i] + ":" + nameVersion[i+1:]
			}
		}
		return nameVersion
	}
	return base
}

// tagUploadedArtifacts sets build properties on artifacts that were uploaded
// by the native "nix copy --to" command. Also collects the uploaded artifacts
// into build-info.
func (c *NixCommand) tagUploadedArtifacts() error {
	buildName, buildNumber, err := c.getBuildNameAndNumber()
	if err != nil || buildName == "" || buildNumber == "" {
		return nil
	}

	if c.repo == "" {
		log.Warn("No --repo specified, skipping build property tagging")
		return nil
	}

	log.Info(fmt.Sprintf("Tagging uploaded artifacts with build info: %s/%s", buildName, buildNumber))

	// Find the store paths that were uploaded by looking at ./result closure
	resultPath := filepath.Join(c.workingDir, "result")
	storePath, err := os.Readlink(resultPath)
	if err != nil {
		return fmt.Errorf("no build output found (./result symlink missing): %w", err)
	}

	// Get all store paths in the closure
	cmd := exec.Command("nix", "path-info", "--recursive", storePath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nix path-info failed: %w", err)
	}

	storePaths := strings.Fields(strings.TrimSpace(string(output)))
	log.Info(fmt.Sprintf("Found %d store path(s) in closure", len(storePaths)))

	// For each store path, read the narinfo from Artifactory to get the real NAR filename,
	// then set build properties on all files and record artifacts with correct paths.
	var artifacts []entities.Artifact
	for _, sp := range storePaths {
		storeHash := filepath.Base(sp)
		if idx := strings.Index(storeHash, "-"); idx != -1 {
			storeHash = storeHash[:idx]
		}

		dirPath := "binary-cache/" + storeHash

		// Set build properties on ALL files in this directory (narinfo + nar.xz)
		c.setBuildPropertiesInDir(c.repo, dirPath, "*", buildName, buildNumber)

		// Find the actual NAR filename by listing files in the directory
		narFileName, narFilePath := c.findNarFile(c.repo, dirPath)

		baseName := filepath.Base(sp)
		name := baseName
		if idx := strings.Index(baseName, "-"); idx != -1 {
			name = baseName[idx+1:]
		}

		if narFileName != "" {
			// NAR archive artifact — fetch checksums from Artifactory
			narChecksum := c.getArtifactChecksums(c.repo, narFilePath)
			artifacts = append(artifacts, entities.Artifact{
				Name:                   narFileName,
				Type:                   "xz",
				Path:                   narFilePath,
				OriginalDeploymentRepo: c.repo,
				Checksum:               narChecksum,
			})
		}

		// narinfo metadata artifact — fetch checksums from Artifactory
		narinfoName := storeHash + ".narinfo"
		narinfoPath := dirPath + "/" + narinfoName
		narinfoChecksum := c.getArtifactChecksums(c.repo, narinfoPath)
		artifacts = append(artifacts, entities.Artifact{
			Name:                   narinfoName,
			Type:                   "narinfo",
			Path:                   narinfoPath,
			OriginalDeploymentRepo: c.repo,
			Checksum:               narinfoChecksum,
		})

		log.Debug(fmt.Sprintf("Artifact: %s → %s/%s", name, c.repo, narFilePath))
	}

	// Save artifacts to build-info
	if len(artifacts) > 0 {
		buildInfo := &entities.BuildInfo{
			Name:   buildName,
			Number: buildNumber,
			Modules: []entities.Module{
				{
					Id:        filepath.Base(c.workingDir),
					Type:      entities.Nix,
					Artifacts: artifacts,
				},
			},
		}

		projectKey := c.buildConfiguration.GetProject()
		if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
			return fmt.Errorf("failed to save build info: %w", err)
		}
		log.Info(fmt.Sprintf("Tagged %d artifact(s) with build properties", len(artifacts)))
	}

	return nil
}

// setupSubstituterAndAuth configures Nix to resolve/upload through Artifactory.
func (c *NixCommand) setupSubstituterAndAuth() {
	if c.serverDetails == nil || c.repo == "" {
		return
	}

	substituterURL := strings.TrimSuffix(c.serverDetails.ArtifactoryUrl, "/") + "/api/nix/" + c.repo

	if err := c.addSubstituterToNixConf(substituterURL); err != nil {
		log.Warn("Failed to configure Artifactory substituter in nix.conf: " + err.Error())
	} else {
		log.Info(fmt.Sprintf("Configured Artifactory substituter: %s", substituterURL))
	}

	c.createNetrcFile()

	// For "nix copy --to", inject the Artifactory URL if the user provided --repo
	// but didn't specify the full --to URL
	if isUploadCommand(c.commandName) && c.repo != "" {
		c.injectCopyTarget(substituterURL)
	}
}

// parseRepoFromToArg extracts the repo name from a --to URL in the args.
// e.g. --to "https://server/artifactory/api/nix/nix-local" → "nix-local"
// e.g. --to "https://server/artifactory/api/nix/nix-virtual/" → "nix-virtual"
func (c *NixCommand) parseRepoFromToArg() string {
	for i, arg := range c.args {
		var url string
		if arg == "--to" && i+1 < len(c.args) {
			url = c.args[i+1]
		} else if strings.HasPrefix(arg, "--to=") {
			url = strings.TrimPrefix(arg, "--to=")
		}
		if url != "" {
			// Look for /api/nix/<repo> pattern
			if idx := strings.Index(url, "/api/nix/"); idx != -1 {
				repo := url[idx+len("/api/nix/"):]
				repo = strings.TrimSuffix(repo, "/")
				if repo != "" {
					log.Info(fmt.Sprintf("Parsed repo '%s' from --to URL", repo))
					return repo
				}
			}
		}
	}
	return ""
}

// injectCopyTarget adds the --to argument for nix copy if not already present.
// Authentication is handled via netrc file (set in NIX_CONFIG env var), not URL credentials.
func (c *NixCommand) injectCopyTarget(substituterURL string) {
	for _, arg := range c.args {
		if strings.HasPrefix(arg, "--to") {
			return // user already specified --to
		}
	}
	// Use plain URL — auth comes from netrc file passed via NIX_CONFIG
	// --refresh forces Nix to re-check the remote, ensuring upload happens even if
	// Nix's local cache thinks the paths are already present on the remote.
	c.args = append([]string{"--refresh", "--to", substituterURL}, c.args...)
	log.Info(fmt.Sprintf("Injected --to target: %s", substituterURL))
}

const nixConfMarker = "# jfrog-cli-managed"

func (c *NixCommand) addSubstituterToNixConf(substituterURL string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	nixConfDir := filepath.Join(homeDir, ".config", "nix")
	nixConfPath := filepath.Join(nixConfDir, "nix.conf")

	existingContent, _ := os.ReadFile(nixConfPath)
	c.originalNixConf = string(existingContent)
	c.nixConfPath = nixConfPath

	if strings.Contains(string(existingContent), substituterURL) {
		return nil
	}

	if err := os.MkdirAll(nixConfDir, 0755); err != nil {
		return fmt.Errorf("create nix config dir: %w", err)
	}

	extraLines := fmt.Sprintf("\nextra-substituters = %s %s\n", substituterURL, nixConfMarker)
	newContent := string(existingContent) + extraLines

	return os.WriteFile(nixConfPath, []byte(newContent), 0644)
}

func (c *NixCommand) restoreNixConf() {
	if c.nixConfPath == "" {
		return
	}
	currentContent, err := os.ReadFile(c.nixConfPath)
	if err != nil {
		return
	}

	var cleanedLines []string
	for _, line := range strings.Split(string(currentContent), "\n") {
		if !strings.Contains(line, nixConfMarker) {
			cleanedLines = append(cleanedLines, line)
		}
	}
	cleaned := strings.TrimRight(strings.Join(cleanedLines, "\n"), "\n") + "\n"
	os.WriteFile(c.nixConfPath, []byte(cleaned), 0644)
}

func (c *NixCommand) createNetrcFile() {
	user := c.serverDetails.User
	password := c.serverDetails.Password
	if password == "" {
		password = c.serverDetails.AccessToken
	}
	if user == "" || password == "" {
		return
	}

	host := c.serverDetails.ArtifactoryUrl
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	host = strings.TrimSuffix(host, "/")
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	netrcContent := fmt.Sprintf("machine %s\nlogin %s\npassword %s\n", host, user, password)
	tmpFile, err := os.CreateTemp("", "nix-netrc-")
	if err != nil {
		return
	}
	tmpFile.WriteString(netrcContent)
	tmpFile.Close()
	c.netrcPath = tmpFile.Name()
}

// getArtifactChecksums fetches sha1/sha256/md5 for a file in Artifactory.
func (c *NixCommand) getArtifactChecksums(repo, pathInRepo string) entities.Checksum {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return entities.Checksum{}
	}

	// Use AQL to find the file and get its checksums
	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": "%s","name": "%s"}]}]}`,
		repo,
		filepath.Dir(pathInRepo),
		filepath.Base(pathInRepo))
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return entities.Checksum{}
	}
	defer reader.Close()

	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		return entities.Checksum{
			Sha1:   item.Actual_Sha1,
			Sha256: item.Sha256,
			Md5:    item.Actual_Md5,
		}
	}
	return entities.Checksum{}
}

// findNarFile searches for the .nar.xz file in a binary-cache directory.
func (c *NixCommand) findNarFile(repo, dirPath string) (string, string) {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return "", ""
	}

	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": "%s","name": {"$match": "*.nar.xz"}}]}]}`, repo, dirPath)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return "", ""
	}
	defer reader.Close()

	// Read first result
	for item := new(specutils.ResultItem); reader.NextRecord(item) == nil; item = new(specutils.ResultItem) {
		// Build path relative to repo root: path/name
		pathInRepo := item.Path + "/" + item.Name
		if item.Path == "." {
			pathInRepo = item.Name
		}
		return item.Name, pathInRepo
	}
	return "", ""
}

func (c *NixCommand) setBuildPropertiesInDir(repo, dirPath, namePattern, buildName, buildNumber string) {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("Failed to create service manager: " + err.Error())
		return
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)

	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": "%s","name": {"$match": "%s"}}]}]}`, repo, dirPath, namePattern)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		log.Debug("Failed to search for artifacts in " + dirPath + ": " + err.Error())
		return
	}

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  props,
	}
	_, err = servicesManager.SetProps(propsParams)
	if err != nil {
		log.Debug("Failed to set build properties in " + dirPath + ": " + err.Error())
	}
}

func (c *NixCommand) setBuildProperties(repo, artifactPattern, buildName, buildNumber string) {
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("Failed to create service manager: " + err.Error())
		return
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)

	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": {"$match": "*"},"name": {"$match": "%s"}}]}]}`, repo, artifactPattern)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}

	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		log.Debug("Failed to search for artifact: " + err.Error())
		return
	}

	propsParams := services.PropsParams{
		Reader: reader,
		Props:  props,
	}
	servicesManager.SetProps(propsParams)
}

func (c *NixCommand) getBuildNameAndNumber() (string, string, error) {
	buildName, err := c.buildConfiguration.GetBuildName()
	if err != nil || buildName == "" {
		return "", "", fmt.Errorf("build name not configured")
	}
	buildNumber, err := c.buildConfiguration.GetBuildNumber()
	if err != nil || buildNumber == "" {
		return "", "", fmt.Errorf("build number not configured")
	}
	return buildName, buildNumber, nil
}

func (c *NixCommand) GetCmd() *exec.Cmd {
	args := append([]string{c.commandName}, c.args...)
	return exec.Command("nix", args...)
}

func (c *NixCommand) GetEnv() map[string]string {
	env := map[string]string{}
	if c.netrcPath != "" {
		env["NIX_CONFIG"] = "netrc-file = " + c.netrcPath
	}
	return env
}

func (c *NixCommand) GetStdWriter() io.WriteCloser { return nil }
func (c *NixCommand) GetErrWriter() io.WriteCloser { return nil }

func (c *NixCommand) CommandName() string {
	return "rt_nix"
}

func (c *NixCommand) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func saveBuildInfoLocally(buildInfo *entities.BuildInfo, projectKey string) error {
	service := buildUtils.CreateBuildInfoService()
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	return nil
}
