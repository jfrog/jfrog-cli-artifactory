package intoto

import (
	"encoding/json"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"strings"

	"github.com/jfrog/jfrog-client-go/artifactory"
)

const PayloadType = "application/vnd.in-toto+json"
const StatementType = "https://in-toto.io/Statement/v0.1"

type Statement struct {
	Type          string               `json:"_type"`
	Subject       []ResourceDescriptor `json:"subject"`
	PredicateType string               `json:"predicateType"`
	Predicate     json.RawMessage      `json:"predicate"`
}

type ResourceDescriptor struct {
	Uri    string `json:"uri"`
	Digest Digest `json:"digest"`
}

type Digest struct {
	Sha256 string `json:"sha256"`
}

func NewStatement(predicate []byte, predicateType string) *Statement {
	return &Statement{
		Type:          StatementType,
		PredicateType: predicateType,
		Predicate:     predicate,
	}
}

func (s *Statement) SetSubject(servicesManager artifactory.ArtifactoryServicesManager, subjects string) error {
	subjectsSlice := strings.Split(subjects, ";")
	s.Subject = make([]ResourceDescriptor, len(subjectsSlice))
	for i, subject := range subjectsSlice {
		subjectAndSha := strings.Split(subject, "@")
		s.Subject[i].Uri = subjectAndSha[0]
		if len(subjectAndSha) > 1 {
			s.Subject[i].Digest.Sha256 = subjectAndSha[1]
		}
	}

	for i, subject := range s.Subject {
		res, err := servicesManager.FileInfo(subject.Uri)
		if err != nil {
			return err
		}
		if subject.Digest.Sha256 != "" && res.Checksums.Sha256 != subject.Digest.Sha256 {
			return errorutils.CheckErrorf("provided sha256 does not match the file's sha256")
		}
		s.Subject[i].Digest.Sha256 = res.Checksums.Sha256
	}
	return nil
}

func (s *Statement) Marshal() ([]byte, error) {
	intotoJson, err := json.Marshal(s)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return intotoJson, nil
}
