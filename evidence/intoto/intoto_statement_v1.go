package intoto

import (
	"encoding/json"
	"time"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
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
	Markdown      string               `json:"markdown,omitempty"`
	Stage         string               `json:"stage,omitempty"`
}

type ResourceDescriptor struct {
	Digest Digest `json:"digest"`
}

type Digest struct {
	Sha256 string `json:"sha256"`
}

func NewStatement(predicate []byte, predicateType, user string) *Statement {
	return &Statement{
		Type:          StatementType,
		PredicateType: predicateType,
		Predicate:     predicate,
		CreatedAt:     time.Now().UTC().Format(timeLayout),
		CreatedBy:     user,
	}
}

func (s *Statement) SetSubject(subjectSha256 string) error {
	s.Subject = make([]ResourceDescriptor, 1)
	s.Subject[0].Digest.Sha256 = subjectSha256
	return nil
}

func (s *Statement) SetMarkdown(markdown []byte) {
	s.Markdown = string(markdown)
}

func (s *Statement) SetStage(stage string) {
	s.Stage = stage
}

func (s *Statement) Marshal() ([]byte, error) {
	intotoJson, err := json.Marshal(s)
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return intotoJson, nil
}
