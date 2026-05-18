package cli

import (
	agentpluginsCLI "github.com/jfrog/jfrog-cli-artifactory/agentplugins/cli"
	artifactoryCLI "github.com/jfrog/jfrog-cli-artifactory/artifactory/cli"
	distributionCLI "github.com/jfrog/jfrog-cli-artifactory/distribution/cli"
	ideCLI "github.com/jfrog/jfrog-cli-artifactory/ide/cli"
	"github.com/jfrog/jfrog-cli-artifactory/lifecycle"
	skillsCLI "github.com/jfrog/jfrog-cli-artifactory/skills/cli"
	"github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetJfrogCliArtifactoryApp() components.App {
	app := components.CreateEmbeddedApp(
		"artifactory",
		[]components.Command{},
	)
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        string(cliutils.Ds),
		Description: "Distribution V1 commands.",
		Commands:    distributionCLI.GetCommands(),
		Category:    "Command Namespaces",
	})
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        string(cliutils.Rt),
		Description: "Artifactory commands.",
		Commands:    artifactoryCLI.GetCommands(),
		Category:    "Command Namespaces",
	})
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        "ide",
		Description: "IDE commands.",
		Commands:    ideCLI.GetCommands(),
		Category:    "Command Namespaces",
	})
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        "skills",
		Aliases:     []string{"skill"},
		Description: "Skills commands.",
		Hidden:      true,
		Commands:    skillsCLI.GetCommands(),
		Category:    "Command Namespaces",
	})
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        "ai",
		Description: "AI agent artifacts (plugins).",
		Commands:    agentpluginsCLI.GetAiCommands(),
		Category:    "Command Namespaces",
	})
	app.Subcommands = append(app.Subcommands, components.Namespace{
		Name:        "ai-plugins",
		Description: "AI agent plugins (flat form of 'jf ai plugins').",
		Hidden:      true,
		Commands:    agentpluginsCLI.GetAgentPluginsCommands(),
		Category:    "Command Namespaces",
	})
	app.Commands = append(app.Commands, lifecycle.GetCommands()...)

	return app
}
