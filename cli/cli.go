package cli

import (
	artifactoryCLI "github.com/jfrog/jfrog-cli-artifactory/artifactory/cli"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide/jetbrains"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide/vscode"
	distributionCLI "github.com/jfrog/jfrog-cli-artifactory/distribution/cli"
	evidenceCLI "github.com/jfrog/jfrog-cli-artifactory/evidence/cli"
	ideCLI "github.com/jfrog/jfrog-cli-artifactory/ide/cli"
	"github.com/jfrog/jfrog-cli-artifactory/lifecycle"
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
		Name:        "evd",
		Description: "Evidence commands.",
		Commands:    evidenceCLI.GetCommands(),
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
	app.Commands = append(app.Commands, lifecycle.GetCommands()...)

	// Add IDE commands as top-level commands
	app.Commands = append(app.Commands, getTopLevelIDECommands()...)

	return app
}

// getTopLevelIDECommands returns IDE commands configured for top-level access
func getTopLevelIDECommands() []components.Command {
	// Get the original IDE commands
	vscodeCommands := vscode.GetCommands()
	jetbrainsCommands := jetbrains.GetCommands()

	// Use centralized descriptions
	if len(vscodeCommands) > 0 {
		vscodeCommands[0].Description = ide.VscodeConfigDescription
	}
	if len(jetbrainsCommands) > 0 {
		jetbrainsCommands[0].Description = ide.JetbrainsConfigDescription
		jetbrainsCommands[0].Aliases = append(jetbrainsCommands[0].Aliases, "jb")
	}

	return append(vscodeCommands, jetbrainsCommands...)
}
