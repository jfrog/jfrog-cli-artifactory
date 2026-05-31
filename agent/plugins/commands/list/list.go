package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	pluginscommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	sortByName      = "name"
	sortByUpdated   = "updated"
	sortByDownloads = "downloads"
	sortOrderAsc    = "asc"
	sortOrderDesc   = "desc"
	emDash          = "—"

	manifestRepoUnknownDisplay = "(unknown)"
	repoListSourcePrefix       = "Repo: "

	listCheckStatusUnknown = "unknown"
	listCheckStatusBehind  = "behind"
	listCheckStatusCurrent = "current"
	listCheckStatusAhead   = "ahead"
)

// repoListRow is one row for registry mode (jf agent plugins list --repo).
type repoListRow struct {
	Name    string `json:"name" col-name:"NAME"`
	Version string `json:"version" col-name:"VERSION"`
	Source  string `json:"source" col-name:"SOURCE"`
}

// localListRow is one row for local mode (jf agent plugins list --harness).
type localListRow struct {
	Name           string `json:"name" col-name:"PLUGIN"`
	Version        string `json:"version" col-name:"INSTALLED"`
	Description    string `json:"description" col-name:"DESCRIPTION"`
	Repo           string `json:"repo" col-name:"REPO"`
	Path           string `json:"path" col-name:"PATH"`
	RegistryLatest string `json:"registryLatest,omitempty" col-name:"REGISTRY LATEST" omitempty:"true"`
	Status         string `json:"status,omitempty" col-name:"STATUS" omitempty:"true"`
}

// ListCommand lists agent plugins from Artifactory or from a local agent install directory.
type ListCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	agentName     string
	projectDir    string
	global        bool
	format        string
	limit         int
	sortBy        string
	sortOrder     string
	checkUpdates  bool
}

func (lc *ListCommand) SetServerDetails(details *config.ServerDetails) *ListCommand {
	lc.serverDetails = details
	return lc
}

func (lc *ListCommand) SetRepoKey(repoKey string) *ListCommand {
	lc.repoKey = repoKey
	return lc
}

func (lc *ListCommand) SetAgentName(agentName string) *ListCommand {
	lc.agentName = agentName
	return lc
}

func (lc *ListCommand) SetProjectDir(projectDir string) *ListCommand {
	lc.projectDir = projectDir
	return lc
}

func (lc *ListCommand) SetGlobal(isGlobal bool) *ListCommand {
	lc.global = isGlobal
	return lc
}

func (lc *ListCommand) SetFormat(format string) *ListCommand {
	lc.format = format
	return lc
}

func (lc *ListCommand) SetLimit(limit int) *ListCommand {
	lc.limit = limit
	return lc
}

func (lc *ListCommand) SetSortBy(sortBy string) *ListCommand {
	lc.sortBy = sortBy
	return lc
}

func (lc *ListCommand) SetSortOrder(sortOrder string) *ListCommand {
	lc.sortOrder = sortOrder
	return lc
}

func (lc *ListCommand) SetCheckUpdates(v bool) *ListCommand {
	lc.checkUpdates = v
	return lc
}

func (lc *ListCommand) Run() error {
	if lc.repoKey == "" && lc.agentName == "" {
		return fmt.Errorf(
			"jf agent plugins list requires exactly one of:\n"+
				"  Registry: jf agent plugins list --repo <repository-key> [--limit N] [--sort-by updated|downloads]\n"+
				"  Local:    jf agent plugins list --harness <name> [--project-dir <path>]\n"+
				"  Global:   jf agent plugins list --harness <name> --global\n\n"+
				"Supported agents: %s",
			agentcommon.SupportedAgentsList(pluginscommon.Agents, agentcommon.PluginsAgentsKey),
		)
	}
	if lc.repoKey != "" && lc.agentName != "" {
		return fmt.Errorf("--repo and --harness are mutually exclusive; specify only one")
	}
	if lc.global && lc.projectDir != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive, please choose either --global or --project-dir")
	}
	if lc.checkUpdates && lc.repoKey != "" {
		return fmt.Errorf("--check-updates is only supported with --harness, not with --repo")
	}

	if lc.agentName != "" {
		parsed, err := pluginscommon.ParseHarnessForList(lc.agentName)
		if err != nil {
			return err
		}
		lc.agentName = parsed
		if lc.checkUpdates && lc.serverDetails == nil {
			return fmt.Errorf("--check-updates requires a configured Artifactory server (same as other jf agent plugins commands)")
		}
		return lc.listLocalPlugins()
	}
	return lc.listRepoPlugins()
}

