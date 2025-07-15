package cli

import (
	artifactoryCLI "github.com/jfrog/jfrog-cli-artifactory/artifactory/cli"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide/jetbrains"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/cli/ide/vscode"
	distributionCLI "github.com/jfrog/jfrog-cli-artifactory/distribution/cli"
	evidenceCLI "github.com/jfrog/jfrog-cli-artifactory/evidence/cli"
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

	// Modify VSCode command to add 'code' alias and update description
	if len(vscodeCommands) > 0 {
		vscodeCommands[0].Aliases = append(vscodeCommands[0].Aliases, "code")
		vscodeCommands[0].Description = `Configure VSCode to use JFrog Artifactory for extensions.

The service URL should be in the format:
https://<artifactory-url>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery

Examples:
  jf vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery
  jf code https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery

This command will:
- Modify the VSCode product.json file to change the extensions gallery URL
- Create an automatic backup before making changes
- Require VSCode to be restarted to apply changes

Optional: Provide server configuration flags (--url, --user, --password, --access-token, or --server-id) 
to enable repository validation. Without these flags, the command will only modify the local VSCode configuration.

Note: On macOS/Linux, you may need to run with sudo for system-installed VSCode.`
	}

	// Modify JetBrains command to add 'jb' alias and update description
	if len(jetbrainsCommands) > 0 {
		jetbrainsCommands[0].Aliases = append(jetbrainsCommands[0].Aliases, "jb")
		jetbrainsCommands[0].Description = `Configure JetBrains IDEs to use JFrog Artifactory for plugins.

The repository URL should be in the format:
https://<artifactory-url>/artifactory/api/jetbrainsplugins/<repo-key>

Examples:
  jf jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins
  jf jb https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins

This command will:
- Detect all installed JetBrains IDEs
- Modify each IDE's idea.properties file to add the plugins repository URL
- Create automatic backups before making changes
- Require IDEs to be restarted to apply changes

Optional: Provide server configuration flags (--url, --user, --password, --access-token, or --server-id) 
to enable repository validation. Without these flags, the command will only modify the local IDE configuration.

Supported IDEs: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua`
	}

	// Return both modified commands
	return append(vscodeCommands, jetbrainsCommands...)
}
