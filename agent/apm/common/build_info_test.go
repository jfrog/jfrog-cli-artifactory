package apmcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldCollectBuildInfo(t *testing.T) {
	collect, err := ShouldCollectBuildInfo(nil)
	require.NoError(t, err)
	assert.False(t, collect, "nil build config must not enable collection")

	empty := new(buildUtils.BuildConfiguration)
	collect, err = ShouldCollectBuildInfo(empty)
	require.NoError(t, err)
	assert.False(t, collect, "no build-name/number must not enable collection")

	enabled := new(buildUtils.BuildConfiguration)
	require.NoError(t, enabled.SetBuildName("b").SetBuildNumber("1").ValidateBuildAndModuleParams())
	collect, err = ShouldCollectBuildInfo(enabled)
	require.NoError(t, err)
	assert.True(t, collect)
}

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

	t.Run("no manifest file -> empty, matching npm/yarn's BuildInfoModuleId() convention", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)

		assert.Equal(t, "", derivedModuleID(manifestPath))
	})

	t.Run("manifest missing version -> empty, matching npm/yarn's BuildInfoModuleId() convention", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)
		require.NoError(t, os.WriteFile(manifestPath, []byte("name: my-package\n"), 0o644))

		assert.Equal(t, "", derivedModuleID(manifestPath))
	})

	t.Run("manifest missing name -> empty, matching npm/yarn's BuildInfoModuleId() convention", func(t *testing.T) {
		tempDir := t.TempDir()
		manifestPath := filepath.Join(tempDir, ApmManifestName)
		require.NoError(t, os.WriteFile(manifestPath, []byte("version: 1.2.3\n"), 0o644))

		assert.Equal(t, "", derivedModuleID(manifestPath))
	})
}

func TestLocalPackedArtifactChecksum(t *testing.T) {
	t.Run("hashes the deterministically-named local zip", func(t *testing.T) {
		tempDir := t.TempDir()
		zipPath := filepath.Join(tempDir, "my-package-1.2.3.zip")
		require.NoError(t, os.WriteFile(zipPath, []byte("fake zip contents"), 0o644))

		checksum, err := localPackedArtifactChecksum(zipPath)
		require.NoError(t, err)
		// "fake zip contents" sha256, computed independently (shasum -a 256) to catch any
		// hashing regression, not just "some non-empty string came back".
		assert.Equal(t, "58b184a82c063327f97c38ed97f21acbfb8d4bc50d52b4070b9aed8c06b4bc73", checksum.Sha256)
		assert.NotEmpty(t, checksum.Sha1)
		assert.NotEmpty(t, checksum.Md5)
	})

	t.Run("missing zip -> error, not a panic or silent empty checksum", func(t *testing.T) {
		tempDir := t.TempDir()

		_, err := localPackedArtifactChecksum(filepath.Join(tempDir, "never-published-9.9.9.zip"))
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
	err := CollectAndSavePublishBuildInfo(manifestPath, "acme", "my-package", "acme-repo", "", nil, buildConfig)
	require.NoError(t, err)
}

func TestCollectAndSavePublishBuildInfo_UsesExplicitZipPath(t *testing.T) {
	testutil.WithJfrogHome(t)
	tempDir := t.TempDir()

	manifestPath := filepath.Join(tempDir, ApmManifestName)
	require.NoError(t, os.WriteFile(manifestPath, []byte("name: my-package\nversion: 1.2.3\n"), 0o644))
	// --zip lets the caller publish a pre-built archive under any name/path, unlike apm's own
	// deterministic {name}-{version}.zip auto-pack naming.
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "build"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "build", "custom.zip"), []byte("fake zip contents"), 0o644))

	buildConfig := new(buildUtils.BuildConfiguration)
	require.NoError(t, buildConfig.SetBuildName("test-build").SetBuildNumber("1").ValidateBuildAndModuleParams())

	err := CollectAndSavePublishBuildInfo(manifestPath, "acme", "my-package", "acme-repo", filepath.Join("build", "custom.zip"), nil, buildConfig)
	require.NoError(t, err)
}

func TestSavePublishBuildInfo_ArtifactPathUsesPackageNameNotManifestName(t *testing.T) {
	testutil.WithJfrogHome(t)

	buildConfig := new(buildUtils.BuildConfiguration)
	require.NoError(t, buildConfig.SetBuildName("test-build-package-name-path").SetBuildNumber("1").ValidateBuildAndModuleParams())
	// Partials for this build name/number live outside WithJfrogHome's per-test temp dir (see
	// note below), so clean them up explicitly rather than leaking state into later test runs.
	t.Cleanup(func() { _ = buildUtils.RemoveBuildDir("test-build-package-name-path", "1", "") }) // best-effort test cleanup

	// apm.yml's own name: field ("internal-name") can differ from the --package identity apm
	// actually uploads under ("published-name") - the artifact record must reflect where the
	// file really landed in Artifactory, not apm.yml's name.
	err := SavePublishBuildInfo("acme", "internal-name", "published-name", "1.0.0", entities.Checksum{Sha256: "abc"}, "acme-repo", nil, buildConfig)
	require.NoError(t, err)

	// WithJfrogHome's isolation doesn't extend to build-info's own partials directory (it reads a
	// build-name-derived path outside the per-test temp dir), so a leftover partial from an
	// earlier run of this same build name/number can still be present; check the most recent one
	// rather than requiring exactly one.
	partials, err := buildUtils.ReadPartialBuildInfoFiles("test-build-package-name-path", "1", "")
	require.NoError(t, err)
	require.NotEmpty(t, partials)
	lastPartial := partials[len(partials)-1]
	require.Len(t, lastPartial.Artifacts, 1)
	artifact := lastPartial.Artifacts[0]
	assert.Equal(t, "published-name-1.0.0.zip", artifact.Name)
	assert.Equal(t, "acme/published-name/published-name-1.0.0.zip", artifact.Path)
}

func TestAnchorRequestedByToModule(t *testing.T) {
	t.Run("direct dependency with no chain gets one anchored to the module id", func(t *testing.T) {
		got := anchorRequestedByToModule(nil, "consumer:1.0.0")
		assert.Equal(t, [][]string{{"consumer:1.0.0"}}, got)
	})

	t.Run("transitive dependency's chain gets the module id appended as its terminal element", func(t *testing.T) {
		got := anchorRequestedByToModule([][]string{{"owner/direct-dep"}}, "consumer:1.0.0")
		assert.Equal(t, [][]string{{"owner/direct-dep", "consumer:1.0.0"}}, got)
	})

	t.Run("multiple diamond-dependency paths each get anchored independently", func(t *testing.T) {
		got := anchorRequestedByToModule([][]string{{"owner/a"}, {"owner/b"}}, "consumer:1.0.0")
		assert.Equal(t, [][]string{{"owner/a", "consumer:1.0.0"}, {"owner/b", "consumer:1.0.0"}}, got)
	})
}
