package container

import (
	"testing"

	servicesutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

func TestGetLayerSha256(t *testing.T) {
	// Layers searched with properties.
	assert.Equal(t, "from-property", getLayerSha256(servicesutils.ResultItem{
		Sha256:     "from-field",
		Properties: []servicesutils.Property{{Key: "docker.manifest", Value: "1.0"}, {Key: "sha256", Value: "from-property"}},
	}))
	// Layers searched without properties, such as the platform manifests layers of a multi-platform image.
	assert.Equal(t, "from-field", getLayerSha256(servicesutils.ResultItem{Sha256: "from-field"}))
	assert.Empty(t, getLayerSha256(servicesutils.ResultItem{}))
}
