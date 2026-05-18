package nix

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	nixpkg "github.com/jfrog/build-info-go/flexpack/nix"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// NixCommand wraps native Nix CLI tools with build-info support.
// Dispatches to the correct native tool based on nativeTool field:
//
//	nix-channel → passthrough, no build-info
//	nix-env    → run + collect deps from runtime closure
//	nix-build  → run + collect deps from output store path
//	copy       → run "nix copy" + set build properties + collect artifacts
type NixCommand struct {
	nativeTool         string // "nix-channel", "nix-env", "nix-build", "copy"
	args               []string
	serverDetails      *config.ServerDetails
	buildConfiguration *buildUtils.BuildConfiguration
	workingDir         string
	repo               string
	netrcPath          string
}

func NewNixCommand() *NixCommand {
	return &NixCommand{}
}

func (c *NixCommand) SetNativeTool(tool string) *NixCommand {
	c.nativeTool = tool
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

func (c *NixCommand) Run() error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	c.workingDir = workingDir

	// Set up auth (netrc) for Artifactory access
	if c.serverDetails != nil {
		c.createNetrcFile()
		defer func() {
			if c.netrcPath != "" {
				_ = os.Remove(c.netrcPath)
			}
		}()
	}

	switch c.nativeTool {
	case "nix-channel":
		return c.runNixChannel()
	case "nix-env":
		return c.runNixEnv()
	case "nix-build":
		return c.runNixBuild()
	case "build":
		return c.runNixFlakeBuild()
	case "copy":
		return c.runNixCopy()
	default:
		return c.runPassthrough()
	}
}

// runNixChannel executes "nix-channel" with all args. No build-info.
func (c *NixCommand) runNixChannel() error {
	log.Info("Running nix-channel")
	cmd := exec.Command("nix-channel", c.args...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix-channel failed: %w", err)
	}
	return nil
}

// runNixEnv executes "nix-env" with args, then collects build-info from runtime closure.
func (c *NixCommand) runNixEnv() error {
	log.Info("Running nix-env")
	cmd := exec.Command("nix-env", c.args...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix-env failed: %w", err)
	}

	// Collect build-info: resolve store path from the installed package
	if c.buildConfiguration != nil {
		return c.collectDepsFromEnvArgs()
	}
	return nil
}

// runNixBuild executes "nix-build" with args, then collects build-info from output store path.
func (c *NixCommand) runNixBuild() error {
	log.Info("Running nix-build")
	cmd := exec.Command("nix-build", c.args...)
	cmd.Env = c.buildEnv()
	cmd.Stderr = os.Stderr // Show build progress to user
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nix-build failed: %s: %w", string(output), err)
	}

	// nix-build prints only the output store path(s) to stdout, one per line
	storePaths := strings.Fields(strings.TrimSpace(string(output)))
	if len(storePaths) > 0 {
		log.Info(fmt.Sprintf("Built: %s", strings.Join(storePaths, ", ")))
	}

	// Collect build-info from the output store paths
	if c.buildConfiguration != nil && len(storePaths) > 0 {
		return c.collectBuildInfoFromStorePaths(storePaths)
	}
	return nil
}

// runNixFlakeBuild executes "nix build" (flake-style) with args, then collects build-info
// from the ./result symlink. Unlike nix-build which prints store paths to stdout,
// "nix build" creates a ./result symlink pointing to the output store path.
func (c *NixCommand) runNixFlakeBuild() error {
	log.Info("Running nix build")
	cmd := exec.Command("nix", append([]string{"build"}, c.args...)...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix build failed: %w", err)
	}

	// "nix build" creates ./result symlink → resolve it to get the store path
	var storePaths []string
	resultLink := filepath.Join(c.workingDir, "result")
	if target, err := os.Readlink(resultLink); err == nil {
		storePaths = append(storePaths, target)
		log.Info(fmt.Sprintf("Built: %s", target))
	}

	// Collect build-info from the output store paths
	if c.buildConfiguration != nil && len(storePaths) > 0 {
		return c.collectBuildInfoFromStorePaths(storePaths)
	}
	return nil
}

