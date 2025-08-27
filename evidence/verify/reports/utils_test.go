package reports

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/stretchr/testify/assert"
)

func TestVerifyNotEmptyResponse(t *testing.T) {
	tests := []struct {
		name          string
		result        *model.VerificationResponse
		expectedError bool
		errorMessage  string
	}{
		{
			name:          "NilResponse",
			result:        nil,
			expectedError: true,
			errorMessage:  "verification response is empty",
		},
		{
			name: "ValidResponse",
			result: &model.VerificationResponse{
				SchemaVersion:             model.SchemaVersion,
				Subject:                   model.Subject{Path: "test/path", Sha256: "test-sha256"},
				EvidenceVerifications:     &[]model.EvidenceVerification{},
				OverallVerificationStatus: model.Success,
			},
			expectedError: false,
		},
		{
			name: "EmptyResponse",
			result: &model.VerificationResponse{
				SchemaVersion:             "",
				Subject:                   model.Subject{},
				EvidenceVerifications:     nil,
				OverallVerificationStatus: "",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyNotEmptyResponse(tt.result)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
