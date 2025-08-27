package reports

import (
	"fmt"

	"github.com/gookit/color"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var PlaintextReportPrinter = &plaintextReportPrinter{
	success: color.Green.Render("success"),
	failed:  color.Red.Render("failed"),
}

type plaintextReportPrinter struct {
	success string
	failed  string
}

func (p *plaintextReportPrinter) Print(result *model.VerificationResponse) error {
	err := verifyNotEmptyResponse(result)
	if err != nil {
		return err
	}
	fmt.Printf("Subject sha256:        %s\n", result.Subject.Sha256)
	fmt.Printf("Subject:               %s\n", result.Subject.Path)
	evidenceNumber := len(*result.EvidenceVerifications)
	fmt.Printf("Loaded %d evidence\n", evidenceNumber)
	successfulVerifications := 0
	for _, v := range *result.EvidenceVerifications {
		if IsVerificationSucceed(v) {
			successfulVerifications++
		}
	}
	fmt.Println()
	verificationStatusMessage := fmt.Sprintf("Verification passed for %d out of %d evidence", successfulVerifications, evidenceNumber)
	switch {
	case successfulVerifications == 0:
		fmt.Println(color.Red.Render(verificationStatusMessage))
	case successfulVerifications != evidenceNumber:
		fmt.Println(color.Yellow.Render(verificationStatusMessage))
	default:
		fmt.Println(color.Green.Render(verificationStatusMessage))
	}
	fmt.Println()
	for i, verification := range *result.EvidenceVerifications {
		p.printVerificationResult(&verification, i)
	}
	if result.OverallVerificationStatus == model.Failed {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError}
	}
	return nil
}

func (p *plaintextReportPrinter) printVerificationResult(verification *model.EvidenceVerification, index int) {
	fmt.Printf("- Evidence %d:\n", index+1)
	fmt.Printf("    - Media type:                     %s\n", verification.MediaType)
	fmt.Printf("    - Predicate type:                 %s\n", verification.PredicateType)
	fmt.Printf("    - Evidence subject sha256:        %s\n", verification.SubjectChecksum)
	if verification.VerificationResult.KeySource != "" {
		fmt.Printf("    - Key source:                     %s\n", verification.VerificationResult.KeySource)
	}
	if verification.VerificationResult.KeyFingerprint != "" {
		fmt.Printf("    - Key fingerprint:                %s\n", verification.VerificationResult.KeyFingerprint)
	}
	fmt.Printf("    - Sha256 verification status:     %s\n", p.getColoredStatus(verification.VerificationResult.Sha256VerificationStatus))
	if verification.MediaType == model.SimpleDSSE {
		fmt.Printf("    - Signatures verification status: %s\n", p.getColoredStatus(verification.VerificationResult.SignaturesVerificationStatus))
	}
	if verification.MediaType == model.SigstoreBundle {
		fmt.Printf("    - Sigstore verification status:   %s\n", p.getColoredStatus(verification.VerificationResult.SigstoreBundleVerificationStatus))
	}
	if verification.VerificationResult.FailureReason != "" {
		fmt.Printf("    - Failure reason:                 %s\n", verification.VerificationResult.FailureReason)
	}
}

func (p *plaintextReportPrinter) getColoredStatus(status model.VerificationStatus) string {
	switch status {
	case model.Success:
		return p.success
	default:
		return p.failed
	}
}
