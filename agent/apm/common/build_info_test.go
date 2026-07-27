package apmcommon

import (
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/agent/common/testutil"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
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
