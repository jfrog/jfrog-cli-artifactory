package main

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidencecli"
	"github.com/jfrog/jfrog-cli-core/v2/plugins"
)

func main() {
	plugins.PluginMain(evidencecli.GetJfrogCliArtifactoryApp())
}
