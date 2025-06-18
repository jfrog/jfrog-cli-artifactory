package model

type ResponseSearchEvidence struct {
	Data EvidenceData `json:"data"`
}

type EvidenceData struct {
	Evidence Evidence `json:"evidence"`
}

type Evidence struct {
	SearchEvidence SearchEvidence `json:"searchEvidence"`
}

type SearchEvidence struct {
	Edges []SearchEvidenceEdge `json:"edges"`
}

type SearchEvidenceEdge struct {
	Node EvidenceMetadata `json:"node"`
}

type EvidenceMetadata struct {
	DownloadPath      string          `json:"downloadPath"`
	PredicateType     string          `json:"predicateType"`
	PredicateCategory string          `json:"predicateCategory"`
	CreatedAt         string          `json:"createdAt"`
	CreatedBy         string          `json:"createdBy"`
	Subject           EvidenceSubject `json:"subject"`
}

type EvidenceSubject struct {
	Sha256 string `json:"sha256"`
}
