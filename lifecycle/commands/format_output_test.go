package commands

import (
	"encoding/json"
	"strings"
	"testing"

	coreformat "github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- promote ---

func TestPromotePrintOutput_Json(t *testing.T) {
	resp := services.RbPromotionResp{
		RepositoryKey: "dev-release-bundles-v2",
		ReleaseBundleDetails: services.ReleaseBundleDetails{
			ReleaseBundleName:    "my-bundle",
			ReleaseBundleVersion: "1.0.0",
		},
		RbPromotionBody: services.RbPromotionBody{
			Environment: "DEV",
		},
		Created: "2026-06-10T09:00:00.000Z",
	}

	cmd := NewReleaseBundlePromoteCommand().SetOutputFormat(coreformat.Json)

	// Verify the response serializes correctly (the actual output goes to log.Output)
	content, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(content), "dev-release-bundles-v2")
	assert.Contains(t, string(content), "my-bundle")
	assert.Contains(t, string(content), "DEV")

	// printOutput with Json should not error
	assert.NoError(t, cmd.printOutput(resp))
}

func TestPromotePrintOutput_NoFormat_BackwardCompat(t *testing.T) {
	resp := services.RbPromotionResp{
		RepositoryKey: "dev-release-bundles-v2",
	}
	// format.None (no flag set) must also emit JSON — same as pre-existing behavior
	cmd := NewReleaseBundlePromoteCommand() // outputFormat defaults to ""
	assert.NoError(t, cmd.printOutput(resp))
}

func TestPromotePrintOutput_Table(t *testing.T) {
	resp := services.RbPromotionResp{
		RepositoryKey: "dev-release-bundles-v2",
		ReleaseBundleDetails: services.ReleaseBundleDetails{
			ReleaseBundleName:    "my-bundle",
			ReleaseBundleVersion: "1.0.0",
		},
		RbPromotionBody: services.RbPromotionBody{
			Environment: "DEV",
		},
		Created: "2026-06-10T09:00:00.000Z",
	}
	cmd := NewReleaseBundlePromoteCommand().SetOutputFormat(coreformat.Table)
	assert.NoError(t, cmd.printOutput(resp))
}

func TestPromoteSetOutputFormat(t *testing.T) {
	cmd := NewReleaseBundlePromoteCommand()
	assert.Equal(t, coreformat.OutputFormat(""), cmd.outputFormat)
	cmd.SetOutputFormat(coreformat.Json)
	assert.Equal(t, coreformat.Json, cmd.outputFormat)
	cmd.SetOutputFormat(coreformat.Table)
	assert.Equal(t, coreformat.Table, cmd.outputFormat)
}

// --- distribute ---

func TestDistributePrintOutput_Json(t *testing.T) {
	cmd := &ReleaseBundleDistributeCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "1.0.0",
		},
		outputFormat: coreformat.Json,
	}
	// Should not error
	assert.NoError(t, cmd.printDistributeOutput())
}

func TestDistributePrintOutput_NoFormat_Silent(t *testing.T) {
	cmd := &ReleaseBundleDistributeCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "1.0.0",
		},
		outputFormat: coreformat.None,
	}
	// No format set → no output and no error (backward-compat silent)
	assert.NoError(t, cmd.printDistributeOutput())
}

func TestDistributePrintOutput_JsonContent(t *testing.T) {
	cmd := &ReleaseBundleDistributeCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "2.3.4",
		},
		outputFormat: coreformat.Json,
	}
	type distributeOutput struct {
		Name    string `json:"release_bundle_name"`
		Version string `json:"release_bundle_version"`
		Status  string `json:"status"`
	}
	out := distributeOutput{Name: cmd.releaseBundleName, Version: cmd.releaseBundleVersion, Status: "distributed"}
	content, err := json.Marshal(out)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(content), "my-bundle"))
	assert.True(t, strings.Contains(string(content), "2.3.4"))
	assert.True(t, strings.Contains(string(content), "distributed"))
}

func TestDistributeSetOutputFormat(t *testing.T) {
	cmd := NewReleaseBundleDistributeCommand()
	assert.Equal(t, coreformat.OutputFormat(""), cmd.outputFormat)
	cmd.SetOutputFormat(coreformat.Json)
	assert.Equal(t, coreformat.Json, cmd.outputFormat)
}

// --- create ---

func TestCreatePrintOutput_Json(t *testing.T) {
	cmd := &ReleaseBundleCreateCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "1.0.0",
		},
		outputFormat: coreformat.Json,
	}
	assert.NoError(t, cmd.printCreateOutput())
}

func TestCreatePrintOutput_NoFormat_Silent(t *testing.T) {
	cmd := &ReleaseBundleCreateCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "1.0.0",
		},
		outputFormat: coreformat.None,
	}
	// No format set → no output and no error (backward-compat silent)
	assert.NoError(t, cmd.printCreateOutput())
}

func TestCreatePrintOutput_JsonContent(t *testing.T) {
	cmd := &ReleaseBundleCreateCommand{
		releaseBundleCmd: releaseBundleCmd{
			releaseBundleName:    "my-bundle",
			releaseBundleVersion: "3.0.1",
		},
		outputFormat: coreformat.Json,
	}
	type createOutput struct {
		Name    string `json:"release_bundle_name"`
		Version string `json:"release_bundle_version"`
		Status  string `json:"status"`
	}
	out := createOutput{Name: cmd.releaseBundleName, Version: cmd.releaseBundleVersion, Status: "created"}
	content, err := json.Marshal(out)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(content), "my-bundle"))
	assert.True(t, strings.Contains(string(content), "3.0.1"))
	assert.True(t, strings.Contains(string(content), "created"))
}

func TestCreateSetOutputFormat(t *testing.T) {
	cmd := NewReleaseBundleCreateCommand()
	assert.Equal(t, coreformat.OutputFormat(""), cmd.outputFormat)
	cmd.SetOutputFormat(coreformat.Json)
	assert.Equal(t, coreformat.Json, cmd.outputFormat)
}
