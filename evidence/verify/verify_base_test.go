package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify/reports"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/onemodel"
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

// MockOneModelManagerWithQueryCapture captures the GraphQL query for testing
type MockOneModelManagerWithQueryCapture struct {
	GraphqlResponse []byte
	GraphqlError    error
	CapturedQuery   []byte
}

func (m *MockOneModelManagerWithQueryCapture) GraphqlQuery(query []byte) ([]byte, error) {
	m.CapturedQuery = query
	if m.GraphqlError != nil {
		return nil, m.GraphqlError
	}
	return m.GraphqlResponse, nil
}

// Satisfy interface for onemodel.Manager
var _ onemodel.Manager = (*MockOneModelManagerBase)(nil)

func TestVerifyEvidenceBase_PrintVerifyResult_JSON(t *testing.T) {
	v := &verifyEvidenceBase{format: "json"}
	resp := &model.VerificationResponse{
		Subject: model.Subject{
			Sha256: "test-checksum",
		},
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
		Subject: model.Subject{
			Sha256: "test-checksum",
		},
		OverallVerificationStatus: model.Failed,
		EvidenceVerifications: &[]model.EvidenceVerification{{
			SubjectChecksum: "test-checksum",
			PredicateType:   "test-type",
			CreatedBy:       "test-user",
			CreatedAt:       "2024-01-01T00:00:00Z",
			VerificationResult: model.EvidenceVerificationResult{
				Sha256VerificationStatus:     model.Failed,
				SignaturesVerificationStatus: model.Success,
			},
		}},
	}

	// Test that print function executes without error - stdout output testing is complex
	err := v.printVerifyResult(resp)
	// Should get an exit code error since verification failed
	assert.Error(t, err)
}

func TestVerifyEvidenceBase_PrintVerifyResult_Text_Success(t *testing.T) {
	v := &verifyEvidenceBase{format: "text"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		EvidenceVerifications: &[]model.EvidenceVerification{{
			PredicateType: "test-type",
			CreatedBy:     "test-user",
			CreatedAt:     "2024-01-01T00:00:00Z",
			VerificationResult: model.EvidenceVerificationResult{
				Sha256VerificationStatus:     model.Success,
				SignaturesVerificationStatus: model.Success,
			},
		}},
	}

	// Test that the print function executes without error for successful verification
	err := v.printVerifyResult(resp)
	assert.NoError(t, err)
}

func TestVerifyEvidenceBase_PrintVerifyResult_Text_Failed(t *testing.T) {
	v := &verifyEvidenceBase{format: "text"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Failed,
		EvidenceVerifications: &[]model.EvidenceVerification{{
			PredicateType: "test-type",
			CreatedBy:     "test-user",
			CreatedAt:     "2024-01-01T00:00:00Z",
			VerificationResult: model.EvidenceVerificationResult{
				Sha256VerificationStatus:     model.Failed,
				SignaturesVerificationStatus: model.Failed,
			},
		}},
	}

	// Test that the print function returns error for failed verification
	err := v.printVerifyResult(resp)
	assert.Error(t, err)
}

func TestVerifyEvidenceBase_UnknownFormat_DefaultsToText(t *testing.T) {
	v := &verifyEvidenceBase{format: "unknown"}
	resp := &model.VerificationResponse{
		OverallVerificationStatus: model.Success,
		EvidenceVerifications: &[]model.EvidenceVerification{{
			PredicateType: "test-type",
			CreatedBy:     "test-user",
			CreatedAt:     "2024-01-01T00:00:00Z",
			VerificationResult: model.EvidenceVerificationResult{
				Sha256VerificationStatus:     model.Success,
				SignaturesVerificationStatus: model.Success,
			},
		}},
	}

	// Test that unknown format defaults to text and executes without error
	err := v.printVerifyResult(resp)
	assert.NoError(t, err)
}

func TestVerifyEvidenceBase_CreateArtifactoryClient_Success(t *testing.T) {
	serverDetails := &config.ServerDetails{Url: "http://test.com"}
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
		serverDetails: &config.ServerDetails{
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
func TestVerifyEvidenceBase_QueryEvidenceMetadata_SuccessWithPublicKey(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","predicateCategory":"cat","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"},"signingKey":{"alias":"a"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: true,
	}
	edges, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)
	assert.NotNil(t, edges)
	assert.Equal(t, 1, len(*edges))
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_SuccessWithoutPublicKey(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","predicateCategory":"cat","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"},"signingKey":{"alias":"a"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: false,
	}
	edges, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)
	assert.NotNil(t, edges)
	assert.Equal(t, 1, len(*edges))
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_GraphqlError(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlError: errors.New("graphql query failed"),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error querying evidence from One-Model service")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_UnmarshalError(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte("invalid json"),
	}

	v := &verifyEvidenceBase{oneModelClient: mockManager}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal evidence metadata")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_NoEdges(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
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
		serverDetails:  &config.ServerDetails{Url: "http://test.com"},
		oneModelClient: nil,
	}

	// This should fail when trying to query GraphQL with basic server config
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error querying evidence from One-Model service")
}