// runNixCopy executes "nix copy" with args, then sets build properties on uploaded artifacts.
func (c *NixCommand) runNixCopy() error {
	// Parse repo from --to URL for property tagging
	if c.repo == "" {
		c.repo = c.parseRepoFromToArg()
	}

	// If --to points to a virtual repo, resolve to its defaultDeploymentRepo.
	// This ensures artifacts upload to the LOCAL repo (not skip because remote-cache has them).
	// Also add --refresh to force re-check (Nix's internal cache may think it already uploaded).
	if c.repo != "" && c.serverDetails != nil {
		deployRepo := c.resolveDefaultDeploymentRepo(c.repo)
		if deployRepo != "" && deployRepo != c.repo {
			log.Info(fmt.Sprintf("Resolved default deployment repo: %s → %s", c.repo, deployRepo))
			c.replaceRepoInToArg(c.repo, deployRepo)
			c.repo = deployRepo
			// Add --refresh so nix re-checks the LOCAL repo (which is empty)
			// instead of using cached knowledge from previous virtual repo checks
			c.args = append([]string{"--refresh"}, c.args...)
		}
	}

	log.Info("Running nix copy")
	cmd := exec.Command("nix", append([]string{"copy"}, c.args...)...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix copy failed: %w", err)
	}

	// Set build properties on uploaded artifacts
	if c.buildConfiguration != nil && c.repo != "" {
		return c.tagUploadedArtifacts()
	}
	return nil
}

// runPassthrough executes "nix <nativeTool>" for any unrecognized command.
func (c *NixCommand) runPassthrough() error {
	log.Info(fmt.Sprintf("Running nix %s", c.nativeTool))
	cmd := exec.Command("nix", append([]string{c.nativeTool}, c.args...)...)
	cmd.Env = c.buildEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nix %s failed: %w", c.nativeTool, err)
	}
	return nil
}

// collectDepsFromEnvArgs resolves the store path from nix-env args (e.g., "nixpkgs.hello").
func (c *NixCommand) collectDepsFromEnvArgs() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	// Find the package attribute in args (last non-flag arg, e.g., "nixpkgs.hello")
	var pkgAttr string
	for i := len(c.args) - 1; i >= 0; i-- {
		if !strings.HasPrefix(c.args[i], "-") && strings.Contains(c.args[i], ".") {
			pkgAttr = c.args[i]
			break
		}
	}
	if pkgAttr == "" {
		return nil
	}

	// Split "nixpkgs.hello" → channel="nixpkgs", attr="hello"
	parts := strings.SplitN(pkgAttr, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	// Resolve store path: nix-build '<channel>' -A attr --no-out-link
	cmd := exec.Command("nix-build", fmt.Sprintf("<%s>", parts[0]), "-A", parts[1], "--no-out-link")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn(fmt.Sprintf("Could not resolve store path for %s: %s", pkgAttr, string(output)))
		return err
	}

	storePaths := strings.Fields(strings.TrimSpace(string(output)))
	return c.collectBuildInfoFromStorePaths(storePaths)
}

