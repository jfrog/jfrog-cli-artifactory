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
	agentNames    []string
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

func (lc *ListCommand) SetAgentNames(names []string) *ListCommand {
	lc.agentNames = names
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
	if lc.repoKey == "" && len(lc.agentNames) == 0 {
		return fmt.Errorf(
			"jf agent plugins list requires exactly one of:\n"+
				"  Registry: jf agent plugins list --repo <repository-key> [--limit N]\n"+
				"  Local:    jf agent plugins list --harness <name[,name...]> [--project-dir <path>]\n"+
				"  Global:   jf agent plugins list --harness <name[,name...]> --global\n\n"+
				"Supported agents: %s",
			agentcommon.SupportedAgentsList(pluginscommon.Agents, agentcommon.PluginsAgentsKey),
		)
	}
	if lc.repoKey != "" && len(lc.agentNames) > 0 {
		return fmt.Errorf("--repo and --harness are mutually exclusive; specify only one")
	}
	if lc.global && lc.projectDir != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive, please choose either --global or --project-dir")
	}
	if lc.checkUpdates && lc.repoKey != "" {
		return fmt.Errorf("--check-updates is only supported with --harness, not with --repo")
	}
	if lc.checkUpdates && lc.serverDetails == nil {
		return fmt.Errorf("--check-updates requires a configured Artifactory server (same as other jf agent plugins commands)")
	}

	if len(lc.agentNames) > 0 {
		return lc.listLocalPlugins()
	}
	return lc.listRepoPlugins()
}

func (lc *ListCommand) listRepoPlugins() error {
	pluginEntries, err := pluginscommon.ListPlugins(lc.serverDetails, lc.repoKey, lc.limit)
	if err != nil {
		return err
	}

	rows := make([]repoListRow, 0, len(pluginEntries))
	for _, entry := range pluginEntries {
		rows = append(rows, repoListRow{
			Name:    entry.Slug,
			Version: entry.LatestVersion,
			Source:  repoListSourcePrefix + lc.repoKey,
		})
	}
	return lc.printRepoResults(rows)
}

// listLocalPlugins lists installed plugins for each harness in lc.agentNames.
// For multiple harnesses, each gets its own labelled table.
func (lc *ListCommand) listLocalPlugins() error {
	registry, err := agentcommon.LoadAgentRegistry(pluginscommon.Agents, agentcommon.PluginsAgentsKey)
	if err != nil {
		return err
	}

	// For JSON with multiple harnesses, collect into a map keyed by harness name.
	if strings.EqualFold(lc.format, "json") && len(lc.agentNames) > 1 {
		allResults := make(map[string][]localListRow, len(lc.agentNames))
		for _, agentName := range lc.agentNames {
			rows, err := lc.buildPluginRowsForHarness(registry, agentName)
			if err != nil {
				return err
			}
			if rows == nil {
				rows = []localListRow{}
			}
			allResults[agentName] = rows
		}
		data, err := json.MarshalIndent(allResults, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	for _, agentName := range lc.agentNames {
		rows, err := lc.buildPluginRowsForHarness(registry, agentName)
		if err != nil {
			return err
		}
		if err := lc.printLocalResults(rows, agentName); err != nil {
			return err
		}
	}
	return nil
}

// buildPluginRowsForHarness resolves the install dir for agentName and builds a sorted, limited row slice.
func (lc *ListCommand) buildPluginRowsForHarness(registry map[string]agentcommon.AgentSpec, agentName string) ([]localListRow, error) {
	spec, err := agentcommon.ResolveAgent(registry, agentName, pluginscommon.RegistryHelp)
	if err != nil {
		return nil, err
	}

	dir, err := agentcommon.ResolveAgentInstallDir(spec, lc.projectDir, lc.global)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Info(fmt.Sprintf("No plugins directory found for agent %q (expected: %s)", agentName, dir))
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read plugins directory %s: %w", dir, err)
	}

	projectDir := ""
	if !lc.global {
		projectDir = lc.projectDir
	}

	var rows []localListRow
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		row, ok := lc.buildRowForPlugin(filepath.Join(dir, entry.Name()), entry.Name(), projectDir)
		if ok {
			rows = append(rows, row)
		}
	}

	desc := strings.ToLower(lc.sortOrder) == sortOrderDesc
	sort.Slice(rows, func(i, j int) bool {
		ni, nj := strings.ToLower(rows[i].Name), strings.ToLower(rows[j].Name)
		if desc {
			return ni > nj
		}
		return ni < nj
	})

	if lc.limit > 0 && len(rows) > lc.limit {
		rows = rows[:lc.limit]
	}
	return rows, nil
}

