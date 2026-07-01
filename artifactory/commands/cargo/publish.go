package cargo

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jfrog/build-info-go/entities"
	cargoflex "github.com/jfrog/build-info-go/flexpack/cargo"
	artutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// metadataBoolFlags are `cargo metadata`-valid boolean flags that affect resolution.
var metadataBoolFlags = map[string]bool{
	"--all-features":        true,
	"--no-default-features": true,
	"--locked":              true,
	"--frozen":              true,
	"--offline":             true,
}

// metadataValueFlags are `cargo metadata`-valid flags that take a value.
var metadataValueFlags = map[string]bool{
	"--features":      true,
	"--manifest-path": true,
}

// metadataFlagsFromArgs returns the subset of args that are valid for `cargo metadata`
// and affect dependency resolution, preserving order. Handles both "--flag value" and
// "--flag=value" forms for value flags.
func metadataFlagsFromArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case metadataBoolFlags[a]:
			out = append(out, a)
		case metadataValueFlags[a]:
			out = append(out, a)
			if i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--features=") || strings.HasPrefix(a, "--manifest-path="):
			out = append(out, a)
		}
	}
	return out
}

// setModuleCommandProperties records the cargo sub-command and its args on the first
// module of the build-info, so the executed command is captured in the build-info.
func setModuleCommandProperties(bi *entities.BuildInfo, commandName string, args []string) {
	if bi == nil || len(bi.Modules) == 0 {
		return
	}
	props := map[string]string{"cargo.command": commandName}
	if len(args) > 0 {
		props["cargo.args"] = strings.Join(args, " ")
	}
	bi.Modules[0].Properties = props
}

// buildNameNumber returns the configured build name and number (empty strings if unset).
func (c *CargoCommand) buildNameNumber() (string, string) {
	if c.buildConfiguration == nil {
		return "", ""
	}
	name, _ := c.buildConfiguration.GetBuildName()
	number, _ := c.buildConfiguration.GetBuildNumber()
	return name, number
}

// newCollector creates a Cargo FlexPack build-info collector for the working dir.
func (c *CargoCommand) newCollector() (*cargoflex.CargoFlexPack, error) {
	return cargoflex.NewCargoFlexPack(cargoflex.CargoConfig{
		WorkingDirectory:       c.workingDir,
		IncludeDevDependencies: false,
		MetadataArgs:           metadataFlagsFromArgs(c.args),
	})
}

// collectDeps collects dependency build-info and saves it locally.
func (c *CargoCommand) collectDeps() error {
	name, number := c.buildNameNumber()
	if name == "" {
		log.Debug("cargo: no --build-name; skipping build-info collection")
		return nil
	}
	collector, err := c.newCollector()
	if err != nil {
		return err
	}
	bi, err := collector.CollectBuildInfo(name, number)
	if err != nil {
		return err
	}
	setModuleCommandProperties(bi, c.commandName, c.args)
	c.enrichChecksums(bi)
	return c.saveBuildInfo(bi)
}

// collectArtifacts collects deps (module type cargo), appends the scanned crate
// artifacts to module 0, optionally sets build properties, and saves build-info.
func (c *CargoCommand) collectArtifacts(setProps bool) error {
	name, number := c.buildNameNumber()
	if name == "" {
		log.Debug("cargo: no --build-name; skipping artifact collection")
		return nil
	}
	collector, err := c.newCollector()
	if err != nil {
		return err
	}
	bi, err := collector.CollectBuildInfo(name, number) // deps first (module type = cargo)
	if err != nil {
		return err
	}
	setModuleCommandProperties(bi, c.commandName, c.args)
	c.enrichChecksums(bi)

	repo, err := c.targetRepo()
	if err != nil {
		log.Debug("cargo: could not determine target repo: " + err.Error())
	}
	arts, err := scanCrateArtifacts(c.workingDir, repo)
	if err != nil {
		return err
	}
	if len(bi.Modules) > 0 {
		bi.Modules[0].Artifacts = append(bi.Modules[0].Artifacts, arts...)
	}
	if setProps && repo != "" && len(arts) > 0 {
		if err := c.setBuildProperties(arts, repo, name, number); err != nil {
			log.Warn("cargo: failed to set build properties: " + err.Error())
		}
	}
	return c.saveBuildInfo(bi)
}

// targetRepo determines and virtual-resolves the deployment repo from the
// registry index URL configured in .cargo/config.toml.
func (c *CargoCommand) targetRepo() (string, error) {
	regName := registryNameFromArgs(c.args)
	indexURL := cargoRegistryIndexURL(c.workingDir, regName)
	repo := extractRepoNameFromURL(indexURL)
	if repo == "" || c.serverDetails == nil {
		return repo, nil
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return "", err
	}
	return resolveDeploymentRepo(repo, sm), nil
}

