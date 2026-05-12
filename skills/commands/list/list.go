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

	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	sortByUpdated   = "updated"
	sortByDownloads = "downloads"
	sortByName      = "name"
	sortOrderAsc    = "asc"
	sortOrderDesc   = "desc"
	emDash          = "\u2014"

	manifestRepoUnknownDisplay = "(unknown)"
	repoListSourcePrefix       = "Repo: "

	listCheckStatusUnknown = "unknown"
	listCheckStatusBehind  = "behind"
	listCheckStatusCurrent = "current"
)

// repoListRow is one row for registry mode (jf skills list --repo).
type repoListRow struct {
	Name        string `json:"name" col-name:"Name"`
	Version     string `json:"version" col-name:"Version"`
	Description string `json:"description" col-name:"Description"`
	Source      string `json:"source" col-name:"Source"`
}

// localListRow is one row for local mode (jf skills list --agent).
type localListRow struct {
	Name           string `json:"name" col-name:"SKILL"`
	Version        string `json:"version" col-name:"INSTALLED"`
	Description    string `json:"description" col-name:"Description"`
	Repo           string `json:"repo" col-name:"REPO"`
	Path           string `json:"path" col-name:"PATH"`
	RegistryLatest string `json:"registryLatest,omitempty" col-name:"REGISTRY LATEST" omitempty:"true"`
	Status         string `json:"status,omitempty" col-name:"STATUS" omitempty:"true"`
}

// ListCommand lists skills from Artifactory or from a local agent install directory.
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

// ErrSkillsListNoMode is returned when neither --repo nor --agent is set.
func ErrSkillsListNoMode() error {
	return fmt.Errorf(
		"jf skills list requires exactly one of:\n"+
			"  Registry:  jf skills list --repo <repository-key> [--limit N] [--sort-by updated|downloads]\n"+
			"  Project:   jf skills list --agent <id> [--project-dir <path>]   (default --project-dir: current directory)\n"+
			"  Global:    jf skills list --agent <id> --global\n"+
			"\n"+
			"Notes:\n"+
			"  --repo and --agent are mutually exclusive.\n"+
			"  With --agent, --global and --project-dir are mutually exclusive.\n"+
			"  With --agent, add --check-updates to compare installs to the registry (needs jf config server).\n"+
			"\n"+
			"Supported agents: %s",
		common.SupportedAgentsList(),
	)
}

// Run validates flags and prints the listing.
func (lc *ListCommand) Run() error {
	if lc.repoKey == "" && lc.agentName == "" {
		return ErrSkillsListNoMode()
	}
	if lc.repoKey != "" && lc.agentName != "" {
		return fmt.Errorf("--repo and --agent are mutually exclusive; specify only one")
	}
	if lc.global && lc.projectDir != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive, please choose either --global or --project-dir")
	}

	if lc.agentName != "" {
		if lc.checkUpdates && lc.serverDetails == nil {
			return fmt.Errorf("--check-updates requires a configured Artifactory server (same as other jf skills commands)")
		}
		return lc.listLocalSkills()
	}
	return lc.listRepoSkills()
}

func (lc *ListCommand) listRepoSkills() error {
	items, err := common.ListSkills(lc.serverDetails, lc.repoKey, lc.limit, lc.sortBy)
	if err != nil {
		return err
	}

	results := make([]repoListRow, 0, len(items))
	for _, item := range items {
		name := item.Slug
		latestVersion := ""
		if item.LatestVersion != nil {
			latestVersion = item.LatestVersion.Version
		}
		results = append(results, repoListRow{
			Name:        name,
			Version:     latestVersion,
			Description: item.Summary,
			Source:      repoListSourcePrefix + lc.repoKey,
		})
	}
	return lc.printRepoResults(results)
}

