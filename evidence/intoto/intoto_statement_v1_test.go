package intoto

import (
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

type mockArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func (m *mockArtifactoryServicesManager) FileInfo(relativePath string) (*utils.FileInfo, error) {
	fi := &utils.FileInfo{
		Uri:         "dummy_uri",
		DownloadUri: "dummy_download_uri",
		Repo:        "dummy_repo",
		Path:        "dummy_path",
		RemoteUrl:   "dummy_remote_url",

		Checksums: struct {
			Sha1   string `json:"sha1,omitempty"`
			Sha256 string `json:"sha256,omitempty"`
			Md5    string `json:"md5,omitempty"`
		}{
			Sha1:   "dummy_sha1",
			Sha256: "e06f59f5a976c7f4a5406907790bb8cad6148406282f07cd143fd1de64ca169d",
			Md5:    "dummy_md5",
		},
	}
	return fi, nil
}

func TestNewStatement(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	assert.Equal(t, st.Type, StatementType)
}

func TestSetSubjectSha256NotEqual(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	aa := &mockArtifactoryServicesManager{}
	err := st.SetSubject(aa, "path/to/file.txt@e77779f5a976c7f4a5406907790bb8cad6148406282f07cd143fd1de64ca169d")
	assert.Error(t, err)
}

func TestSetSubjectSha256Equal(t *testing.T) {
	predicate := "{\n    \"vendor\": [\n        \"applitools\"\n    ],\n    \"stage\": \"QA\",\n    \"result\": \"PASSED\",\n    \"codeCoverage\": \"76%\",\n    \"passedTests\": [\n        \"(test.yml, ubuntu-latest), (test.yml, windows-latest)\"\n    ],\n    \"warnedTests\": [],\n    \"failedTests\": []\n}\n"
	predicateType := "https://in-toto.io/attestation/vulns"
	st := NewStatement([]byte(predicate), predicateType, "")
	assert.NotNil(t, st)
	aa := &mockArtifactoryServicesManager{}
	err := st.SetSubject(aa, "path/to/file.txt@e06f59f5a976c7f4a5406907790bb8cad6148406282f07cd143fd1de64ca169d")
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