// setBuildProperties sets build.name/number/timestamp on the uploaded artifacts in
// a single batched SetProps call (mirrors conan's BuildPropertySetter).
func (c *CargoCommand) setBuildProperties(arts []entities.Artifact, repo, name, number string) error {
	if len(arts) == 0 || c.serverDetails == nil {
		return nil
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		return fmt.Errorf("create services manager: %w", err)
	}

	writer, err := content.NewContentWriter(content.DefaultKey, true, false)
	if err != nil {
		return err
	}
	for _, a := range arts {
		artifactPath := a.Path
		if strings.HasSuffix(a.Path, a.Name) {
			artifactPath = dirOf(a.Path)
		}
		writer.Write(specutils.ResultItem{
			Repo:        repo,
			Path:        artifactPath,
			Name:        a.Name,
			Actual_Sha1: a.Sha1,
			Actual_Md5:  a.Md5,
			Sha256:      a.Sha256,
		})
	}
	if err := writer.Close(); err != nil {
		return err
	}

	reader := content.NewContentReader(writer.GetFilePath(), content.DefaultKey)
	defer func() {
		if err := reader.Close(); err != nil {
			log.Debug("cargo: failed to close reader: " + err.Error())
		}
	}()

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	props := fmt.Sprintf("build.name=%s;build.number=%s;build.timestamp=%s", name, number, timestamp)
	if c.buildConfiguration != nil {
		if projectKey := c.buildConfiguration.GetProject(); projectKey != "" {
			props += fmt.Sprintf(";build.project=%s", projectKey)
		}
	}

	_, err = sm.SetProps(services.PropsParams{Reader: reader, Props: props, UseDebugLogs: true, IsRecursive: true})
	if err != nil {
		return fmt.Errorf("set properties: %w", err)
	}
	log.Info(fmt.Sprintf("cargo: set build properties on %d artifacts (batch)", len(arts)))
	return nil
}

// enrichChecksums fills any dependency checksums missing after local-cache resolution
// by querying Artifactory. Best-effort: logs and continues on any error.
func (c *CargoCommand) enrichChecksums(bi *entities.BuildInfo) {
	if c.serverDetails == nil {
		return
	}
	repo, err := c.targetRepo()
	if err != nil || repo == "" {
		log.Debug("cargo: no target repo for checksum enrichment; skipping")
		return
	}
	sm, err := artutils.CreateServiceManager(c.serverDetails, -1, 0, false)
	if err != nil {
		log.Debug("cargo: could not create service manager for checksum enrichment: " + err.Error())
		return
	}
	if err := enrichMissingChecksums(bi, repo, sm); err != nil {
		log.Warn("cargo: checksum enrichment failed: " + err.Error())
	}
}

// saveBuildInfo persists the collected build-info locally for later publishing.
// Mirrors conan's saveBuildInfo (commands/conan/upload.go).
func (c *CargoCommand) saveBuildInfo(buildInfo *entities.BuildInfo) error {
	service := buildUtils.CreateBuildInfoService()

	var projectKey string
	if c.buildConfiguration != nil {
		projectKey = c.buildConfiguration.GetProject()
	}
	buildInstance, err := service.GetOrCreateBuildWithProject(buildInfo.Name, buildInfo.Number, projectKey)
	if err != nil {
		return fmt.Errorf("create build: %w", err)
	}
	if err := buildInstance.SaveBuildInfo(buildInfo); err != nil {
		return fmt.Errorf("save build info: %w", err)
	}
	log.Info("cargo build info saved locally")
	return nil
}

// cargoConfigToml is the subset of .cargo/config.toml we parse.
type cargoConfigToml struct {
	Registries map[string]struct {
		Index string `toml:"index"`
	} `toml:"registries"`
}

// parseCargoRegistries reads <workingDir>/.cargo/config.toml and returns a map of
// registry name -> index URL. Returns an empty map on any read/parse error.
func parseCargoRegistries(workingDir string) map[string]string {
	out := map[string]string{}
	configPath := filepath.Join(workingDir, ".cargo", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Debug("cargo: could not read " + configPath + ": " + err.Error())
		return out
	}
	var cfg cargoConfigToml
	if err := toml.Unmarshal(data, &cfg); err != nil {
		log.Debug("cargo: could not parse " + configPath + ": " + err.Error())
		return out
	}
	for name, reg := range cfg.Registries {
		out[name] = reg.Index
	}
	return out
}

// cargoRegistryIndexURL reads <workingDir>/.cargo/config.toml and returns the
// index URL of [registries.<registryName>]. Returns "" on any error or if absent.
func cargoRegistryIndexURL(workingDir, registryName string) string {
	if registryName == "" {
		return ""
	}
	return parseCargoRegistries(workingDir)[registryName]
}

// dirOf returns the directory portion of a forward-slash repo path.
func dirOf(p string) string { return path.Dir(p) }
