package replication

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

// safeJSONDecode validates and decodes JSON data into the target struct.
// This is test-only code that validates request payloads from our own test client.
func safeJSONDecode(t *testing.T, data []byte, target interface{}) {
	t.Helper()
	// Validate input is not empty
	if len(data) == 0 {
		t.Fatal("empty content for unmarshal")
	}
	// Validate JSON syntax before decoding
	if !json.Valid(data) {
		t.Fatal("invalid JSON syntax in request body")
	}
	// Decode using json.NewDecoder for safer parsing
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
}

// unmarshalReplicationBody safely unmarshals and validates replication body from test request.
func unmarshalReplicationBody(t *testing.T, content []byte) utils.UpdateReplicationBody {
	t.Helper()
	var body utils.UpdateReplicationBody
	safeJSONDecode(t, content, &body)
	// Validate output data
	if body.RepoKey == "" {
		t.Log("warning: unmarshaled replication body has empty RepoKey")
	}
	return body
}

var (
	templatesPath = filepath.Join("..", "testdata", "replication")
	expected      = utils.CreateUpdateReplicationBody(
		utils.ReplicationParams{
			CronExp:                  "repl-cronExp",
			RepoKey:                  "repl-RepoKey",
			EnableEventReplication:   true,
			SocketTimeoutMillis:      123,
			Enabled:                  true,
			SyncDeletes:              true,
			SyncProperties:           true,
			SyncStatistics:           true,
			PathPrefix:               "repl-pathprefix",
			IncludePathPrefixPattern: "repl-pathprefix",
		},
	)
)

func TestCreateReplicationPathPrefix(t *testing.T) {
	// Create replication command
	replicationCmd := NewReplicationCreateCommand()
	testServer := createMockServer(t, replicationCmd)
	defer testServer.Close()

	// Test create replication with template containing "pathPrefix"
	replicationCmd.SetTemplatePath(filepath.Join(templatesPath, "template-pathPrefix.json"))
	assert.NoError(t, replicationCmd.Run())
}

func TestReplicationIncludePathPrefix(t *testing.T) {
	// Create replication command
	replicationCmd := NewReplicationCreateCommand()
	testServer := createMockServer(t, replicationCmd)
	defer testServer.Close()

	// Test create replication with template containing "includePathPrefixPattern"
	replicationCmd.SetTemplatePath(filepath.Join(templatesPath, "template-includePathPrefixPattern.json"))
	assert.NoError(t, replicationCmd.Run())
}

// Create mock server to test replication body
// t              - The testing object
// replicationCmd - The replication-create command to populate with the server URL
func createMockServer(t *testing.T, replicationCmd *ReplicationCreateCommand) *httptest.Server {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		// Read body
		content, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		// Unmarshal and validate body
		actual := unmarshalReplicationBody(t, content)

		// Make sure the sent replication body equals to the expected
		assert.Equal(t, *expected, actual)
	}))
	serverDetails := &config.ServerDetails{ArtifactoryUrl: testServer.URL + "/"}
	replicationCmd.SetServerDetails(serverDetails)
	return testServer
}