// collectBuildInfoFromStorePaths collects build-info using NixChannelCollector.
func (c *NixCommand) collectBuildInfoFromStorePaths(storePaths []string) error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Collecting build info for Nix project: %s/%s", buildName, buildNumber))

	collector, err := nixpkg.NewNixChannelCollector(nixpkg.NixChannelConfig{
		WorkingDirectory: c.workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to create Nix collector: %w", err)
	}

	if err := collector.CollectStorePathDependencies(storePaths...); err != nil {
		log.Warn("Failed to collect runtime dependencies: " + err.Error())
	}

	buildInfo, err := collector.CollectBuildInfo(buildName, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to collect build info: %w", err)
	}

	// Resolve dep checksums from Artifactory via AQL.
	// The dependency file is the .nar.xz (compiled binary archive).
	// Search in the virtual repo which sees both local + remote-cache.
	if c.serverDetails != nil && len(buildInfo.Modules) > 0 {
		deps, _ := collector.GetProjectDependencies()
		depPathMap := make(map[string]string)
		for _, d := range deps {
			depPathMap[d.ID] = d.Path
		}

		// Determine repo to search — use --repo if set, else parse from nix.conf substituter
		searchRepo := c.repo
		if searchRepo == "" {
			searchRepo = c.parseRepoFromSubstituter()
		}

		if searchRepo != "" {
			resolved := 0
			for i, dep := range buildInfo.Modules[0].Dependencies {
				storePath, ok := depPathMap[dep.Id]
				if !ok {
					continue
				}
				storeHash := nixpkg.ExtractStoreHash(storePath)
				dirPath := "binary-cache/" + storeHash

				// Find the .nar.xz file — the actual dependency binary
				narXzName, narXzPath := c.findNarFile(searchRepo, dirPath)
				if narXzName != "" {
					artChecksum := c.getArtifactChecksums(searchRepo, narXzPath)
					if artChecksum.Sha1 != "" || artChecksum.Sha256 != "" {
						buildInfo.Modules[0].Dependencies[i].Checksum = artChecksum
						resolved++
					}
				}
			}
			if resolved > 0 {
				log.Info(fmt.Sprintf("Resolved %d dep checksum(s) from Artifactory", resolved))
			}
		}
	}

	// Apply --module override
	if c.buildConfiguration != nil {
		moduleOverride := c.buildConfiguration.GetModule()
		if moduleOverride != "" && len(buildInfo.Modules) > 0 {
			buildInfo.Modules[0].Id = moduleOverride
		}
	}

	projectKey := ""
	if c.buildConfiguration != nil {
		projectKey = c.buildConfiguration.GetProject()
	}
	if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
		return fmt.Errorf("failed to save build info: %w", err)
	}

	log.Info(fmt.Sprintf("Nix build info collected. Use 'jf rt bp %s %s' to publish it.", buildName, buildNumber))
	return nil
}

// tagUploadedArtifacts sets build properties on artifacts uploaded by nix copy.
func (c *NixCommand) tagUploadedArtifacts() error {
	buildName, buildNumber, _ := c.getBuildNameAndNumber()
	if buildName == "" || buildNumber == "" {
		return nil
	}

	log.Info(fmt.Sprintf("Tagging uploaded artifacts with build info: %s/%s", buildName, buildNumber))

	// Find the store path from args (./result or /nix/store/...)
	storePath := c.findStorePathFromArgs()
	if storePath == "" {
		return fmt.Errorf("no store path found in args")
	}

	// Get all store paths in the closure
	cmd := exec.Command("nix", "path-info", "--recursive", storePath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nix path-info failed: %w", err)
	}

	closurePaths := strings.Fields(strings.TrimSpace(string(output)))
	log.Info(fmt.Sprintf("Found %d store path(s) in closure", len(closurePaths)))

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	var artifacts []entities.Artifact

	for _, sp := range closurePaths {
		storeHash := nixpkg.ExtractStoreHash(sp)
		dirPath := "binary-cache/" + storeHash

		// Set build properties on ALL files in this directory
		c.setBuildPropertiesInDir(c.repo, dirPath, "*", buildName, buildNumber, timestamp)

		// Find the NAR file and get checksums
		narFileName, narFilePath := c.findNarFile(c.repo, dirPath)
		if narFileName != "" {
			narChecksum := c.getArtifactChecksums(c.repo, narFilePath)
			artifacts = append(artifacts, entities.Artifact{
				Name:                   narFileName,
				Type:                   "xz",
				Path:                   narFilePath,
				OriginalDeploymentRepo: c.repo,
				Checksum:               narChecksum,
			})
		}

		// narinfo artifact
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
	}

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

		projectKey := ""
		if c.buildConfiguration != nil {
			projectKey = c.buildConfiguration.GetProject()
		}
		if err := saveBuildInfoLocally(buildInfo, projectKey); err != nil {
			return fmt.Errorf("failed to save build info: %w", err)
		}
		log.Info(fmt.Sprintf("Tagged %d artifact(s) with build properties", len(artifacts)))
	}

	return nil
}

