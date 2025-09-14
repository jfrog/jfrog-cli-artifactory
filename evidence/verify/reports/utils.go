package reports

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
)

func verifyNotEmptyResponse(result *model.VerificationResponse) error {
	if result == nil {
		return fmt.Errorf("verification response is empty")
	}
	return nil
}

func IsVerificationSucceed(v model.EvidenceVerification) bool {
	return v.VerificationResult.Sha256VerificationStatus == model.Success &&
		(v.VerificationResult.SignaturesVerificationStatus == model.Success ||
			v.VerificationResult.SigstoreBundleVerificationStatus == model.Success)
}
