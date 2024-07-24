package cli

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetJfrogCliArtifactoryApp() components.App {
	app := components.CreateEmbeddedApp(
		"artifactory",
		[]components.Command{},
		components.Namespace{
			Name:        "evd",
			Description: "Evidence commands.",
			Commands:    GetCommands(),
			Category:    "Evidence",
		},
	)
	return app
}
