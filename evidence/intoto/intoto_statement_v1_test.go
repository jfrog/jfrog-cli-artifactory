package intoto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStatement(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	assert.Equal(t, st.Type, StatementType)
}

func TestSetSubjectSha256AnyValue(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	// Any sha256 should be accepted now (plain setter)
	err := st.SetSubject("e77779f5a976c7f4a5406907790bb8cad6148406282f07cd143fd1de64ca169d")
	assert.NoError(t, err)
}

func TestSetSubjectSha256Equal(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	err := st.SetSubject("e06f59f5a976c7f4a5406907790bb8cad6148406282f07cd143fd1de64ca169d")
	assert.NoError(t, err)
}

func TestMarshal(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	marsheld, err := st.Marshal()
	assert.NoError(t, err)
	assert.NotNil(t, marsheld)
}
