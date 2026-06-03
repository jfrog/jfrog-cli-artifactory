package cli

import (
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/delete"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/install"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/list"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/agent/plugins/commands/search"
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
			Description: "Publish an agent plugin to Artifactory.",
			Arguments:   getPublishArguments(),
			Action:      publish.RunPublish,
		},
		{
			Name:        "install",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsInstall),
			Description: "Install an agent plugin from Artifactory.",
			Arguments:   getInstallArguments(),
			Action:      install.RunInstall,
		},
		{
			Name:        "update",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsUpdate),
			Description: "Update an installed agent plugin.",
			Action:      update.RunUpdate,
		},
		{
			Name:        "delete",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsDelete),
			Description: "Delete a specific agent plugin version from Artifactory.",
			Arguments:   getDeleteArguments(),
			Action:      delete.RunDelete,
		},
		{
			Name:        "list",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsList),
			Description: "List agent plugins from Artifactory or on the local machine.",
			Action:      list.RunList,
		},
		{
			Name:        "search",
			Flags:       flagkit.GetCommandFlags(flagkit.AgentPluginsSearch),
			Description: "Search for agent plugins in Artifactory.",
			Arguments:   getSearchArguments(),
			Action:      search.RunSearch,
		},
	}
}

func getSearchArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "query",
			Description: "Agent plugin name or search term.",
		},
	}
}

func getPublishArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "path",
			Description: "Path to the agent plugin folder containing plugin.json.",
		},
	}
}

func getInstallArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Agent plugin slug to install.",
		},
	}
}

func getDeleteArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "slug",
			Description: "Agent plugin slug to delete.",
		},
	}
}
