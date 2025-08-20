package verify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gookit/color"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify/verifiers"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var success = color.Green.Render("success")
var failed = color.Red.Render("failed")

const searchEvidenceQueryWithPublicKey = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType createdAt createdBy subject { sha256 } signingKey {alias, publicKey} } } } } }"}`
const searchEvidenceQueryWithoutPublicKey = `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType createdAt createdBy subject { sha256 } } } } } }"}`

// verifyEvidenceBase provides shared logic for evidence verification commands.
type verifyEvidenceBase struct {
	serverDetails      *config.ServerDetails
	format             string
	keys               []string
	useArtifactoryKeys bool
	artifactoryClient  *artifactory.ArtifactoryServicesManager
	oneModelClient     onemodel.Manager
	verifier           verifiers.EvidenceVerifierInterface
}

// printVerifyResult prints the verification result in the requested format.
func (v *verifyEvidenceBase) printVerifyResult(result *model.VerificationResponse) error {
	if v.format == "json" {
		return printJson(result)
	}
	return printText(result)
}

// verifyEvidence runs the verification process for the given evidence metadata and subject sha256.
func (v *verifyEvidenceBase) verifyEvidence(client *artifactory.ArtifactoryServicesManager, evidenceMetadata *[]model.SearchEvidenceEdge, sha256, subjectPath string) error {
	if v.verifier == nil {
		v.verifier = verifiers.NewEvidenceVerifier(v.keys, v.useArtifactoryKeys, client)
	}
	verify, err := v.verifier.Verify(sha256, evidenceMetadata, subjectPath)
	if err != nil {
		return err
	}
	return v.printVerifyResult(verify)
}

// createArtifactoryClient creates an Artifactory client for evidence operations.
func (v *verifyEvidenceBase) createArtifactoryClient() (*artifactory.ArtifactoryServicesManager, error) {
	if v.artifactoryClient != nil {
		return v.artifactoryClient, nil
	}
	artifactoryClient, err := utils.CreateUploadServiceManager(v.serverDetails, 1, 0, 0, false, nil)
	if err != nil {
		return nil, err
	}
	v.artifactoryClient = &artifactoryClient
	return v.artifactoryClient, nil
}

// queryEvidenceMetadata queries evidence metadata for a given repo, path, and name.
func (v *verifyEvidenceBase) queryEvidenceMetadata(repo string, path string, name string) (*[]model.SearchEvidenceEdge, error) {
	err := createOneModelService(v)
	if err != nil {
		return nil, err
	}
	var query string
	if v.useArtifactoryKeys {
		query = fmt.Sprintf(searchEvidenceQueryWithPublicKey, repo, path, name)
	} else {
		query = fmt.Sprintf(searchEvidenceQueryWithoutPublicKey, repo, path, name)
	}
	log.Debug("Fetch evidence metadata using query:", query)
	queryByteArray := []byte(query)
	response, err := v.oneModelClient.GraphqlQuery(queryByteArray)
	if err != nil {
		errStr := err.Error()
		if isPublicKeyFieldNotFound(errStr) {
			return nil, fmt.Errorf("the evidence service version should be at least 7.125.0 and the onemodel version should be at least 1.55.0")
		}
		return nil, fmt.Errorf("error querying evidence from One-Model service: %w", err)
	}
	evidence := model.ResponseSearchEvidence{}
	err = json.Unmarshal(response, &evidence)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal evidence metadata: %w", err)
	}
	edges := evidence.Data.Evidence.SearchEvidence.Edges
	if len(edges) == 0 {
		return nil, fmt.Errorf("no evidence found for the given subject")
	}
	return &edges, nil
}

func createOneModelService(v *verifyEvidenceBase) error {
	if v.oneModelClient != nil {
		return nil
	}
	manager, err := utils.CreateOnemodelServiceManager(v.serverDetails, false)
	if err != nil {
		return err
	}
	v.oneModelClient = manager
	return nil
}

// printText prints the verification result in a human-readable format.
func printText(result *model.VerificationResponse) error {
	err := validateResponse(result)
	if err != nil {
		return err
	}
	fmt.Printf("Subject sha256:        %s\n", result.Subject.Sha256)
	fmt.Printf("Subject:               %s\n", result.Subject.Path)
	evidenceNumber := len(*result.EvidenceVerifications)
	fmt.Printf("Loaded %d evidence\n", evidenceNumber)
	successfulVerifications := 0
	for _, v := range *result.EvidenceVerifications {
		if isVerificationSucceed(v) {
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
		printVerificationResult(&verification, i)
	}
	if result.OverallVerificationStatus == model.Failed {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError}
	}
	return nil
}

func printVerificationResult(verification *model.EvidenceVerification, index int) {
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
	if verification.MediaType == model.SimpleDSSE {
		fmt.Printf("    - Sha256 verification status:     %s\n", getColoredStatus(verification.VerificationResult.Sha256VerificationStatus))
		fmt.Printf("    - Signatures verification status: %s\n", getColoredStatus(verification.VerificationResult.SignaturesVerificationStatus))
	}
	if verification.MediaType == model.SigstoreBundle {
		fmt.Printf("    - Sigstore verification status:   %s\n", getColoredStatus(verification.VerificationResult.SigstoreBundleVerificationStatus))
	}
	if verification.VerificationResult.FailureReason != "" {
		fmt.Printf("    - Failure reason:                 %s\n", verification.VerificationResult.FailureReason)
	}
}

func validateResponse(result *model.VerificationResponse) error {
	if result == nil {
		return fmt.Errorf("verification response is empty")
	}
	return nil
}

// printJson prints the verification result in JSON format.
func printJson(result *model.VerificationResponse) error {
	err := validateResponse(result)
	if err != nil {
		return err
	}
	resultJson, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(resultJson))
	if result.OverallVerificationStatus == model.Failed {
		return coreutils.CliError{ExitCode: coreutils.ExitCodeError}
	}
	return nil
}

func getColoredStatus(status model.VerificationStatus) string {
	switch status {
	case model.Success:
		return success
	default:
		return failed
	}
}

func isPublicKeyFieldNotFound(errStr string) bool {
	return strings.Contains(errStr, "publicKey")
}

func isVerificationSucceed(v model.EvidenceVerification) bool {
	return v.VerificationResult.Sha256VerificationStatus == model.Success &&
		v.VerificationResult.SignaturesVerificationStatus == model.Success ||
		v.VerificationResult.SigstoreBundleVerificationStatus == model.Success
}