func TestVerifyEvidenceBase_SearchEvidenceQueryExactMatch(t *testing.T) {
	// Test the exact query string to protect against accidental modifications
	// This test ensures the GraphQL query structure remains unchanged
	expectedQuery := `{"query":"{ evidence { searchEvidence( where: { hasSubjectWith: { repositoryKey: \"%s\", path: \"%s\", name: \"%s\" }} ) { edges { cursor node { downloadPath predicateType createdAt createdBy subject { sha256 } signingKey {alias, publicKey} } } } } }"}`

	assert.Equal(t, expectedQuery, searchEvidenceQueryWithPublicKey,
		"searchEvidenceQueryWithPublicKey has been modified. If this change is intentional, please update this test. "+
			"This test protects against accidental modifications to the GraphQL query structure.")

	// Verify the query can be formatted with test parameters
	formattedQuery := fmt.Sprintf(searchEvidenceQueryWithPublicKey, "test-repo", "test/path", "test-file.txt")
	assert.Contains(t, formattedQuery, "test-repo")
	assert.Contains(t, formattedQuery, "test/path")
	assert.Contains(t, formattedQuery, "test-file.txt")

	// Verify the formatted query is valid JSON structure
	var jsonCheck any
	err := json.Unmarshal([]byte(formattedQuery), &jsonCheck)
	assert.NoError(t, err, "Formatted query should be valid JSON")
}

func TestVerifyEvidenceBase_Integration(t *testing.T) {
	// Test the integration of verifyEvidenceBase components
	v := &verifyEvidenceBase{
		serverDetails: &config.ServerDetails{Url: "http://test.com"},
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
			name:   "Default format",
			format: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := &verifyEvidenceBase{format: tc.format}
			resp := &model.VerificationResponse{
				OverallVerificationStatus: model.Success,
				EvidenceVerifications: &[]model.EvidenceVerification{{
					PredicateType: "test-type",
					CreatedBy:     "test-user",
					CreatedAt:     "2024-01-01T00:00:00Z",
					VerificationResult: model.EvidenceVerificationResult{
						SignaturesVerificationStatus: model.Success,
					},
				}},
			}

			err := v.printVerifyResult(resp)
			assert.NoError(t, err)
		})
	}
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_QueryContainsPublicKey_WhenUseArtifactoryKeysTrue(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"},"signingKey":{"alias":"a","publicKey":"test-key"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: true,
	}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)

	// Verify that the captured query contains publicKey
	capturedQuery := string(mockManager.CapturedQuery)
	assert.Contains(t, capturedQuery, "publicKey", "Query should contain publicKey when useArtifactoryKeys is true")
	assert.Contains(t, capturedQuery, "signingKey", "Query should contain signingKey when useArtifactoryKeys is true")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_QueryContainsPublicKey_WhenUseArtifactoryKeysFalse(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: false,
	}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)

	// Verify that the captured query does NOT contain publicKey or signingKey
	capturedQuery := string(mockManager.CapturedQuery)
	assert.NotContains(t, capturedQuery, "publicKey", "Query should NOT contain publicKey when useArtifactoryKeys is false")
	assert.NotContains(t, capturedQuery, "signingKey", "Query should NOT contain signingKey when useArtifactoryKeys is false")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_QueryStructure_WithPublicKey(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"},"signingKey":{"alias":"a","publicKey":"test-key"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: true,
	}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)

	// Verify the query structure and parameters
	capturedQuery := string(mockManager.CapturedQuery)
	assert.Contains(t, capturedQuery, "test-repo", "Query should contain the repository parameter")
	assert.Contains(t, capturedQuery, "test/path", "Query should contain the path parameter")
	assert.Contains(t, capturedQuery, "test-file.txt", "Query should contain the name parameter")

	// Verify the GraphQL structure includes signingKey with publicKey
	assert.Contains(t, capturedQuery, "signingKey {alias, publicKey}", "Query should request signingKey with alias and publicKey")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_QueryStructure_WithoutPublicKey(t *testing.T) {
	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlResponse: []byte(`{"data":{"evidence":{"searchEvidence":{"edges":[{"cursor":"c","node":{"downloadPath":"p","predicateType":"t","createdAt":"now","createdBy":"me","subject":{"sha256":"abc"}}}]}}}}`),
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: false,
	}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.NoError(t, err)

	// Verify the query structure and parameters
	capturedQuery := string(mockManager.CapturedQuery)
	assert.Contains(t, capturedQuery, "test-repo", "Query should contain the repository parameter")
	assert.Contains(t, capturedQuery, "test/path", "Query should contain the path parameter")
	assert.Contains(t, capturedQuery, "test-file.txt", "Query should contain the name parameter")

	// Verify the GraphQL structure does NOT include signingKey with publicKey
	assert.NotContains(t, capturedQuery, "signingKey {alias, publicKey}", "Query should NOT request signingKey with alias and publicKey when useArtifactoryKeys is false")
}