// findStorePathFromArgs finds a store path or ./result from the command args.
func (c *NixCommand) findStorePathFromArgs() string {
	for _, arg := range c.args {
		if strings.HasPrefix(arg, "/nix/store/") {
			return arg
		}
		if arg == "./result" || arg == "result" {
			resolved, err := os.Readlink(arg)
			if err == nil {
				return resolved
			}
		}
		// Check if it's a path that resolves to a store path
		if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "http") {
			resolved, err := filepath.Abs(arg)
			if err == nil {
				link, err := os.Readlink(resolved)
				if err == nil && strings.HasPrefix(link, "/nix/store/") {
					return link
				}
			}
		}
	}
	return ""
}

// resolveDefaultDeploymentRepo queries Artifactory REST API to find the defaultDeploymentRepo
// for a virtual repo. Returns empty if not virtual or no default deployment repo.
func (c *NixCommand) resolveDefaultDeploymentRepo(repoName string) string {
	if c.serverDetails == nil || c.serverDetails.ArtifactoryUrl == "" {
		return ""
	}

	// Query the repo config: GET /api/repositories/<name>
	url := strings.TrimSuffix(c.serverDetails.ArtifactoryUrl, "/") + "/api/repositories/" + repoName
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	if c.serverDetails.User != "" {
		password := c.serverDetails.Password
		if password == "" {
			password = c.serverDetails.AccessToken
		}
		req.SetBasicAuth(c.serverDetails.User, password)
	}

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	var repoConfig struct {
		Rclass                string `json:"rclass"`
		DefaultDeploymentRepo string `json:"defaultDeploymentRepo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoConfig); err != nil {
		return ""
	}

	if repoConfig.Rclass == "virtual" && repoConfig.DefaultDeploymentRepo != "" {
		return repoConfig.DefaultDeploymentRepo
	}
	return ""
}

// replaceRepoInToArg replaces the repo name in the --to URL argument.
func (c *NixCommand) replaceRepoInToArg(oldRepo, newRepo string) {
	for i, arg := range c.args {
		if strings.Contains(arg, "/api/nix/"+oldRepo) {
			c.args[i] = strings.Replace(arg, "/api/nix/"+oldRepo, "/api/nix/"+newRepo, 1)
		}
		// Also check next arg if this is "--to"
		if arg == "--to" && i+1 < len(c.args) && strings.Contains(c.args[i+1], "/api/nix/"+oldRepo) {
			c.args[i+1] = strings.Replace(c.args[i+1], "/api/nix/"+oldRepo, "/api/nix/"+newRepo, 1)
		}
	}
}

// parseRepoFromSubstituter reads nix.conf and extracts the repo name from the substituter URL.
func (c *NixCommand) parseRepoFromSubstituter() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(homeDir, ".config", "nix", "nix.conf"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "substituters") || strings.HasPrefix(line, "extra-substituters") {
			if idx := strings.Index(line, "/api/nix/"); idx != -1 {
				repo := line[idx+len("/api/nix/"):]
				// Remove query params (?priority=1) and trailing whitespace
				if qIdx := strings.IndexAny(repo, "? \t#"); qIdx != -1 {
					repo = repo[:qIdx]
				}
				repo = strings.TrimSuffix(repo, "/")
				if repo != "" {
					return repo
				}
			}
		}
	}
	return ""
}

// parseRepoFromToArg extracts the repo name from --to URL.
func (c *NixCommand) parseRepoFromToArg() string {
	for i, arg := range c.args {
		var url string
		if arg == "--to" && i+1 < len(c.args) {
			url = c.args[i+1]
		} else if strings.HasPrefix(arg, "--to=") {
			url = strings.TrimPrefix(arg, "--to=")
		}
		if url != "" {
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

// createNetrcFile creates a temporary netrc file for Nix authentication.
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
	if _, err = tmpFile.WriteString(netrcContent); err != nil {
		return
	}
	if err = tmpFile.Close(); err != nil {
		return
	}
	c.netrcPath = tmpFile.Name()
}

// buildEnv returns the current environment with NIX_CONFIG for netrc auth.
func (c *NixCommand) buildEnv() []string {
	env := os.Environ()
	if c.netrcPath != "" {
		env = append(env, "NIX_CONFIG=netrc-file = "+c.netrcPath)
	}
	return env
}

// setBuildPropertiesInDir sets build properties on all files in a directory.
func (c *NixCommand) setBuildPropertiesInDir(repo, dirPath, namePattern, buildName, buildNumber, timestamp string) {
	if c.serverDetails == nil {
		return
	}
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return
	}

	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", buildName, buildNumber, timestamp)
	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": "%s","name": {"$match": "%s"}}]}]}`, repo, dirPath, namePattern)
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}
	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return
	}
	_, _ = servicesManager.SetProps(services.PropsParams{Reader: reader, Props: props})
}