func (lc *ListCommand) listRepoPlugins() error {
	items, err := pluginscommon.ListPlugins(lc.serverDetails, lc.repoKey, lc.limit, lc.sortBy)
	if err != nil {
		return err
	}

	results := make([]repoListRow, 0, len(items))
	for _, item := range items {
		results = append(results, repoListRow{
			Name:    item.Slug,
			Version: item.LatestVersion,
			Source:  repoListSourcePrefix + lc.repoKey,
		})
	}
	return lc.printRepoResults(results)
}

func (lc *ListCommand) listLocalPlugins() error {
	registry, err := agentcommon.LoadAgentRegistry(pluginscommon.Agents, agentcommon.PluginsAgentsKey)
	if err != nil {
		return err
	}
	spec, err := agentcommon.ResolveAgent(registry, lc.agentName, pluginscommon.RegistryHelp)
	if err != nil {
		return err
	}

	dir, err := agentcommon.ResolveAgentInstallDir(spec, lc.projectDir, lc.global)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Info(fmt.Sprintf("No plugins directory found for agent %q (expected: %s)", lc.agentName, dir))
			return nil
		}
		return fmt.Errorf("failed to read plugins directory %s: %w", dir, err)
	}

	projectDir := ""
	if !lc.global {
		projectDir = lc.projectDir
	}

	var results []localListRow
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, entry.Name())
		meta, err := pluginscommon.ReadPluginMeta(pluginDir)
		if err != nil {
			log.Warn(fmt.Sprintf("Skipping plugin '%s': %s", entry.Name(), err.Error()))
			continue
		}
		manifest, err := agentcommon.ReadInstallInfoManifest(pluginDir, pluginscommon.PluginInfoManifestFile)
		if err != nil {
			log.Warn(fmt.Sprintf("Plugin '%s': invalid install manifest (%s); treating as missing", entry.Name(), err.Error()))
			manifest = nil
		}

		repo := manifestRepoUnknownDisplay
		if manifest != nil && strings.TrimSpace(manifest.Repo) != "" {
			repo = manifest.Repo
		}
		installedVer := strings.TrimSpace(meta.Version)
		if manifest != nil && strings.TrimSpace(manifest.InstalledVersion) != "" {
			installedVer = strings.TrimSpace(manifest.InstalledVersion)
		}

		row := localListRow{
			Name:        entry.Name(),
			Version:     installedVer,
			Description: meta.Description,
			Repo:        repo,
			Path:        pluginDisplayPath(pluginDir, projectDir, lc.global),
		}
		if lc.checkUpdates {
			lc.fillUpdateStatus(&row)
		}
		results = append(results, row)
	}

	desc := strings.ToLower(lc.sortOrder) == sortOrderDesc
	sort.Slice(results, func(i, j int) bool {
		ni, nj := strings.ToLower(results[i].Name), strings.ToLower(results[j].Name)
		if desc {
			return ni > nj
		}
		return ni < nj
	})

	if lc.limit > 0 && len(results) > lc.limit {
		results = results[:lc.limit]
	}

	return lc.printLocalResults(results)
}