func TestVerifyEvidenceBase_QueryEvidenceMetadata_GraphqlValidationError_PublicKey(t *testing.T) {
	// Mock the GraphQL validation error for publicKey field
	graphqlError := fmt.Errorf(`{"errors":[{"message":"Cannot query field \"publicKey\" on type \"EvidenceSigningKey\"."}]}`)

	mockManager := &MockOneModelManagerWithQueryCapture{
		GraphqlError: graphqlError,
	}

	v := &verifyEvidenceBase{
		oneModelClient:     mockManager,
		useArtifactoryKeys: true,
	}
	_, err := v.queryEvidenceMetadata("test-repo", "test/path", "test-file.txt")
	assert.Error(t, err)

	// Check if the error contains the expected version requirement message
	assert.Contains(t, err.Error(), "the evidence service version should be at least 7.125.0")
	assert.Contains(t, err.Error(), "the onemodel version should be at least 1.55.0")
}

func TestIsVerificationSucceed(t *testing.T) {
	tests := []struct {
		name           string
		verification   model.EvidenceVerification
		expectedResult bool
		description    string
	}{
		{
			name: "DSSE_BothSuccess",
			verification: model.EvidenceVerification{
				MediaType: model.SimpleDSSE,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:     model.Success,
					SignaturesVerificationStatus: model.Success,
				},
			},
			expectedResult: true,
			description:    "DSSE verification should succeed when both Sha256 and Signatures are success",
		},
		{
			name: "DSSE_Sha256Failed",
			verification: model.EvidenceVerification{
				MediaType: model.SimpleDSSE,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:     model.Failed,
					SignaturesVerificationStatus: model.Success,
				},
			},
			expectedResult: false,
			description:    "DSSE verification should fail when Sha256 verification fails",
		},
		{
			name: "DSSE_SignaturesFailed",
			verification: model.EvidenceVerification{
				MediaType: model.SimpleDSSE,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:     model.Success,
					SignaturesVerificationStatus: model.Failed,
				},
			},
			expectedResult: false,
			description:    "DSSE verification should fail when Signatures verification fails",
		},
		{
			name: "DSSE_BothFailed",
			verification: model.EvidenceVerification{
				MediaType: model.SimpleDSSE,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:     model.Failed,
					SignaturesVerificationStatus: model.Failed,
				},
			},
			expectedResult: false,
			description:    "DSSE verification should fail when both Sha256 and Signatures verification fail",
		},
		{
			name: "SigstoreBundle_Success",
			verification: model.EvidenceVerification{
				MediaType: model.SigstoreBundle,
				VerificationResult: model.EvidenceVerificationResult{
					SigstoreBundleVerificationStatus: model.Success,
				},
			},
			expectedResult: true,
			description:    "Sigstore bundle verification should succeed when SigstoreBundleVerificationStatus is success",
		},
		{
			name: "SigstoreBundle_Failed",
			verification: model.EvidenceVerification{
				MediaType: model.SigstoreBundle,
				VerificationResult: model.EvidenceVerificationResult{
					SigstoreBundleVerificationStatus: model.Failed,
				},
			},
			expectedResult: false,
			description:    "Sigstore bundle verification should fail when SigstoreBundleVerificationStatus is failed",
		},
		{
			name: "SigstoreBundle_SuccessWithOthersFailed",
			verification: model.EvidenceVerification{
				MediaType: model.SigstoreBundle,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:         model.Failed,
					SignaturesVerificationStatus:     model.Failed,
					SigstoreBundleVerificationStatus: model.Success,
				},
			},
			expectedResult: true,
			description:    "Verification should succeed with Sigstore bundle success even if DSSE fields are failed",
		},
		{
			name: "DSSE_SuccessWithSigstoreFailed",
			verification: model.EvidenceVerification{
				MediaType: model.SimpleDSSE,
				VerificationResult: model.EvidenceVerificationResult{
					Sha256VerificationStatus:         model.Success,
					SignaturesVerificationStatus:     model.Success,
					SigstoreBundleVerificationStatus: model.Failed,
				},
			},
			expectedResult: true,
			description:    "Verification should succeed with DSSE success even if Sigstore bundle field is failed",
		},
		{
			name: "AllFieldsEmpty",
			verification: model.EvidenceVerification{
				VerificationResult: model.EvidenceVerificationResult{},
			},
			expectedResult: false,
			description:    "Verification should fail when all verification status fields are empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reports.IsVerificationSucceed(tt.verification)
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}
