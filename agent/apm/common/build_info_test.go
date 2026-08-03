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

func TestLocalPackedArtifactChecksum(t *testing.T) {
	t.Run("hashes the deterministically-named local zip", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "my-package-1.2.3.zip")
		require.NoError(t, os.WriteFile(zipPath, []byte("fake zip contents"), 0o644))

		checksum, err := localPackedArtifactChecksum(tempDir, "my-package", "1.2.3")
		require.NoError(t, err)
		// "fake zip contents" sha256, computed independently (shasum -a 256) to catch any
		// hashing regression, not just "some non-empty string came back".
		assert.Equal(t, "58b184a82c063327f97c38ed97f21acbfb8d4bc50d52b4070b9aed8c06b4bc73", checksum.Sha256)
		assert.NotEmpty(t, checksum.Sha1)
		assert.NotEmpty(t, checksum.Md5)
	})

	t.Run("missing zip -> error, not a panic or silent empty checksum", func(t *testing.T) {
		tempDir := t.TempDir()

		_, err := localPackedArtifactChecksum(tempDir, "never-published", "9.9.9")
		require.Error(t, err)
	})
}

func TestCollectAndSavePublishBuildInfo_FallsBackToLocalZipWhenHeadUnavailable(t *testing.T) {
	testutil.WithJfrogHome(t)
	tempDir := t.TempDir()

	manifestPath := filepath.Join(tempDir, ApmManifestName)
	require.NoError(t, os.WriteFile(manifestPath, []byte("name: my-package\nversion: 1.2.3\n"), 0o644))
	// The zip apm would have packed for this publish, left in the working directory exactly as
	// the real apm CLI leaves it after a successful publish.
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "my-package-1.2.3.zip"), []byte("fake zip contents"), 0o644))

	buildConfig := new(buildUtils.BuildConfiguration)
	require.NoError(t, buildConfig.SetBuildName("test-build").SetBuildNumber("1").ValidateBuildAndModuleParams())

	// serverDetails=nil makes lookupPublishedArtifactChecksum return an empty checksum
	// immediately (no network call attempted) - the fallback local-zip hash must fire, and the
	// overall call must still succeed rather than record an artifact with no checksum at all.
	err := CollectAndSavePublishBuildInfo(manifestPath, "acme", "acme-repo", nil, buildConfig)
	require.NoError(t, err)
}
