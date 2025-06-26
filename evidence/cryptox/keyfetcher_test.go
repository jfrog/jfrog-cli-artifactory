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
			{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkeAsaT7uICw20vjj+rBl\nRqjpjdp2C+Ixz+ATqHsEZrITnN4EZ3lK8L7pGihmwYrLTa+Zh7LZkCwMa/rUg6V7\n8p+3Vx5c5AxlMfLr7t53fLMb/8YZgXMkJfsJ5IzyRmsS8dzFvk1PzGW0Z+Vn55kJ\nGq4kuTq10kAULtVVBbiCj7Trc4wgjiwuKEavYLhUDUWUXvjy9L69bZJYOFqFBd62\nTgp3cTvMpL6Y3inihMfz9jQu9Mt6jgdzpJJ5eStwTWPuVLw5PAEG5QDkzUF4JWkm\n1yqSLXQeFRfYw533TXcg9w6xi0TuVeGPDlDEg42WltjLLhwa6Un7p8oILtAhDQfp\neQIDAQAB\n-----END PUBLIC KEY-----"},
			{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA3t2pFq+I1OMzbex9JEyE\nQWbWSiYlaXsXVFjNiN9M0x3loWPSN+i3pH8892PcoHwXD0k6x830fS5nu0/pG4DN\nIYkxNWhYh5BujEG1Sy2VGMkZbcMcFG+te0QPErZzt8yFKj/s3QH+FMTMIqfFIPr5\n9krVMkFL1gA/ZXTfqu8FnU1UMs2We/94ymdI9pkEIGxAG5leNDtCLGzZfIdnclXm\nulHK6PaOKi7gfBNJhlmU3RifDfgVr2HbUPOyppBKaAYCIl3klg7i5oQLiZJpVV9E\no+1jUAvrvN2ue6X4s3B5jmyfaXwSEs7BJpWjJhtGDG6LLocpoKIIuushy3GHHvPv\n0wIDAQAB\n-----END PUBLIC KEY-----"},
		},
	}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetTrustedKeysResponse: mockResponse,
	}
	mockClient.On("GetTrustedKeys").Return(mockResponse, nil)

	verifiers, err := FetchTrustedKeys(mockClient)

	assert.NoError(t, err)
	// With valid RSA keys, LoadKey should succeed and create verifiers
	assert.NotNil(t, verifiers)
	assert.IsType(t, []dsse.Verifier{}, verifiers)
	// Valid RSA keys should result in non-empty verifiers slice
	assert.Len(t, verifiers, 2) // We expect 2 verifiers for 2 valid keys
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
		{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkeAsaT7uICw20vjj+rBl\nRqjpjdp2C+Ixz+ATqHsEZrITnN4EZ3lK8L7pGihmwYrLTa+Zh7LZkCwMa/rUg6V7\n8p+3Vx5c5AxlMfLr7t53fLMb/8YZgXMkJfsJ5IzyRmsS8dzFvk1PzGW0Z+Vn55kJ\nGq4kuTq10kAULtVVBbiCj7Trc4wgjiwuKEavYLhUDUWUXvjy9L69bZJYOFqFBd62\nTgp3cTvMpL6Y3inihMfz9jQu9Mt6jgdzpJJ5eStwTWPuVLw5PAEG5QDkzUF4JWkm\n1yqSLXQeFRfYw533TXcg9w6xi0TuVeGPDlDEg42WltjLLhwa6Un7p8oILtAhDQfp\neQIDAQAB\n-----END PUBLIC KEY-----"},
		{PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA3t2pFq+I1OMzbex9JEyE\nQWbWSiYlaXsXVFjNiN9M0x3loWPSN+i3pH8892PcoHwXD0k6x830fS5nu0/pG4DN\nIYkxNWhYh5BujEG1Sy2VGMkZbcMcFG+te0QPErZzt8yFKj/s3QH+FMTMIqfFIPr5\n9krVMkFL1gA/ZXTfqu8FnU1UMs2We/94ymdI9pkEIGxAG5leNDtCLGzZfIdnclXm\nulHK6PaOKi7gfBNJhlmU3RifDfgVr2HbUPOyppBKaAYCIl3klg7i5oQLiZJpVV9E\no+1jUAvrvN2ue6X4s3B5jmyfaXwSEs7BJpWjJhtGDG6LLocpoKIIuushy3GHHvPv\n0wIDAQAB\n-----END PUBLIC KEY-----"},
	}

	mockClient := &MockArtifactoryServicesManagerKeyfetcher{
		GetKeyPairsResponse: mockResponse,
	}
	mockClient.On("GetKeyPairs").Return(mockResponse, nil)

	verifiers, err := FetchKeyPairs(mockClient)

	assert.NoError(t, err)
	// With valid RSA keys, LoadKey should succeed and create verifiers
	assert.NotNil(t, verifiers)
	assert.IsType(t, []dsse.Verifier{}, verifiers)
	// Valid RSA keys should result in non-empty verifiers slice
	assert.Len(t, verifiers, 2) // We expect 2 verifiers for 2 valid keys
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
