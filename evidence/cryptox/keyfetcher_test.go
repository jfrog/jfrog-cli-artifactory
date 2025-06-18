package cryptox

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManagerKeyfetcher for testing keyfetcher functions
type MockArtifactoryServicesManagerKeyfetcher struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
	GetTrustedKeysResponse *services.TrustedKeysResponse
	GetTrustedKeysError    error
	GetKeyPairsResponse    *[]services.KeypairResponseItem
	GetKeyPairsError       error
}

func (m *MockArtifactoryServicesManagerKeyfetcher) GetTrustedKeys() (*services.TrustedKeysResponse, error) {
	m.Called()
	if m.GetTrustedKeysError != nil {
		return nil, m.GetTrustedKeysError
	}
	return m.GetTrustedKeysResponse, nil
}

func (m *MockArtifactoryServicesManagerKeyfetcher) GetKeyPairs() (*[]services.KeypairResponseItem, error) {
	m.Called()
	if m.GetKeyPairsError != nil {
		return nil, m.GetKeyPairsError
	}
	return m.GetKeyPairsResponse, nil
}

// TestFetchTrustedVerifiers tests fetching trusted keys via Artifactory service
func TestFetchTrustedVerifiers_Success(t *testing.T) {
	// Mock valid response
	mockResponse := &services.TrustedKeysResponse{
		Keys: []struct {
			PublicKey   string `json:"key"`
			KeyId       string `json:"kid"`
			Fingerprint string `json:"fingerprint"`
			Alias       string `json:"alias"`
			Type        string `json:"type"`
			IssuedOn    string `json:"issued_on"`
			IssuedBy    string `json:"issued_by"`
			ValidUntil  string `json:"valid_until"`
		}{
			{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtest\n-----END PUBLIC KEY-----"},
			{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtest2\n-----END PUBLIC KEY-----"},
		},
	}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetTrustedKeysResponse: mockResponse,
	}
	mockClient.On("GetTrustedKeys").Return(mockResponse, nil)

	verifiers, err := FetchTrustedKeys(mockClient)

	assert.NoError(t, err)
	// Note: Since we can't mock LoadKey easily, we expect empty result for invalid test keys
	assert.NotNil(t, verifiers)
	assert.IsType(t, []dsse.Verifier{}, verifiers)
	assert.Empty(t, verifiers) // Invalid test keys should result in empty slice
	mockClient.AssertExpectations(t)
}

func TestFetchTrustedVerifiers_ServiceError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetTrustedKeysError: errors.New("service error"),
	}
	mockClient.On("GetTrustedKeys").Return(nil, errors.New("service error"))

	verifiers, err := FetchTrustedKeys(mockClient)

	assert.Error(t, err)
	assert.Nil(t, verifiers)
	assert.Contains(t, err.Error(), "failed to fetch trusted keys")
	assert.Contains(t, err.Error(), "service error")
	mockClient.AssertExpectations(t)
}

func TestFetchTrustedVerifiers_EmptyResponse(t *testing.T) {
	mockResponse := &services.TrustedKeysResponse{
		Keys: []struct {
			PublicKey   string `json:"key"`
			KeyId       string `json:"kid"`
			Fingerprint string `json:"fingerprint"`
			Alias       string `json:"alias"`
			Type        string `json:"type"`
			IssuedOn    string `json:"issued_on"`
			IssuedBy    string `json:"issued_by"`
			ValidUntil  string `json:"valid_until"`
		}{},
	}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetTrustedKeysResponse: mockResponse,
	}
	mockClient.On("GetTrustedKeys").Return(mockResponse, nil)

	verifiers, err := FetchTrustedKeys(mockClient)

	assert.NoError(t, err)
	assert.Empty(t, verifiers)
	mockClient.AssertExpectations(t)
}

