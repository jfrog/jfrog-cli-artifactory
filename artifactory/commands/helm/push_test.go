package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helm pushes chart version X.Y.Z+meta under the OCI tag X.Y.Z_meta, so the lookup must search the '_' folder.
func TestResolveOCIPushArtifactsBuildMetadataVersion(t *testing.T) {
	t.Run("finds the artifacts under the underscore tag", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{"mychart/1.2.3_abc1234": true}}

		repo, chartPath, results, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local", "mychart", "1.2.3+abc1234")

		require.NoError(t, err)
		assert.Equal(t, "helm-local", repo)
		assert.Equal(t, "mychart/1.2.3_abc1234", chartPath)
		assert.Contains(t, results, "manifest.json")
		for _, query := range fake.queries {
			assert.NotContains(t, query, "+")
		}
	})

	t.Run("finds the artifacts under a repo subpath", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{"team/app/mychart/1.2.3_abc1234": true}}

		repo, chartPath, _, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local/team/app", "mychart", "1.2.3+abc1234")

		require.NoError(t, err)
		assert.Equal(t, "helm-local", repo)
		assert.Equal(t, "team/app/mychart/1.2.3_abc1234", chartPath)
	})

	t.Run("not found error shows the real version", func(t *testing.T) {
		fake := &fakeSearchManager{t: t, existingPaths: map[string]bool{}}

		_, _, _, err := resolveOCIPushArtifacts(fake, "oci://acme.jfrog.io/helm-local", "mychart", "1.2.3+abc1234")

		assert.ErrorContains(t, err, "no OCI artifacts found for chart mychart:1.2.3+abc1234")
	})
}

func TestNewHelmModuleKeepsRealVersion(t *testing.T) {
	module := newHelmModule("mychart", "1.2.3+abc1234", nil)

	assert.Equal(t, "mychart:1.2.3+abc1234", module.Id)
	assert.Equal(t, "helm", string(module.Type))
}
