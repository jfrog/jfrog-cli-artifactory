package main

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli"
	"github.com/jfrog/jfrog-cli-core/v2/plugins"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func main() {
	plugins.PluginMain(GetJfrogCliArtifactoryApp())
}

func GetJfrogCliArtifactoryApp() components.App {
	app := components.CreateEmbeddedApp(
		"artifactory",
		[]components.Command{},
		components.Namespace{
			Name:        "evd",
			Description: "Evidence commands.",
			Commands:    cli.GetCommands(),
		},
	)
	return app
}
