package evidenceproviders

type EvidenceProvider interface {
	GetEvidence() ([]byte, error)
}
