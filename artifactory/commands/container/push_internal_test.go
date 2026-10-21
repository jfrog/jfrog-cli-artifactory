package container

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	servicesutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLayerSha256(t *testing.T) {
	testCases := []struct {
		name     string
		layer    servicesutils.ResultItem
		expected string
	}{
		{
			name: "searched with properties",
			layer: servicesutils.ResultItem{
				Sha256:     "from-field",
				Properties: []servicesutils.Property{{Key: "docker.manifest", Value: "1.0"}, {Key: "sha256", Value: "from-property"}},
			},
			expected: "from-property",
		},
		{
			name:     "searched without properties, such as the layers of a multi-platform image",
			layer:    servicesutils.ResultItem{Sha256: "from-field"},
			expected: "from-field",
		},
		{
			name: "properties without sha256",
			layer: servicesutils.ResultItem{
				Sha256:     "from-field",
				Properties: []servicesutils.Property{{Key: "docker.manifest", Value: "1.0"}},
			},
			expected: "from-field",
		},
		{
			name: "sha256 property with an empty value",
			layer: servicesutils.ResultItem{
				Sha256:     "from-field",
				Properties: []servicesutils.Property{{Key: "sha256", Value: ""}},
			},
			expected: "from-field",
		},
		{
			name:     "no sha256 at all",
			layer:    servicesutils.ResultItem{},
			expected: "",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.expected, getLayerSha256(testCase.layer))
		})
	}
}

func TestLayersMapToFileTransferDetails(t *testing.T) {
	layers := []servicesutils.ResultItem{
		{Repo: "docker-local", Path: "img/sha256:aaa", Name: "manifest.json", Sha256: "from-field"},
		{
			Repo: "docker-local", Path: "img/1", Name: "list.manifest.json", Sha256: "ignored",
			Properties: []servicesutils.Property{{Key: "sha256", Value: "from-property"}},
		},
	}
	pc := NewPushCommand(ocicontainer.DockerClient)
	require.NoError(t, pc.layersMapToFileTransferDetails("https://acme.jfrog.io/artifactory/", &layers))

	result := pc.Result()
	require.NotNil(t, result)
	assert.Equal(t, len(layers), result.SuccessCount())
	reader := result.Reader()
	defer func() { assert.NoError(t, reader.Close()) }()
	var details []clientutils.FileTransferDetails
	for detail := new(clientutils.FileTransferDetails); reader.NextRecord(detail) == nil; detail = new(clientutils.FileTransferDetails) {
		details = append(details, *detail)
	}
	require.NoError(t, reader.GetError())
	assert.Equal(t, []clientutils.FileTransferDetails{
		{TargetPath: "docker-local/img/sha256:aaa/manifest.json", RtUrl: "https://acme.jfrog.io/artifactory/", Sha256: "from-field"},
		{TargetPath: "docker-local/img/1/list.manifest.json", RtUrl: "https://acme.jfrog.io/artifactory/", Sha256: "from-property"},
	}, details)
}
