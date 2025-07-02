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
	Name              string          `json:"name"`
	Sha256            string          `json:"sha256"`
	RepositoryKey     string          `json:"repositoryKey"`
	Path              string          `json:"path"`
	PredicateType     string          `json:"predicateType"`
	PredicateCategory string          `json:"predicateCategory"`
	PredicateSlug     string          `json:"predicateSlug"`
	Predicate         string          `json:"predicate"`
	CreatedAt         string          `json:"createdAt"`
	CreatedBy         string          `json:"createdBy"`
	Verified          string          `json:"verified"`
	Subject           EvidenceSubject `json:"subject"`
	ProviderId        string          `json:"providerId"`
	SigningKey        SingingKey      `json:"signingKey"`
}

type EvidenceSubject struct {
	Sha256        string `json:"sha256"`
	RepositoryKey string `json:"repositoryKey"`
	Path          string `json:"path"`
	Name          string `json:"name"`
}

type SingingKey struct {
	Alias     string `json:"alias"`
	PublicKey string `json:"publicKey"`
}
