package verifiers

import (
	"fmt"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

type EvidenceVerifierInterface interface {
	Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error)
}

type evidenceVerifier struct {
	keys               []string
	useArtifactoryKeys bool
	artifactoryClient  artifactory.ArtifactoryServicesManager
	parser             evidenceParserInterface
	dsseVerifier       dsseVerifierInterface
	sigstoreVerifier   sigstoreVerifierInterface
}

func NewEvidenceVerifier(keys []string, useArtifactoryKeys bool, client *artifactory.ArtifactoryServicesManager) EvidenceVerifierInterface {
	return &evidenceVerifier{
		keys:               keys,
		artifactoryClient:  *client,
		useArtifactoryKeys: useArtifactoryKeys,
		parser:             newEvidenceParser(client),
		dsseVerifier:       newDsseVerifier(keys, useArtifactoryKeys),
		sigstoreVerifier:   newSigstoreVerifier(),
	}
}

func (v *evidenceVerifier) Verify(subjectSha256 string, evidenceMetadata *[]model.SearchEvidenceEdge, subjectPath string) (*model.VerificationResponse, error) {
	if evidenceMetadata == nil || len(*evidenceMetadata) == 0 {
		return nil, fmt.Errorf("no evidence metadata provided")
	}
	verificationResponse := &model.VerificationResponse{
		SchemaVersion: model.SchemaVersion,
		Subject: model.Subject{
			Path:   subjectPath,
			Sha256: subjectSha256,
		},
		OverallVerificationStatus: model.Success,
	}
	evidenceVerifications := make([]model.EvidenceVerification, 0, len(*evidenceMetadata))
	for i := range *evidenceMetadata {
		evidence := &(*evidenceMetadata)[i]
		verification, err := v.verifyEvidence(evidence, subjectSha256)
		if err != nil {
			return nil, err
		}
		evidenceVerifications = append(evidenceVerifications, *verification)
		if shouldFailOverall(verification) {
			verificationResponse.OverallVerificationStatus = model.Failed
		}
	}
	verificationResponse.EvidenceVerifications = &evidenceVerifications
	return verificationResponse, nil
}

func (v *evidenceVerifier) verifyEvidence(evidence *model.SearchEvidenceEdge, subjectSha256 string) (*model.EvidenceVerification, error) {
	if evidence == nil {
		return nil, fmt.Errorf("nil evidence provided")
	}
	evidenceVerification := &model.EvidenceVerification{
		DownloadPath:       evidence.Node.DownloadPath,
		SubjectChecksum:    evidence.Node.Subject.Sha256,
		PredicateType:      evidence.Node.PredicateType,
		CreatedBy:          evidence.Node.CreatedBy,
		CreatedAt:          evidence.Node.CreatedAt,
		VerificationResult: model.EvidenceVerificationResult{},
	}
	if err := v.parser.parseEvidence(evidence, evidenceVerification); err != nil {
		return nil, fmt.Errorf("failed to read envelope: %w", err)
	}
	if err := v.performVerification(evidence, evidenceVerification, subjectSha256); err != nil {
		return nil, err
	}
	return evidenceVerification, nil
}

func (v *evidenceVerifier) performVerification(evidence *model.SearchEvidenceEdge, result *model.EvidenceVerification, subjectSha256 string) error {
	switch result.MediaType {
	case model.SigstoreBundle:
		return v.sigstoreVerifier.verify(subjectSha256, result)
	case model.SimpleDSSE:
		return v.dsseVerifier.verify(subjectSha256, evidence, result)
	default:
		return fmt.Errorf("unsupported verification mode: %v", result.MediaType)
	}
}

func shouldFailOverall(verification *model.EvidenceVerification) bool {
	return verification.VerificationResult.SignaturesVerificationStatus == model.Failed ||
		verification.VerificationResult.Sha256VerificationStatus == model.Failed ||
		verification.VerificationResult.SigstoreBundleVerificationStatus == model.Failed
}
