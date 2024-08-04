package intoto

import (
	"encoding/json"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory"
)

const (
	PayloadType   = "application/vnd.in-toto+json"
	StatementType = "https://in-toto.io/Statement/v1"
	timeLayout    = "2006-01-02T15:04:05.000Z"
)

type Statement struct {
	Type          string               `json:"_type"`
	Subject       []ResourceDescriptor `json:"subject"`
	PredicateType string               `json:"predicateType"`
	Predicate     json.RawMessage      `json:"predicate"`
	CreatedAt     string               `json:"createdAt"`
	CreatedBy     string               `json:"createdBy"`
}

type ResourceDescriptor struct {
	Digest Digest `json:"digest"`
}

type Digest struct {
	Sha256 string `json:"sha256"`
}

func NewStatement(predicate []byte, predicateType string, user string) *Statement {
	return &Statement{
		Type:          StatementType,
		PredicateType: predicateType,
		Predicate:     predicate,
		CreatedAt:     time.Now().UTC().Format(timeLayout),
		CreatedBy:     user,
	}
}

func (s *Statement) SetSubject(servicesManager artifactory.ArtifactoryServicesManager, subject, subjectSha256 string) error {
	s.Subject = make([]ResourceDescriptor, 1)
	res, err := servicesManager.FileInfo(subject)
	if err != nil {
		return err
	}
	if res.Checksums.Sha256 != subjectSha256 {
		return errorutils.CheckErrorf("provided sha256 does not match the file's sha256")
	}
	s.Subject[0].Digest.Sha256 = res.Checksums.Sha256
	return nil
}

func (s *Statement) Marshal() ([]byte, error) {
	intotoJson, err := json.Marshal(s)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return intotoJson, nil
}
