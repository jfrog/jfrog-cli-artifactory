package model

import (
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const SchemaVersion = "1.1"

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
	MediaType          MediaType                  `json:"mediaType"`
	DownloadPath       string                     `json:"downloadPath"`
	SubjectChecksum    string                     `json:"evidenceSubjectSha256"`
	PredicateType      string                     `json:"predicateType"`
	CreatedBy          string                     `json:"createdBy"`
	CreatedAt          string                     `json:"createdAt"`
	VerificationResult EvidenceVerificationResult `json:"verificationResult"`
	DsseEnvelope       *dsse.Envelope             `json:"dsseEnvelope,omitempty"`
	SigstoreBundle     *bundle.Bundle             `json:"sigstoreBundle,omitempty"`
}

type EvidenceVerificationResult struct {
	Sha256VerificationStatus         VerificationStatus         `json:"sha256VerificationStatus,omitempty"`
	SignaturesVerificationStatus     VerificationStatus         `json:"signaturesVerificationStatus,omitempty"`
	SigstoreBundleVerificationStatus VerificationStatus         `json:"sigstoreBundleVerificationStatus,omitempty"`
	KeySource                        string                     `json:"keySource,omitempty"`
	KeyFingerprint                   string                     `json:"keyFingerprint,omitempty"`
	SigstoreBundleVerificationResult *verify.VerificationResult `json:"sigstoreBundleVerificationResult,omitempty"`
	FailureReason                    string                     `json:"failureReason,omitempty"`
}

type VerificationStatus string

const (
	Success = "success"
	Failed  = "failed"
)

type MediaType string

const (
	SigstoreBundle MediaType = "sigstore.bundle"
	SimpleDSSE     MediaType = "evidence.dsse"
)
