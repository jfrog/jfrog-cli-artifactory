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

// listNativePluginsFunc lists installed plugins from a native agent's own registry
// (claude/codex). Tests may replace it.
var listNativePluginsFunc = pluginscommon.ListNativePlugins

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

	specs := make([]agentcommon.AgentSpec, 0, len(lc.agentNames))
	for _, agentName := range lc.agentNames {
		spec, err := agentcommon.ResolveAgent(registry, agentName, pluginscommon.RegistryHelp)
		if err != nil {
			return err
		}
		specs = append(specs, spec)
	}
	// claude/cursor/codex have no project-scoped plugin registry to list from — same
	// restriction install/update already enforce (RejectUnsupportedProjectScope). Without
	// this, a non-global list would silently show claude/codex's one global native list
	// (see buildNativeRows) under a --project-dir the plugins were never actually scoped to.
	if err := pluginscommon.RejectUnsupportedProjectScope(!lc.global, specs, "list"); err != nil {
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

// buildPluginRowsForHarness builds a sorted, limited row slice for agentName. Claude and
// Codex have their own native plugin registry, so their installed-plugin list comes from
// that CLI directly (same `plugin list --json` query `jf agent plugins update` uses to
// verify native registration) rather than from jf's local install directory — this also
// surfaces plugins jf did not itself install (e.g. Claude's official marketplace ones).
// Agents with no native registry (e.g. cursor) fall back to scanning jf's local install
// directory, as before.
func (lc *ListCommand) buildPluginRowsForHarness(registry map[string]agentcommon.AgentSpec, agentName string) ([]localListRow, error) {
	var rows []localListRow
	var err error
	if pluginscommon.HasNativeRegistry(agentName) {
		rows, err = lc.buildNativeRows(agentName)
	} else {
		rows, err = lc.buildDirRows(registry, agentName)
	}
	if err != nil {
		return nil, err
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

// buildDirRows resolves the install dir for agentName and lists installed plugins by
// scanning it directly. Used for agents with no native plugin registry (e.g. cursor).
func (lc *ListCommand) buildDirRows(registry map[string]agentcommon.AgentSpec, agentName string) ([]localListRow, error) {
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
		row, ok := lc.buildRowForPlugin(filepath.Join(dir, entry.Name()), entry.Name(), projectDir, agentName)
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// buildNativeRows lists every plugin agentName's own CLI reports as installed. The native
// registries themselves don't track a description, but the plugin's own manifest usually
// exists on disk at its native install path (e.g. .claude-plugin/plugin.json), so Description
// is read from there via the same ReadPluginMeta used by buildDirRows. A missing manifest is
// expected here (e.g. third-party marketplace plugins that ship no plugin.json), so it's only
// debug-logged, not warned about.
func (lc *ListCommand) buildNativeRows(agentName string) ([]localListRow, error) {
	entries, err := listNativePluginsFunc(agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins for %s: %w", agentName, err)
	}

	projectDir := ""
	if !lc.global {
		projectDir = lc.projectDir
	}

	rows := make([]localListRow, 0, len(entries))
	for _, e := range entries {
		description := emDash
		if meta, metaErr := pluginscommon.ReadPluginMetaForAgent(e.Path, agentName); metaErr != nil {
			log.Debug(fmt.Sprintf("Plugin '%s': could not read plugin.json (%s)", e.Slug, metaErr.Error()))
		} else {
			description = descriptionOrPlaceholder(meta.Description)
		}

		row := localListRow{
			Name:        e.Slug,
			Version:     e.Version,
			Description: description,
			Repo:        e.Repo,
			Path:        pluginDisplayPath(e.Path, projectDir, lc.global),
		}
		if lc.checkUpdates {
			lc.fillUpdateStatus(&row)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// buildRowForPlugin builds one local list row. Inclusion and installed version match update:
// ReadInstalledPluginVersion (plugin-info.json, then plugin.json). Description comes from
// agentName's own manifest convention when present (see ReadPluginMetaForAgent), so cursor's
// listing reads .cursor-plugin/plugin.json rather than always falling back to Claude's.
func (lc *ListCommand) buildRowForPlugin(pluginDir, name, projectDir, agentName string) (localListRow, bool) {
	installedVer, err := pluginscommon.ReadInstalledPluginVersion(pluginDir)
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

	description := emDash
	if meta, metaErr := pluginscommon.ReadPluginMetaForAgent(pluginDir, agentName); metaErr != nil {
		log.Warn(fmt.Sprintf("Plugin '%s': could not read plugin.json (%s)", name, metaErr.Error()))
	} else {
		description = descriptionOrPlaceholder(meta.Description)
	}

	row := localListRow{
		Name:        name,
		Version:     installedVer,
		Description: description,
		Repo:        repo,
		Path:        pluginDisplayPath(pluginDir, projectDir, lc.global),
	}
	if lc.checkUpdates {
		lc.fillUpdateStatus(&row)
	}
	return row, true
}

// descriptionOrPlaceholder returns desc, or emDash when no description is available —
// matches how fillUpdateStatus already marks other "no value" cells in this table.
func descriptionOrPlaceholder(desc string) string {
	if strings.TrimSpace(desc) == "" {
		return emDash
	}
	return desc
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
			return "$HOME/" + filepath.ToSlash(rel)
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
	isGlobal = resolveListScope(isGlobal, projectDir, rawHarness != "")
	if projectDir != "" {
		abs, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("invalid --project-dir path %q: %w", projectDir, err)
		}
		projectDir = abs
	}

	var agentNames []string
	if rawHarness != "" {
		agentNames, err = agentcommon.ParseHarnessList(rawHarness)
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

// resolveListScope applies the same project/global-scope default as install/update's
// DefaultGlobalScope: when neither --global nor --project-dir is explicitly given, list
// defaults to global scope, not project scope. claude/cursor/codex only support a global
// native registry/config anyway (see agentcommon.RejectUnsupportedProjectScope), so
// defaulting to project scope here would just make list --harness (no flags) reject in
// listLocalPlugins where install/update would have quietly gone global.
func resolveListScope(isGlobal bool, projectDir string, hasHarness bool) bool {
	if !isGlobal && projectDir == "" && hasHarness {
		return true
	}
	return isGlobal
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
