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
)

type listResult struct {
	Name        string `json:"name" col-name:"Name"`
	Version     string `json:"version" col-name:"Version"`
	Description string `json:"description" col-name:"Description"`
	Source      string `json:"source" col-name:"Source"`
}

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

func (lc *ListCommand) SetGlobal(useGlobal bool) *ListCommand {
	lc.global = useGlobal
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

func (lc *ListCommand) Run() error {
	if lc.repoKey == "" && lc.agentName == "" {
		return fmt.Errorf("either --repo <repository> or --agent <agent-name> must be specified.\nSupported agents: %s", common.SupportedAgentsList())
	}
	if lc.repoKey != "" && lc.agentName != "" {
		return fmt.Errorf("--repo and --agent are mutually exclusive; specify only one")
	}
	if lc.global && lc.projectDir != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive")
	}

	if lc.agentName != "" {
		return lc.listLocalSkills()
	}
	return lc.listRepoSkills()
}

func (lc *ListCommand) listRepoSkills() error {
	items, err := common.ListSkills(lc.serverDetails, lc.repoKey, lc.limit, lc.sortBy)
	if err != nil {
		return err
	}

	results := make([]listResult, 0, len(items))
	for _, item := range items {
		name := item.Slug
		latestVersion := ""
		if item.LatestVersion != nil {
			latestVersion = item.LatestVersion.Version
		}
		results = append(results, listResult{
			Name:        name,
			Version:     latestVersion,
			Description: item.Summary,
			Source:      "Repo: " + lc.repoKey,
		})
	}
	return lc.printResults(results)
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

	var results []listResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, e.Name())
		meta, err := publish.ParseSkillMeta(skillDir)
		if err != nil {
			log.Warn(fmt.Sprintf("Skipping skill '%s':\n %s", e.Name(), err.Error()))
			continue
		}
		results = append(results, listResult{
			Name:        e.Name(),
			Version:     meta.Version,
			Description: meta.Description,
			Source:      filepath.Join(dir, e.Name()),
		})
	}

	// Sort by name (only supported sort for local skills)
	desc := strings.ToLower(lc.sortOrder) == sortOrderDesc
	sort.Slice(results, func(i, j int) bool {
		ni, nj := strings.ToLower(results[i].Name), strings.ToLower(results[j].Name)
		if desc {
			return ni > nj
		}
		return ni < nj
	})

	// Apply --limit
	if lc.limit > 0 && len(results) > lc.limit {
		results = results[:lc.limit]
	}

	return lc.printResults(results)
}

func (lc *ListCommand) printResults(results []listResult) error {
	if results == nil {
		results = []listResult{}
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

func RunList(c *components.Context) error {
	repoKey := c.GetStringFlagValue("repo")
	agentName := c.GetStringFlagValue("agent")

	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}

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
		// --repo: sort-by accepts updated (default) or downloads; sort-order not supported
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
		// --agent / --project-dir: sort-by accepts name (default); sort-order asc/desc
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
	useGlobal := c.GetBoolFlagValue("global")
	if useGlobal && projectDir != "" {
		return fmt.Errorf("--global and --project-dir are mutually exclusive")
	}
	// --agent without --project-dir/--global: use cwd
	if !useGlobal && projectDir == "" && agentName != "" {
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
		SetGlobal(useGlobal).
		SetFormat(format).
		SetLimit(limit).
		SetSortBy(sortBy).
		SetSortOrder(sortOrder)

	if repoKey != "" {
		serverDetails, err := common.GetServerDetails(c)
		if err != nil {
			return err
		}
		cmd.SetServerDetails(serverDetails)
	}

	return cmd.Run()
}
