package reports

import (
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/stretchr/testify/assert"
)

func TestMarkdown_Print_Success(t *testing.T) {
	resp := &model.VerificationResponse{
		Subject: model.Subject{
			Sha256: "test-checksum",
			Path:   "test/path",
		},
		OverallVerificationStatus: model.Success,
		EvidenceVerifications: &[]model.EvidenceVerification{{
			MediaType:     model.SimpleDSSE,
			PredicateType: "pred-1",
			VerificationResult: model.EvidenceVerificationResult{
				SignaturesVerificationStatus: model.Success,
				Sha256VerificationStatus:     model.Success,
			},
		}, {
			MediaType:     model.SigstoreBundle,
			PredicateType: "pred-2",
			VerificationResult: model.EvidenceVerificationResult{
				SigstoreBundleVerificationStatus: model.Failed,
			},
		}},
	}

	out := captureOutput(func() {
		err := MarkdownReportPrinter.Print(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "## Evidence Verification Result Summary")
	assert.Contains(t, out, "| pred-1 | evidence.dsse | - | - | ✅ Verified | - |")
	assert.Contains(t, out, "| pred-2 | sigstore.bundle | - | - | ❌ Failed | - |")
	assert.Contains(t, out, "## Attestation Verification Full Results")
}

func TestMarkdown_Print_NilResponse(t *testing.T) {
	err := MarkdownReportPrinter.Print(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}
