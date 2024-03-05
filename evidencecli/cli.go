package evidencecli

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func GetJfrogCliArtifactoryApp() components.App {
	app := components.CreateEmbeddedApp(
		"evidence",
		getCommands(),
	)
	return app
}
