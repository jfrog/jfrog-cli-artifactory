package maven_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
	cmaven "github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents/maven"
	"github.com/jfrog/jfrog-client-go/xray/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrationMockClient struct {
	resp services.ComponentResolutionResponse
}

func (m *integrationMockClient) GetVersion() (string, error) {
	return healcomponents.HealComponentsMinVersion, nil
}

func (m *integrationMockClient) HealComponents(_ services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	return &m.resp, false, nil
}

func TestIntegration_MavenHealingWritesPOM(t *testing.T) {
	dir := t.TempDir()
	orig := `<?xml version="1.0"?><project><artifactId>before</artifactId></project>`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(orig), 0644))

	healed := `<?xml version="1.0"?><project><artifactId>after</artifactId></project>`
	client := &integrationMockClient{resp: services.ComponentResolutionResponse{
		Lockfile: healed,
		Changes:  []services.Change{{Package: "com.demo:lib:1.0", BeforeIntegrity: "a", AfterIntegrity: "b"}},
	}}
	_, healedFlag, err := healcomponents.RunIfEnabled(context.Background(), client, "maven-virtual",
		cmaven.NewBuildTool(), "resolve", dir, nil)
	require.NoError(t, err)
	assert.True(t, healedFlag)

	data, err := os.ReadFile(filepath.Join(dir, "pom.xml"))
	require.NoError(t, err)
	assert.Equal(t, healed, string(data))
}