func skillDisplayPath(skillDirAbs, projectDir string, global bool) string {
	if !global && projectDir != "" {
		relativeToProject, err := filepath.Rel(projectDir, skillDirAbs)
		if err == nil && relativeToProject != "." && !strings.HasPrefix(relativeToProject, "..") {
			return filepath.ToSlash(relativeToProject)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		relativeToHome, err := filepath.Rel(home, skillDirAbs)
		if err == nil && relativeToHome != "." && !strings.HasPrefix(relativeToHome, "..") {
			return "~/" + filepath.ToSlash(relativeToHome)
		}
	}
	return filepath.ToSlash(skillDirAbs)
}

func (lc *ListCommand) listLocalSkills() error {
	registry, err := common.LoadAgentRegistry()
	if err != nil {
		return err
	}
	spec, err := common.ResolveAgent(registry, lc.agentName)
	if err != nil {
		return err
	}

	dir, err := common.ResolveAgentInstallDir(spec, lc.projectDir, lc.global)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Info(fmt.Sprintf("No skills directory found for agent %q (expected: %s)", lc.agentName, dir))
			return nil
		}
		return fmt.Errorf("failed to read skills directory %s: %w", dir, err)
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
		skillDir := filepath.Join(dir, entry.Name())
		meta, err := publish.ParseSkillMeta(skillDir)
		if err != nil {
			log.Warn(fmt.Sprintf("Skipping skill '%s':\n %s", entry.Name(), err.Error()))
			continue
		}
		manifest, err := common.ReadSkillInfoManifest(skillDir)
		if err != nil {
			log.Warn(fmt.Sprintf("Skill '%s': invalid install manifest (%s); treating as missing", entry.Name(), err.Error()))
			manifest = nil
		}
		slugForAPI := entry.Name()
		if manifest != nil && manifest.Slug != "" && manifest.Slug != entry.Name() {
			log.Warn(fmt.Sprintf("Manifest slug %q differs from directory name %q; using directory name for registry lookups", manifest.Slug, entry.Name()))
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
			Path:        skillDisplayPath(skillDir, projectDir, lc.global),
		}
		if lc.checkUpdates {
			lc.fillUpdateStatus(&row, slugForAPI)
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

func (lc *ListCommand) fillUpdateStatus(row *localListRow, slug string) {
	if row.Repo == "" || row.Repo == manifestRepoUnknownDisplay {
		row.RegistryLatest = emDash
		row.Status = listCheckStatusUnknown
		return
	}
	versions, err := common.ListVersions(lc.serverDetails, row.Repo, slug)
	if err != nil || len(versions) == 0 {
		row.RegistryLatest = emDash
		row.Status = listCheckStatusUnknown
		return
	}
	available := make([]string, len(versions))
	for versionIndex, skillVersion := range versions {
		available[versionIndex] = skillVersion.Version
	}
	latest, err := common.LatestVersion(available)
	if err != nil {
		row.RegistryLatest = emDash
		row.Status = listCheckStatusUnknown
		return
	}
	row.RegistryLatest = latest
	semverComparison, err := common.CompareSemver(row.Version, latest)
	if err != nil {
		row.Status = listCheckStatusUnknown
		return
	}
	switch {
	case semverComparison < 0:
		row.Status = listCheckStatusBehind
	case semverComparison == 0:
		row.Status = listCheckStatusCurrent
	default:
		row.Status = listCheckStatusCurrent
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
		log.Info("No skills found.")
		return nil
	}
	return coreutils.PrintTable(results, "Skills", "No skills found", false)
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
		log.Info("No skills found.")
		return nil
	}
	return coreutils.PrintTable(results, "Skills", "No skills found", false)
}

// RunList is the CLI action for `jf skills list`.
func RunList(c *components.Context) error {
	repoKey := c.GetStringFlagValue("repo")
	agentName := c.GetStringFlagValue("agent")

	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

	checkUpdates := c.GetBoolFlagValue("check-updates")
	if checkUpdates && repoKey != "" {
		return fmt.Errorf("--check-updates is only supported with --agent, not with --repo")
	}

	isGlobal := c.GetBoolFlagValue("global")

	limit := 0
	if limitStr := c.GetStringFlagValue("limit"); limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			return fmt.Errorf("--limit must be a positive integer, got: %q", limitStr)
		}
	}

	var sortBy string
	var sortOrder string

	if repoKey != "" {
		if c.GetStringFlagValue("sort-order") != "" {
			return fmt.Errorf("--sort-order is not supported with --repo")
		}
		sortBy = sortByUpdated
		if rawSortBy := strings.ToLower(c.GetStringFlagValue("sort-by")); rawSortBy != "" {
			if rawSortBy != sortByUpdated && rawSortBy != sortByDownloads {
				return fmt.Errorf("--sort-by for --repo accepts 'updated' or 'downloads', got: %q", rawSortBy)
			}
			sortBy = rawSortBy
		}
	} else {
		sortBy = sortByName
		if rawSortBy := strings.ToLower(c.GetStringFlagValue("sort-by")); rawSortBy != "" {
			if rawSortBy != sortByName {
				return fmt.Errorf("--sort-by for --agent only accepts 'name', got: %q", rawSortBy)
			}
			sortBy = rawSortBy
		}
		sortOrder = sortOrderAsc
		if rawSortOrder := strings.ToLower(c.GetStringFlagValue("sort-order")); rawSortOrder != "" {
			if rawSortOrder != sortOrderAsc && rawSortOrder != sortOrderDesc {
				return fmt.Errorf("--sort-order must be 'asc' or 'desc', got: %q", rawSortOrder)
			}
			sortOrder = rawSortOrder
		}
	}

	projectDir := c.GetStringFlagValue("project-dir")
	if !isGlobal && projectDir == "" && agentName != "" {
		projectDir = "."
	}
	if projectDir != "" {
		absPath, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("invalid --project-dir path %q: %w", projectDir, err)
		}
		projectDir = absPath
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
		serverDetails, err := common.GetServerDetails(c)
		if err != nil {
			return err
		}
		cmd.SetServerDetails(serverDetails)
	}

	return cmd.Run()
}
