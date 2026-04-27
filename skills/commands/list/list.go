package list

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
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
	normalized := strings.ToLower(strings.TrimSpace(lc.agentName))

	agent, known := common.Agents[normalized]
	if !known {
		return fmt.Errorf("unknown agent %q. Supported agents: %s", lc.agentName, common.SupportedAgentsList())
	}

	var dir string
	if lc.projectDir != "" {
		// Project-scoped: only look in <projectDir>/<agent-relative-path>, no fallback
		dir = filepath.Join(lc.projectDir, agent.ProjectDir)
	} else {
		// Global scope
		dir = expandHome(agent.GlobalDir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No skills directory found for agent %q (expected: %s)\n", lc.agentName, dir)
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
		meta, err := readSkillMeta(skillDir)
		if err != nil {
			continue
		}
		results = append(results, listResult{
			Name:        e.Name(),
			Version:     meta.version,
			Description: meta.description,
			Source:      filepath.Join(dir, e.Name()),
		})
	}

	// Sort by name (only supported sort for local skills)
	desc := strings.ToLower(lc.sortOrder) == "desc"
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
	if len(results) == 0 {
		fmt.Println("No skills found.")
		return nil
	}
	if strings.EqualFold(lc.format, "json") {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	return coreutils.PrintTable(results, "Skills", "No skills found", false)
}

// skillMeta holds the fields we care about from SKILL.md frontmatter.
type skillMeta struct {
	name        string
	version     string
	description string
}

func readSkillMeta(skillDir string) (skillMeta, error) {
	skillMdPath := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(skillMdPath)
	if err != nil {
		return skillMeta{}, err
	}
	return parseFrontmatter(string(data)), nil
}

// parseFrontmatter extracts name, version, description from YAML frontmatter between --- delimiters.
func parseFrontmatter(content string) skillMeta {
	var meta skillMeta
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			continue
		}
		if kv := strings.SplitN(trimmed, ":", 2); len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
			switch key {
			case "name":
				meta.name = val
			case "version":
				meta.version = val
			case "description":
				meta.description = val
			}
		}
	}
	return meta
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func RunList(c *components.Context) error {
	repoKey := c.GetStringFlagValue("repo")
	agentName := c.GetStringFlagValue("agent")

	if repoKey == "" && agentName == "" {
		return fmt.Errorf("either --repo <repository> or --agent <agent-name> must be specified.\nSupported agents: %s", common.SupportedAgentsList())
	}

	format := "table"
	if f := c.GetStringFlagValue("format"); f != "" {
		format = f
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
		if o := c.GetStringFlagValue("sort-order"); o != "" {
			return fmt.Errorf("--sort-order is not supported with --repo")
		}
		sortBy = "updated"
		if s := c.GetStringFlagValue("sort-by"); s != "" {
			s = strings.ToLower(s)
			if s != "updated" && s != "downloads" {
				return fmt.Errorf("--sort-by for --repo accepts 'updated' or 'downloads', got: %q", s)
			}
			sortBy = s
		}
	} else {
		// --agent / --project-dir: sort-by accepts name (default); sort-order asc/desc
		sortBy = "name"
		if s := c.GetStringFlagValue("sort-by"); s != "" {
			s = strings.ToLower(s)
			if s != "name" {
				return fmt.Errorf("--sort-by for --agent only accepts 'name', got: %q", s)
			}
			sortBy = s
		}
		sortOrder = "asc"
		if o := c.GetStringFlagValue("sort-order"); o != "" {
			o = strings.ToLower(o)
			if o != "asc" && o != "desc" {
				return fmt.Errorf("--sort-order must be 'asc' or 'desc', got: %q", o)
			}
			sortOrder = o
		}
	}

	projectDir := ""
	if pd := c.GetStringFlagValue("project-dir"); pd != "" {
		absPath, err := filepath.Abs(pd)
		if err != nil {
			return fmt.Errorf("invalid --project-dir path %q: %w", pd, err)
		}
		projectDir = absPath
	}

	cmd := &ListCommand{}
	cmd.SetRepoKey(repoKey).
		SetAgentName(agentName).
		SetProjectDir(projectDir).
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
