package apmcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectAndSaveInstallBuildInfo_MissingLockfileIsNotAnError(t *testing.T) {
	testutil.WithJfrogHome(t)
	tempDir := t.TempDir()

	buildConfig := new(buildUtils.BuildConfiguration)
	require.NoError(t, buildConfig.SetBuildName("test-build").SetBuildNumber("1").ValidateBuildAndModuleParams())

	// apm doesn't write apm.lock.yaml at all for a zero-dependency project ("No changes --
	// install state already up to date") - this must be treated the same as "0 dependencies
	// found", not surfaced as a collection failure.
	err := CollectAndSaveInstallBuildInfo(
		filepath.Join(tempDir, ApmLockfileName),
		filepath.Join(tempDir, ApmManifestName),
		nil,
		buildConfig,
	)
	require.NoError(t, err)
}

func TestDerivedModuleID(t *testing.T) {
	t.Run("manifest with name and version -> name:version, matching npm/yarn's convention", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)
		require.NoError(t, os.WriteFile(manifestPath, []byte("name: my-package\nversion: 1.2.3\n"), 0o644))

		assert.Equal(t, "my-package:1.2.3", derivedModuleID(manifestPath))
	})

	t.Run("no manifest file -> falls back to directory name", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)

		assert.Equal(t, filepath.Base(tempDir), derivedModuleID(manifestPath))
	})

	t.Run("manifest missing version -> falls back to directory name", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)
		require.NoError(t, os.WriteFile(manifestPath, []byte("name: my-package\n"), 0o644))

		assert.Equal(t, filepath.Base(tempDir), derivedModuleID(manifestPath))
	})

	t.Run("manifest missing name -> falls back to directory name", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)
		require.NoError(t, os.WriteFile(manifestPath, []byte("version: 1.2.3\n"), 0o644))

		assert.Equal(t, filepath.Base(tempDir), derivedModuleID(manifestPath))
	})
}