// findNarFile searches for the .nar.xz file in a binary-cache directory.
func (c *NixCommand) findNarFile(repo, dirPath string) (string, string) {
	if c.serverDetails == nil {
		return "", ""
	}
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
	defer func() { _ = reader.Close() }()

	item := new(specutils.ResultItem)
	if reader.NextRecord(item) == nil {
		pathInRepo := item.Path + "/" + item.Name
		if item.Path == "." {
			pathInRepo = item.Name
		}
		return item.Name, pathInRepo
	}
	return "", ""
}

// getArtifactChecksums fetches sha1/sha256/md5 for a file in Artifactory.
func (c *NixCommand) getArtifactChecksums(repo, pathInRepo string) entities.Checksum {
	if c.serverDetails == nil {
		return entities.Checksum{}
	}
	servicesManager, err := utils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return entities.Checksum{}
	}

	searchQuery := fmt.Sprintf(`{"repo": "%s", "$or": [{"$and":[{"path": "%s","name": "%s"}]}]}`,
		repo, filepath.Dir(pathInRepo), filepath.Base(pathInRepo))
	searchParams := services.SearchParams{
		CommonParams: &specutils.CommonParams{
			Aql: specutils.Aql{ItemsFind: searchQuery},
		},
	}
	reader, err := servicesManager.SearchFiles(searchParams)
	if err != nil {
		return entities.Checksum{}
	}
	defer func() { _ = reader.Close() }()

	item := new(specutils.ResultItem)
	if reader.NextRecord(item) == nil {
		return entities.Checksum{
			Sha1:   item.Actual_Sha1,
			Sha256: item.Sha256,
			Md5:    item.Actual_Md5,
		}
	}
	return entities.Checksum{}
}

func (c *NixCommand) getBuildNameAndNumber() (string, string, error) {
	if c.buildConfiguration == nil {
		return "", "", fmt.Errorf("no build configuration")
	}
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

// GetCmd returns the exec.Cmd for gofrogcmd.RunCmd interface.
func (c *NixCommand) GetCmd() *exec.Cmd {
	return exec.Command("nix", append([]string{c.nativeTool}, c.args...)...)
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
func (c *NixCommand) CommandName() string           { return "rt_nix" }

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

// Ensure unused imports don't cause compile errors
var _ = json.Marshal
