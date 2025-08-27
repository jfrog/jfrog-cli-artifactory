package reports

import (
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
)

var MarkdownReportPrinter = &markdownReportPrinter{}

type markdownReportPrinter struct {
}

func (p *markdownReportPrinter) Print(result *model.VerificationResponse) error {
	err := verifyNotEmptyResponse(result)
	if err != nil {
		return err
	}

	fmt.Println("# Evidence Verification Result")
	fmt.Println()
	fmt.Printf("**Subject:** %s  \n", result.Subject.Path)
	fmt.Printf("**Subject sha256:** %s  \n", result.Subject.Sha256)

	successfulVerifications := 0
	failedVerifications := 0
	fmt.Println("## Quick Summary")
	fmt.Println("| Predicate type | Verification status |")
	fmt.Println("|-|-|")
	for _, verification := range *result.EvidenceVerifications {
		var verificationStatus string
		if IsVerificationSucceed(verification) {
			verificationStatus = "success"
			successfulVerifications++
		} else {
			verificationStatus = "failed"
			failedVerifications++
		}
		fmt.Printf("| %s | %s |\n", verification.PredicateType, verificationStatus)
	}

	fmt.Printf("**Total loaded evidence:** %d  \n", len(*result.EvidenceVerifications))
	fmt.Printf("**Successful verifications:** %d  \n", successfulVerifications)
	fmt.Printf("**Failed verifications:** %d  \n", failedVerifications)
	fmt.Printf("**Overall verification status:** %s  \n", strings.ToLower(string(result.OverallVerificationStatus)))

	fmt.Println()
	fmt.Println("## Full Results")
	fmt.Println("| Predicate type | Subject Path | Subject Digest | Media type | Key source | Key fingerprint | Verification status | Failure reason |")
	fmt.Println("|-|-|-|-|-|-|-|-|")
	for _, verification := range *result.EvidenceVerifications {
		verificationStatus := "failed"
		if IsVerificationSucceed(verification) {
			verificationStatus = "success"
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
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			verification.PredicateType,
			result.Subject.Path,
			result.Subject.Sha256,
			verification.MediaType,
			keySource,
			keyFingerprint,
			verificationStatus,
			failureReason)
	}
	if result.OverallVerificationStatus == model.Failed {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError}
	}

	fmt.Println()
	return nil
}
