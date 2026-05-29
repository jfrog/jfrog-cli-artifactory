package delete

import (
	"fmt"

	agentcommon "github.com/jfrog/jfrog-cli-artifactory/agent/common"
	pluginscommon "github.com/jfrog/jfrog-cli-artifactory/agent/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// DeleteCommand deletes a specific agent plugin version from a repository.
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
	return "plugins_delete"
}

func (dc *DeleteCommand) Run() error {
	if dc.version == "" {
		return fmt.Errorf("--version is required for delete")
	}

	deletePath := fmt.Sprintf("%s/%s/%s/", dc.repoKey, dc.slug, dc.version)

	if dc.dryRun {
		if dc.serverDetails != nil {
			exists, err := agentcommon.PackageVersionExists(dc.serverDetails, dc.repoKey, dc.slug, dc.version)
			if err != nil {
				return fmt.Errorf("failed to verify plugin existence: %w", err)
			}
			if !exists {
				return fmt.Errorf("plugin '%s' v%s not found in repository '%s'", dc.slug, dc.version, dc.repoKey)
			}
		}
		log.Info(fmt.Sprintf("[DRY RUN] Would delete plugin '%s' v%s from '%s' (path: %s)", dc.slug, dc.version, dc.repoKey, deletePath))
		return nil
	}

	if err := agentcommon.DeleteVersion(dc.serverDetails, dc.repoKey, dc.slug, dc.version); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Plugin '%s' v%s deleted from '%s'.", dc.slug, dc.version, dc.repoKey))
	return nil
}

// RunDelete is the CLI action for `jf agent plugins delete`.
func RunDelete(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf agent plugins delete <slug> --version <version> [--repo <repo>] [options]")
	}

	slug := c.GetArgumentAt(0)

	serverDetails, err := agentcommon.GetServerDetails(c)
	if err != nil {
		return err
	}

	repoKey, err := agentcommon.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), agentcommon.IsQuiet(c), pluginscommon.RepoOptions())
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
