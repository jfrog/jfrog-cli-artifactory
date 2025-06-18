package model

type VerificationResponse struct {
	Checksum                     string                        `json:"checksum"`
	EvidencesVerificationResults *[]EvidenceVerificationResult `json:"evidences_verification_results"`
	OverallVerificationStatus    VerificationStatus            `json:"overall_verification_status"`
}

type EvidenceVerificationResult struct {
	ChecksumVerificationStatus   string `json:"checksum_verification_status" col-name:"Checksum status"`
	Checksum                     string `json:"checksum" col-name:"Checksum" extended:"true"`
	SignaturesVerificationStatus string `json:"signatures_verified" col-name:"Signatures status"`
	EvidenceType                 string `json:"evidence_type" col-name:"Evidence Type"`
	Category                     string `json:"category" col-name:"Category"`
	CreatedBy                    string `json:"created_by" col-name:"Created By"`
	Time                         string `json:"time" col-name:"Time"`
}

type VerificationStatus string

const (
	Success = "Success"
	Failed  = "Failed"
)
