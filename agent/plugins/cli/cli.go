package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/update"
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/flagkit"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

// GetSubCommands returns leaf commands for `jf agent plugins` (publish, install, delete, …).
func GetSubCommands() []components.Command {
	return []components.Command{
		{
			Name:        "publish",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsPublish),
			Description: "Publish an agent plugin to Artifactory. Signs and attaches evidence if a signing key is provided.",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
		{
			Name:  "install",
			Flags: flagkit.GetCommandFlags(flagkit.AgentPluginsInstall),
			Description: "Install an agent plugin from Artifactory. Use --harness <name[,name...]> with " +
				"--project-dir (default: current directory) or --global; or --path <dir> for a direct install " +
				"at <dir>/<slug>. If --version is omitted with --harness, the install command downloads each " +
				"<harness>-marketplace.json and uses the version listed there (all must match); with --path, the latest " +
				"published version is used. Use --format json for machine-readable install summary.",
			Arguments: getInstallArguments(),
			Action:    install.RunInstall,
		},
		{
			Name:  "update",
			Flags: flagkit.GetCommandFlags(flagkit.AgentPluginsUpdate),
			Description: "Update an installed plugin to the latest (or a specific) version. " +
				"Use --slug with --harness (comma-separated) and --project-dir or --global; or --slug with --path <dir>. " +
				"With --all (requires --harness), updates every discovered plugin under those harnesses to latest in one summary table " +
				"(interactive confirmation before proceeding; folder name is the slug, same as --slug). " +
				"Resolves versions directly from Artifactory (no marketplace lookup). " +
				"Skips targets not installed or already at the target version (use --force to re-download).",
			Action: update.RunUpdate,
		},
		{
			Name:        "delete",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsDelete),
			Description: "Delete a specific agent plugin version from Artifactory.",
			Arguments:   getDeleteArguments(),
			Action:      delete.RunDelete,
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the plugin folder containing plugin.json.",
		},
	}
}

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Slug (name) of the plugin to install.",
		},
	}
}

func getDeleteArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Plugin name/slug to delete.",
		},
	}
}
