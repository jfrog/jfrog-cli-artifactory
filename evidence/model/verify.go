package model

import "github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"

const SchemaVersion = "1.0"

type VerificationResponse struct {
	// Update the schemaVersion value when this structure is updated.
	SchemaVersion             string                  `json:"schemaVersion"`
	Subject                   Subject                 `json:"subject"`
	EvidenceVerifications     *[]EvidenceVerification `json:"evidenceVerifications"`
	OverallVerificationStatus VerificationStatus      `json:"overallVerificationStatus"`
}

type Subject struct {
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
}

type EvidenceVerification struct {
	DsseEnvelope       dsse.Envelope              `json:"dsseEnvelope"`
	DownloadPath       string                     `json:"downloadPath"`
	SubjectChecksum    string                     `json:"evidenceSubjectSha256"`
	PredicateType      string                     `json:"predicateType"`
	CreatedBy          string                     `json:"createdBy"`
	CreatedAt          string                     `json:"createdAt"`
	VerificationResult EvidenceVerificationResult `json:"verificationResult"`
}

type EvidenceVerificationResult struct {
	Sha256VerificationStatus     VerificationStatus `json:"sha256VerificationStatus"`
	SignaturesVerificationStatus VerificationStatus `json:"signaturesVerificationStatus"`
	KeySource                    string             `json:"keySource,omitempty"`
	KeyFingerprint               string             `json:"keyFingerprint,omitempty"`
}

type VerificationStatus string

const (
	Success = "success"
	Failed  = "failed"
)
