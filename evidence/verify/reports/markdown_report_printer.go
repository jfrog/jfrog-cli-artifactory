package reports

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
)

var MarkdownReportPrinter = &markdownReportPrinter{}

type markdownReportPrinter struct {
}

// getStatusDisplay converts a verification status to a combined icon + text representation.
// For example: Success -> "✅ Verified", Failed -> "❌ Failed".
func getStatusDisplay(status model.VerificationStatus) string {
	if status == model.Success {
		return "✅ Verified"
	}
	return "❌ Failed"
}

func (p *markdownReportPrinter) Print(result *model.VerificationResponse) error {
	err := verifyNotEmptyResponse(result)
	if err != nil {
		return err
	}

	fmt.Println("## Evidence Verification Result Summary")
	fmt.Println()
	fmt.Printf("**Subject:** %s  \n", result.Subject.Path)
	fmt.Printf("**Subject sha256:** %s  \n", result.Subject.Sha256)
	fmt.Println()
	fmt.Printf("**Overall attestation verification status:** %s  \n", getStatusDisplay(result.OverallVerificationStatus))

	fmt.Println()
	fmt.Println("## Attestation Verification Full Results")
	fmt.Println("| Predicate type | Media type | Key source | Key fingerprint | Verification status | Failure reason |")
	fmt.Println("|-|-|-|-|-|-|")
	for _, verification := range *result.EvidenceVerifications {
		var verificationStatus model.VerificationStatus = model.Failed
		if IsVerificationSucceed(verification) {
			verificationStatus = model.Success
		}
		failureReason := verification.VerificationResult.FailureReason
		if failureReason == "" {
			failureReason = "-"
		}
		keySource := verification.VerificationResult.KeySource
		if keySource == "" {
			keySource = "-"
		}
		keyFingerprint := verification.VerificationResult.KeyFingerprint
		if keyFingerprint == "" {
			keyFingerprint = "-"
		}
		fmt.Printf("| %s | %s | %s | %s | %s | %s |\n",
			verification.PredicateType,
			verification.MediaType,
			keySource,
			keyFingerprint,
			getStatusDisplay(verificationStatus),
			failureReason)
	}
	fmt.Println()

	return nil
}