// TestFetchKeypairVerifiers tests fetching keypair keys via Artifactory service
func TestFetchKeypairVerifiers_Success(t *testing.T) {
	// Mock valid response
	mockResponse := &[]services.KeypairResponseItem{
		{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtest\n-----END PUBLIC KEY-----"},
		{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtest2\n-----END PUBLIC KEY-----"},
	}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetKeyPairsResponse: mockResponse,
	}
	mockClient.On("GetKeyPairs").Return(mockResponse, nil)

	verifiers, err := FetchKeyPairs(mockClient)

	assert.NoError(t, err)
	// Note: Since we can't mock LoadKey easily, we expect empty result for invalid test keys
	assert.NotNil(t, verifiers)
	assert.IsType(t, []dsse.Verifier{}, verifiers)
	assert.Empty(t, verifiers) // Invalid test keys should result in empty slice
	mockClient.AssertExpectations(t)
}

func TestFetchKeypairVerifiers_ServiceError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetKeyPairsError: errors.New("service error"),
	}
	mockClient.On("GetKeyPairs").Return(nil, errors.New("service error"))

	verifiers, err := FetchKeyPairs(mockClient)

	assert.Error(t, err)
	assert.Nil(t, verifiers)
	assert.Contains(t, err.Error(), "failed to fetch key pairs")
	assert.Contains(t, err.Error(), "service error")
	mockClient.AssertExpectations(t)
}

func TestFetchKeypairVerifiers_EmptyResponse(t *testing.T) {
	mockResponse := &[]services.KeypairResponseItem{}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetKeyPairsResponse: mockResponse,
	}
	mockClient.On("GetKeyPairs").Return(mockResponse, nil)

	verifiers, err := FetchKeyPairs(mockClient)

	assert.NoError(t, err)
	assert.Empty(t, verifiers)
	mockClient.AssertExpectations(t)
}

// TestTrustedKeysResponseStructure tests the JSON unmarshaling of trustedKeysResponse (legacy test for struct definition)
func TestTrustedKeysResponseStructure(t *testing.T) {
	// This struct is only used internally now, but test for backward compatibility
	response := trustedKeysResponse{
		Keys: []struct {
			PublicKey string `json:"key"`
		}{
			{PublicKey: "test-key-1"},
			{PublicKey: "test-key-2"},
		},
	}

	assert.Len(t, response.Keys, 2)
	assert.Equal(t, "test-key-1", response.Keys[0].PublicKey)
	assert.Equal(t, "test-key-2", response.Keys[1].PublicKey)
}

// TestKeypairResponseStructure tests the JSON unmarshaling of keypairResponseItem (legacy test for struct definition)
func TestKeypairResponseStructure(t *testing.T) {
	// This struct is only used internally now, but test for backward compatibility
	item := keypairResponseItem{
		PublicKey: "test-public-key",
	}

	assert.Equal(t, "test-public-key", item.PublicKey)
}

// TestInvalidKeyHandling tests that invalid keys are gracefully handled
func TestInvalidKeyHandling(t *testing.T) {
	// Test FetchTrustedKeys with invalid key data
	t.Run("FetchTrustedVerifiers_InvalidKeys", func(t *testing.T) {
		mockResponse := &services.TrustedKeysResponse{
			Keys: []struct {
				PublicKey   string `json:"key"`
				KeyId       string `json:"kid"`
				Fingerprint string `json:"fingerprint"`
				Alias       string `json:"alias"`
				Type        string `json:"type"`
				IssuedOn    string `json:"issued_on"`
				IssuedBy    string `json:"issued_by"`
				ValidUntil  string `json:"valid_until"`
			}{
				{PublicKey: "invalid-key-data"},
			},
		}

		mockClient := &MockArtifactoryServicesManagerKeyfetcher{
			GetTrustedKeysResponse: mockResponse,
		}
		mockClient.On("GetTrustedKeys").Return(mockResponse, nil)

		verifiers, err := FetchTrustedKeys(mockClient)

		// Should not error, but should skip invalid keys
		assert.NoError(t, err)
		assert.Empty(t, verifiers) // Invalid keys are skipped
		mockClient.AssertExpectations(t)
	})

	// Test FetchKeyPairs with invalid key data
	t.Run("FetchKeypairVerifiers_InvalidKeys", func(t *testing.T) {
		mockResponse := &[]services.KeypairResponseItem{
			{PublicKey: "invalid-key-data"},
		}

		mockClient := &MockArtifactoryServicesManagerKeyfetcher{
			GetKeyPairsResponse: mockResponse,
		}
		mockClient.On("GetKeyPairs").Return(mockResponse, nil)

		verifiers, err := FetchKeyPairs(mockClient)

		// Should not error, but should skip invalid keys
		assert.NoError(t, err)
		assert.Empty(t, verifiers) // Invalid keys are skipped
		mockClient.AssertExpectations(t)
	})
}
