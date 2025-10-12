package cli

import (
	artifactoryCLI "github.com/jfrog/jfrog-cli-artifactory/artifactory/cli"
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

	return app
}