// buildRowForPlugin reads plugin.json and install manifest for a single plugin directory.
// Returns (row, true) on success, (zero, false) if the plugin should be skipped.
func (lc *ListCommand) buildRowForPlugin(pluginDir, name, projectDir string) (localListRow, bool) {
	meta, err := pluginscommon.ReadPluginMeta(pluginDir)
	if err != nil {
		log.Warn(fmt.Sprintf("Skipping plugin '%s': %s", name, err.Error()))
		return localListRow{}, false
	}

	manifest, err := agentcommon.ReadInstallInfoManifest(pluginDir, pluginscommon.PluginInfoManifestFile)
	if err != nil {
		log.Warn(fmt.Sprintf("Plugin '%s': invalid install manifest (%s); treating as missing", name, err.Error()))
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
		Name:        name,
		Version:     installedVer,
		Description: meta.Description,
		Repo:        repo,
		Path:        pluginDisplayPath(pluginDir, projectDir, lc.global),
	}
	if lc.checkUpdates {
		lc.fillUpdateStatus(&row)
	}
	return row, true
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

func (lc *ListCommand) printRepoResults(rows []repoListRow) error {
	if strings.EqualFold(lc.format, "json") {
		if rows == nil {
			rows = []repoListRow{}
		}
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if len(rows) == 0 {
		log.Info("No plugins found.")
		return nil
	}
	return coreutils.PrintTable(rows, "Plugins", "No plugins found", false)
}

// printLocalResults prints one harness's plugin rows. When multiple harnesses are listed,
// agentName is used as a section label above the table.
func (lc *ListCommand) printLocalResults(rows []localListRow, agentName string) error {
	if strings.EqualFold(lc.format, "json") {
		if rows == nil {
			rows = []localListRow{}
		}
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if len(lc.agentNames) > 1 {
		log.Output(fmt.Sprintf("\n%s", agentName))
	}
	if len(rows) == 0 {
		log.Info("No plugins found.")
		return nil
	}
	return coreutils.PrintTable(rows, "Plugins", "No plugins found", false)
}

// RunList is the CLI action for `jf agent plugins list`.
func RunList(c *components.Context) error {
	repoKey := c.GetStringFlagValue("repo")
	rawHarness := strings.TrimSpace(c.GetStringFlagValue("harness"))

	format := "table"
	if v := c.GetStringFlagValue("format"); v != "" {
		format = v
	}

	limit, err := parseLimitFlag(c)
	if err != nil {
		return err
	}

	sortBy, sortOrder, err := parseSortConfig(c, repoKey)
	if err != nil {
		return err
	}

	isGlobal := c.GetBoolFlagValue("global")
	checkUpdates := c.GetBoolFlagValue("check-updates")

	projectDir := c.GetStringFlagValue("project-dir")
	if !isGlobal && projectDir == "" && rawHarness != "" {
		projectDir = "."
	}
	if projectDir != "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("invalid --project-dir path %q: %w", projectDir, err)
		}
		projectDir = abs
	}

	var agentNames []string
	if rawHarness != "" {
		agentNames, err = pluginscommon.ParseHarnessList(rawHarness)
		if err != nil {
			return err
		}
	}

	cmd := &ListCommand{}
	cmd.SetRepoKey(repoKey).
		SetAgentNames(agentNames).
		SetProjectDir(projectDir).
		SetGlobal(isGlobal).
		SetFormat(format).
		SetLimit(limit).
		SetSortBy(sortBy).
		SetSortOrder(sortOrder).
		SetCheckUpdates(checkUpdates)

	if repoKey != "" || (len(agentNames) > 0 && checkUpdates) {
		serverDetails, err := agentcommon.GetServerDetails(c)
		if err != nil {
			return err
		}
		cmd.SetServerDetails(serverDetails)
	}

	return cmd.Run()
}

func parseLimitFlag(c *components.Context) (int, error) {
	limitStr := c.GetStringFlagValue("limit")
	if limitStr == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("--limit must be a positive integer, got: %q", limitStr)
	}
	return limit, nil
}

func parseSortConfig(c *components.Context, repoKey string) (sortBy, sortOrder string, err error) {
	if repoKey != "" {
		sortBy = sortByUpdated
		if raw := strings.ToLower(c.GetStringFlagValue("sort-by")); raw != "" {
			if raw != sortByUpdated && raw != sortByDownloads {
				return "", "", fmt.Errorf("--sort-by for --repo accepts 'updated' or 'downloads', got: %q", raw)
			}
			sortBy = raw
		}
		return sortBy, "", nil
	}

	sortBy = sortByName
	sortOrder = sortOrderAsc
	if raw := strings.ToLower(c.GetStringFlagValue("sort-by")); raw != "" {
		if raw != sortByName {
			return "", "", fmt.Errorf("--sort-by for --harness only accepts 'name', got: %q", raw)
		}
		sortBy = raw
	}
	if raw := strings.ToLower(c.GetStringFlagValue("sort-order")); raw != "" {
		if raw != sortOrderAsc && raw != sortOrderDesc {
			return "", "", fmt.Errorf("--sort-order must be 'asc' or 'desc', got: %q", raw)
		}
		sortOrder = raw
	}
	return sortBy, sortOrder, nil
}
