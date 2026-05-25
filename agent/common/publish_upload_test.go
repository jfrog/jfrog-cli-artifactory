package common

import (
	"testing"

	rtServicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePublishUploadSummary(t *testing.T) {
	t.Run("nil summary", func(t *testing.T) {
		err := validatePublishUploadSummary(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "0 files transferred")
	})

	t.Run("zero succeeded", func(t *testing.T) {
		err := validatePublishUploadSummary(&rtServicesUtils.OperationSummary{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected at least 1")
	})

	t.Run("failed upload", func(t *testing.T) {
		err := validatePublishUploadSummary(&rtServicesUtils.OperationSummary{TotalFailed: 1})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed file")
	})

	t.Run("success", func(t *testing.T) {
		err := validatePublishUploadSummary(&rtServicesUtils.OperationSummary{TotalSucceeded: 1})
		assert.NoError(t, err)
	})
}

func TestValidatePublishUploadCounts(t *testing.T) {
	assert.NoError(t, validatePublishUploadCounts(1, 0))
	require.Error(t, validatePublishUploadCounts(0, 0))
	require.Error(t, validatePublishUploadCounts(1, 1))
}
