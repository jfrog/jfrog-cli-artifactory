package model

type CreateResponse struct {
	Repository        string `json:"repository"`
	Path              string `json:"path"`
	Name              string `json:"name"`
	Uri               string `json:"uri"`
	Sha256            string `json:"sha256"`
	PredicateCategory string `json:"predicate_category"`
	PredicateType     string `json:"predicate_type"`
	PredicateSlug     string `json:"predicate_slug"`
	CreatedAt         string `json:"created_at"`
	CreatedBy         string `json:"created_by"`
	Verified          bool   `json:"verified"`
	ProviderId        string `json:"provider_id"`
}
