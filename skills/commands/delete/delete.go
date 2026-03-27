package delete

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// DeleteCommand deletes a specific skill version from a repository.
type DeleteCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	dryRun        bool
}

func NewDeleteCommand() *DeleteCommand {
	return &DeleteCommand{}
}

func (dc *DeleteCommand) SetServerDetails(details *config.ServerDetails) *DeleteCommand {
	dc.serverDetails = details
	return dc
}

func (dc *DeleteCommand) SetRepoKey(repoKey string) *DeleteCommand {
	dc.repoKey = repoKey
	return dc
}

func (dc *DeleteCommand) SetSlug(slug string) *DeleteCommand {
	dc.slug = slug
	return dc
}

func (dc *DeleteCommand) SetVersion(version string) *DeleteCommand {
	dc.version = version
	return dc
}

func (dc *DeleteCommand) SetDryRun(dryRun bool) *DeleteCommand {
	dc.dryRun = dryRun
	return dc
}

func (dc *DeleteCommand) ServerDetails() (*config.ServerDetails, error) {
	return dc.serverDetails, nil
}

func (dc *DeleteCommand) CommandName() string {
	return "skills_delete"
}

func (dc *DeleteCommand) Run() error {
	if dc.version == "" {
		return fmt.Errorf("--version is required for delete")
	}

	deletePath := fmt.Sprintf("%s/%s/%s/", dc.repoKey, dc.slug, dc.version)

	if dc.dryRun {
		if dc.serverDetails != nil {
			exists, err := common.VersionExists(dc.serverDetails, dc.repoKey, dc.slug, dc.version)
			if err != nil {
				if strings.Contains(err.Error(), "404 Not Found") {
					return fmt.Errorf("repository '%s' or skill '%s' not found", dc.repoKey, dc.slug)
				}
				return fmt.Errorf("failed to verify skill existence: %w", err)
			}
			if !exists {
				return fmt.Errorf("skill '%s' v%s not found in repository '%s'", dc.slug, dc.version, dc.repoKey)
			}
		}
		log.Info(fmt.Sprintf("[DRY RUN] Would delete skill '%s' v%s from '%s' (path: %s)", dc.slug, dc.version, dc.repoKey, deletePath))
		return nil
	}

	if err := common.DeleteSkillVersion(dc.serverDetails, dc.repoKey, dc.slug, dc.version); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Skill '%s' v%s deleted from '%s'.", dc.slug, dc.version, dc.repoKey))
	return nil
}

// RunDelete is the CLI action for `jf skills delete`.
func RunDelete(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills delete <slug> --version <version> [--repo <repo>] [options]")
	}

	slug := c.GetArgumentAt(0)

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := common.IsQuiet(c)
	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	cmd := NewDeleteCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(c.GetStringFlagValue("version")).
		SetDryRun(c.GetBoolFlagValue("dry-run"))

	return cmd.Run()
}