func pluginDisplayPath(pluginDirAbs, projectDir string, global bool) string {
	if !global && projectDir != "" {
		rel, err := filepath.Rel(projectDir, pluginDirAbs)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		rel, err := filepath.Rel(home, pluginDirAbs)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(pluginDirAbs)
}

func (lc *ListCommand) fillUpdateStatus(row *localListRow) {
	if row.Repo == "" || row.Repo == manifestRepoUnknownDisplay {
		row.RegistryLatest = emDash
		row.Status = listCheckStatusUnknown
		return
	}
	latest, err := pluginscommon.ResolveLatestPluginVersion(lc.serverDetails, row.Repo, row.Name)
	if err != nil {
		row.RegistryLatest = emDash
		row.Status = listCheckStatusUnknown
		return
	}
	row.RegistryLatest = latest
	cmp, err := agentcommon.CompareSemver(row.Version, latest)
	if err != nil {
		row.Status = listCheckStatusUnknown
		return
	}
	switch {
	case cmp < 0:
		row.Status = listCheckStatusBehind
	case cmp == 0:
		row.Status = listCheckStatusCurrent
	default:
		row.Status = listCheckStatusAhead
	}
}

func (lc *ListCommand) printRepoResults(results []repoListRow) error {
	if results == nil {
		results = []repoListRow{}
	}
	if strings.EqualFold(lc.format, "json") {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if len(results) == 0 {
		log.Info("No plugins found.")
		return nil
	}
	return coreutils.PrintTable(results, "Plugins", "No plugins found", false)
}

func (lc *ListCommand) printLocalResults(results []localListRow) error {
	if results == nil {
		results = []localListRow{}
	}
	if strings.EqualFold(lc.format, "json") {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if len(results) == 0 {
		log.Info("No plugins found.")
		return nil
	}
	return coreutils.PrintTable(results, "Plugins", "No plugins found", false)
}

// RunList is the CLI action for `jf agent plugins list`.
func RunList(c *components.Context) error {
	repoKey := c.GetStringFlagValue("repo")
	agentName := strings.TrimSpace(c.GetStringFlagValue("harness"))

	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	checkUpdates := c.GetBoolFlagValue("check-updates")
	isGlobal := c.GetBoolFlagValue("global")

	limit := 0
	if limitStr := c.GetStringFlagValue("limit"); limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			return fmt.Errorf("--limit must be a positive integer, got: %q", limitStr)
		}
	}

	var sortBy, sortOrder string
	if repoKey != "" {
		sortBy = sortByUpdated
		if raw := strings.ToLower(c.GetStringFlagValue("sort-by")); raw != "" {
			if raw != sortByUpdated && raw != sortByDownloads {
				return fmt.Errorf("--sort-by for --repo accepts 'updated' or 'downloads', got: %q", raw)
			}
			sortBy = raw
		}
	} else {
		sortBy = sortByName
		sortOrder = sortOrderAsc
		if raw := strings.ToLower(c.GetStringFlagValue("sort-by")); raw != "" {
			if raw != sortByName {
				return fmt.Errorf("--sort-by for --harness only accepts 'name', got: %q", raw)
			}
			sortBy = raw
		}
		if raw := strings.ToLower(c.GetStringFlagValue("sort-order")); raw != "" {
			if raw != sortOrderAsc && raw != sortOrderDesc {
				return fmt.Errorf("--sort-order must be 'asc' or 'desc', got: %q", raw)
			}
			sortOrder = raw
		}
	}

	projectDir := c.GetStringFlagValue("project-dir")
	if !isGlobal && projectDir == "" && agentName != "" {
		projectDir = "."
	}
	if projectDir != "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("invalid --project-dir path %q: %w", projectDir, err)
		}
		projectDir = abs
	}

	cmd := &ListCommand{}
	cmd.SetRepoKey(repoKey).
		SetAgentName(agentName).
		SetProjectDir(projectDir).
		SetGlobal(isGlobal).
		SetFormat(format).
		SetLimit(limit).
		SetSortBy(sortBy).
		SetSortOrder(sortOrder).
		SetCheckUpdates(checkUpdates)

	if repoKey != "" || (agentName != "" && checkUpdates) {
		serverDetails, err := agentcommon.GetServerDetails(c)
		if err != nil {
			return err
		}
		cmd.SetServerDetails(serverDetails)
	}

	return cmd.Run()
}
