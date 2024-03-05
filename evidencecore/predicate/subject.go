package predicate

type Predicate struct {
	Id          int64
	EvidenceId  int64
	Repository  string
	Path        string
	Sha256      string
	Name        string
	Version     string
	PackageType string
	LinkTypeId  int16
}
