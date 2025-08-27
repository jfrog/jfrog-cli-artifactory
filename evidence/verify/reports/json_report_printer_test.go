package reports

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/stretchr/testify/assert"
)

func TestJsonPrinter_Success(t *testing.T) {
	resp := &model.VerificationResponse{
		Subject: model.Subject{
			Sha256: "test-checksum",
		},
		OverallVerificationStatus: model.Success,
	}

	err := JsonReportPrinter.Print(resp)
	assert.NoError(t, err)
}

func TestJsonPrinter_NilResponse(t *testing.T) {
	err := JsonReportPrinter.Print(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}
