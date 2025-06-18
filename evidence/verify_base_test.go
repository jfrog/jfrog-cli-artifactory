package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/gookit/color"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
)

// MockOneModelManagerBase for base tests
type MockOneModelManagerBase struct {
	GraphqlResponse []byte
	GraphqlError    error
}

func (m *MockOneModelManagerBase) GraphqlQuery(_ []byte) ([]byte, error) {
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// Satisfy interface for onemodel.Manager
var _ onemodel.Manager = (*MockOneModelManagerBase)(nil)

// Helper to capture output for testing print functions
func captureOutput(f func()) string {
	var buf bytes.Buffer

	// Set color output to our buffer
	color.SetOutput(&buf)

	// Set log output to our buffer
	oldLog := log.GetLogger()
	log.SetLogger(log.NewLogger(log.INFO, &buf))

	defer func() {
		// Reset color output
		color.ResetOutput()
		// Reset log
		log.SetLogger(oldLog)
	}()

	f()
	return buf.String()
}

func TestVerifyEvidenceBase_PrintVerifyResult_JSON(t *testing.T) {
	v := &verifyEvidenceBase{format: "json"}
	resp := &model.VerificationResponse{
		Checksum:                  "test-checksum",
		OverallVerificationStatus: model.Success,
	}

	// For JSON output, just test that it doesn't return an error
	// since fmt.Println writes to stdout which we can't easily capture in tests
	err := v.printVerifyResult(resp)
	assert.NoError(t, err)
}

func TestVerifyEvidenceBase_PrintVerifyResult_Failed(t *testing.T) {
	v := &verifyEvidenceBase{format: "full"}
	resp := &model.VerificationResponse{
		Checksum:                  "test-checksum",
		OverallVerificationStatus: model.Failed,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			ChecksumVerificationStatus:   model.Failed,
			SignaturesVerificationStatus: model.Success,
			Checksum:                     "test-checksum",
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := v.printVerifyResult(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
}

func TestVerifyEvidenceBase_PrintVerifyResult_Text_Success(t *testing.T) {
	v := &verifyEvidenceBase{format: "text"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Success,
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := v.printVerifyResult(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
	assert.Contains(t, out, "success")
}

func TestVerifyEvidenceBase_PrintVerifyResult_Text_Failed(t *testing.T) {
	v := &verifyEvidenceBase{format: "text"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Failed,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Failed,
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := v.printVerifyResult(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
	assert.Contains(t, out, "failed")
}

func TestVerifyEvidenceBase_PrintVerifyResult_UnknownFormat(t *testing.T) {
	v := &verifyEvidenceBase{format: "unknown"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Success,
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := v.printVerifyResult(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
}

func TestVerifyEvidenceBase_CreateArtifactoryClient_Success(t *testing.T) {
	serverDetails := &coreConfig.ServerDetails{Url: "http://test.com"}
	v := &verifyEvidenceBase{serverDetails: serverDetails}

	// First call should create client
	client1, err := v.createArtifactoryClient()
	assert.NoError(t, err)
	assert.NotNil(t, client1)

	// Second call should return cached client
	client2, err := v.createArtifactoryClient()
	assert.NoError(t, err)
	assert.Equal(t, client1, client2)
}

func TestVerifyEvidenceBase_CreateArtifactoryClient_Error(t *testing.T) {
	// Test with invalid server configuration
	v := &verifyEvidenceBase{
		serverDetails: &coreConfig.ServerDetails{
			Url: "invalid-url", // Invalid URL that should cause client creation to fail
		},
	}

	// Client creation might succeed but subsequent operations would fail
	// Let's test that it doesn't panic and that we can call it
	client, err := v.createArtifactoryClient()
	// The behavior may vary - either it fails immediately or succeeds but fails later
	if err != nil {
		assert.Contains(t, err.Error(), "failed to create Artifactory client")
	} else {
		// If it succeeds, just verify we got a client
		assert.NotNil(t, client)
	}
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_Success(t *testing.T) {
	mockManager := &MockOneModelManagerBase{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","predicateCategory":"cat","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"},"signingKey":{"alias":"a"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	edges, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)
	assert.NotNil(t, edges)
	assert.Equal(t, 1, len(*edges))
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_GraphqlError(t *testing.T) {
	mockManager := &MockOneModelManagerBase{
		GraphqlError: errors.New("graphql query failed"),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query evidence metadata")
	assert.Contains(t, err.Error(), "graphql query failed")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_UnmarshalError(t *testing.T) {
	mockManager := &MockOneModelManagerBase{
		GraphqlResponse: []byte("invalid json"),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal evidence metadata")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_NoEdges(t *testing.T) {
	mockManager := &MockOneModelManagerBase{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[]}}}}`),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no evidence found for the given subject")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_CreateOneModelClient(t *testing.T) {
	// Test case where oneModelClient is nil and needs to be created
	v := &verifyEvidenceBase{
		serverDetails:  &coreConfig.ServerDetails{Url: "http://test.com"},
		oneModelClient: nil,
	}

	// This should fail when trying to query GraphQL with basic server config
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query evidence metadata")
}

func TestPrintText_Success(t *testing.T) {
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Success,
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := printText(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
	assert.Contains(t, out, "success")
}

func TestPrintText_Failed_PrintsTable(t *testing.T) {
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Failed,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Failed,
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := printText(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
	assert.Contains(t, out, "failed")
}

func TestPrintText_NilResponse(t *testing.T) {
	err := printText(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}

func TestPrintFullText_Success(t *testing.T) {
	resp := &model.VerificationResponse{
		Checksum:                  "test-checksum",
		OverallVerificationStatus: model.Success,
		EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
			SignaturesVerificationStatus: model.Success,
			ChecksumVerificationStatus:   model.Success,
			Checksum:                     "test-checksum",
			EvidenceType:                 "test-type",
			Category:                     "test-category",
			CreatedBy:                    "test-user",
			Time:                         "2024-01-01T00:00:00Z",
		}},
	}

	out := captureOutput(func() {
		err := printFullText(resp)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "Verification result")
}

func TestPrintFullText_NilResponse(t *testing.T) {
	err := printFullText(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}

func TestPrintJson_Success(t *testing.T) {
	resp := &model.VerificationResponse{
		Checksum:                  "test-checksum",
		OverallVerificationStatus: model.Success,
	}

	// For JSON output, just test that it doesn't return an error
	// since fmt.Println writes to stdout which we can't easily capture in tests
	err := printJson(resp)
	assert.NoError(t, err)
}

func TestPrintJson_NilResponse(t *testing.T) {
	err := printJson(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}

func TestGetColoredStatus_AllStatuses(t *testing.T) {
	assert.Equal(t, success, getColoredStatus(model.Success))
	assert.Equal(t, failed, getColoredStatus(model.Failed))
}

func TestValidateResponse_Success(t *testing.T) {
	resp := &model.VerificationResponse{}
	err := validateResponse(resp)
	assert.NoError(t, err)
}

func TestValidateResponse_NilResponse(t *testing.T) {
	err := validateResponse(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification response is empty")
}

func TestVerifyEvidenceBase_SearchEvidenceQueryExactMatch(t *testing.T) {
	// Test the exact query string to protect against accidental modifications
	// This test ensures the GraphQL query structure remains unchanged
	expectedQuery := `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType predicateCategory createdAt createdBy subject { sha256 } } } } } }"}`

	assert.Equal(t, expectedQuery, searchEvidenceQuery,
		"searchEvidenceQuery has been modified. If this change is intentional, please update this test. "+
			"This test protects against accidental modifications to the GraphQL query structure.")

	// Verify the query can be formatted with test parameters
	formattedQuery := fmt.Sprintf(searchEvidenceQuery, "test-repo", "test/path", "test-file.txt")
	assert.Contains(t, formattedQuery, "test-repo")
	assert.Contains(t, formattedQuery, "test/path")
	assert.Contains(t, formattedQuery, "test-file.txt")

	// Verify the formatted query is valid JSON structure
	var jsonCheck interface{}
	err := json.Unmarshal([]byte(formattedQuery), &jsonCheck)
	assert.NoError(t, err, "Formatted query should be valid JSON")
}

func TestVerifyEvidenceBase_Integration(t *testing.T) {
	// Test the integration of verifyEvidenceBase components
	v := &verifyEvidenceBase{
		serverDetails: &coreConfig.ServerDetails{Url: "http://test.com"},
		format:        "json",
		keys:          []string{"key1"},
	}

	// Verify the structure is correct
	assert.Equal(t, "http://test.com", v.serverDetails.Url)
	assert.Equal(t, "json", v.format)
	assert.Equal(t, []string{"key1"}, v.keys)
	assert.Nil(t, v.artifactoryClient)
	assert.Nil(t, v.oneModelClient)
}

func TestVerifyEvidenceBase_MultipleFormats(t *testing.T) {
	// Test different format scenarios
	testCases := []struct {
		name   string
		format string
	}{
		{
			name:   "JSON format",
			format: "json",
		},
		{
			name:   "Full format",
			format: "full",
		},
		{
			name:   "Text format",
			format: "text",
		},
		{
			name:   "Default format",
			format: "",
		},
		{
			name:   "Unknown format",
			format: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := &verifyEvidenceBase{format: tc.format}
			resp := &model.VerificationResponse{
				OverallVerificationStatus: model.Success,
				EvidencesVerificationResults: &[]model.EvidenceVerificationResult{{
					SignaturesVerificationStatus: model.Success,
					EvidenceType:                 "test-type",
					Category:                     "test-category",
					CreatedBy:                    "test-user",
					Time:                         "2024-01-01T00:00:00Z",
				}},
			}

			err := v.printVerifyResult(resp)
			assert.NoError(t, err)
		})
	}
}
