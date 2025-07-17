package model

import "github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"

type VerificationResponse struct {
	SubjectPath               string                  `json:"subjectPath"`
	SubjectChecksum           string                  `json:"subjectDigest"`
	EvidenceVerifications     *[]EvidenceVerification `json:"evidenceVerifications"`
	OverallVerificationStatus VerificationStatus      `json:"overallVerificationStatus"`
}

type EvidenceVerification struct {
	DsseEnvelope       dsse.Envelope              `json:"dsseEnvelope"`
	EvidencePath       string                     `json:"evidencePath"`
	SubjectChecksum    string                     `json:"evidenceSubjectDigest"`
	PredicateType      string                     `json:"predicateType"`
	CreatedBy          string                     `json:"createdBy"`
	Time               string                     `json:"time"`
	VerificationResult EvidenceVerificationResult `json:"verificationResult"`
}

type EvidenceVerificationResult struct {
	ChecksumVerificationStatus   VerificationStatus `json:"digestVerificationStatus"`
	SignaturesVerificationStatus VerificationStatus `json:"signaturesVerified"`
	KeySource                    string             `json:"keySource,omitempty"`
	KeyFingerprint               string             `json:"keyFingerprint,omitempty"`
}

type VerificationStatus string

const (
	Success = "success"
	Failed  = "failed"
)
