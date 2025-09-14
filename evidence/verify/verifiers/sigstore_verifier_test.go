package verifiers

import (
	"errors"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/stretchr/testify/assert"
)

func TestSigstoreVerifier_VerifyNilResult(t *testing.T) {
	verifier := &sigstoreVerifier{}

	err := verifier.verify(nil)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence verification or Sigstore bundle provided for verification", err.Error())
}

func TestSigstoreVerifier_VerifyResultWithNilSigstoreBundle(t *testing.T) {
	verifier := &sigstoreVerifier{}

	result := &model.EvidenceVerification{
		SigstoreBundle:     nil,
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Equal(t, "empty evidence verification or Sigstore bundle provided for verification", err.Error())
}

func TestSigstoreVerifier_VerifyNilProtobufBundle(t *testing.T) {
	mockProvider := &MockTUFRootCertificateProvider{}
	mockProvider.On("LoadTUFRootCertificate").Return(nil, nil)

	verifier := &sigstoreVerifier{
		rootCertificateProvider: mockProvider,
	}

	result := &model.EvidenceVerification{
		SigstoreBundle: &bundle.Bundle{
			Bundle: nil,
		},
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Equal(t, "invalid bundle: missing protobuf bundle", err.Error())

	mockProvider.AssertExpectations(t)
}

func TestSigstoreVerifier_VerifyNilBundle(t *testing.T) {
	mockProvider := &MockTUFRootCertificateProvider{}
	// Even for nil bundle, the TUF provider gets called first, so we need to mock it
	mockProvider.On("LoadTUFRootCertificate").Return(nil, errors.New("mock TUF provider"))

	verifier := &sigstoreVerifier{
		rootCertificateProvider: mockProvider,
	}

	result := &model.EvidenceVerification{
		SigstoreBundle: &bundle.Bundle{
			Bundle: nil, // nil protobuf bundle
		},
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load TUF root certificate")

	// Verify that the mock provider was called
	mockProvider.AssertExpectations(t)
}

func TestSigstoreVerifier_VerifyTUFProviderError(t *testing.T) {
	mockProvider := &MockTUFRootCertificateProvider{}
	mockProvider.On("LoadTUFRootCertificate").Return(nil, errors.New("TUF load failed"))

	verifier := &sigstoreVerifier{
		rootCertificateProvider: mockProvider,
	}

	result := &model.EvidenceVerification{
		SigstoreBundle: &bundle.Bundle{
			Bundle: &protobundle.Bundle{}, // Empty but not nil
		},
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load TUF root certificate")
	assert.Contains(t, err.Error(), "TUF load failed")

	// Verify that the mock provider was called
	mockProvider.AssertExpectations(t)
}

func TestSigstoreVerifier_Creation(t *testing.T) {
	verifier := newSigstoreVerifier()
	assert.NotNil(t, verifier)

	// Verify it implements the interface
	var _ sigstoreVerifierInterface = verifier
}

func TestSigstoreVerifier_VerifyNilBundleAfterTUFSuccess(t *testing.T) {
	mockProvider := &MockTUFRootCertificateProvider{}
	// Mock successful TUF loading to test bundle validation
	mockProvider.On("LoadTUFRootCertificate").Return(nil, nil) // Return success

	verifier := &sigstoreVerifier{
		rootCertificateProvider: mockProvider,
	}

	result := &model.EvidenceVerification{
		SigstoreBundle: &bundle.Bundle{
			Bundle: nil, // nil protobuf bundle
		},
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bundle: missing protobuf bundle")

	// Verify that the mock provider was called
	mockProvider.AssertExpectations(t)
}

func TestSigstoreVerifier_InvalidBundleCreation(t *testing.T) {
	mockProvider := &MockTUFRootCertificateProvider{}
	// Mock successful TUF loading
	mockProvider.On("LoadTUFRootCertificate").Return(nil, nil)

	verifier := &sigstoreVerifier{
		rootCertificateProvider: mockProvider,
	}

	// Create a minimal invalid protobuf bundle that will fail bundle creation
	result := &model.EvidenceVerification{
		SigstoreBundle: &bundle.Bundle{
			Bundle: &protobundle.Bundle{
				MediaType: "invalid-media-type",
			},
		},
		VerificationResult: model.EvidenceVerificationResult{},
	}

	err := verifier.verify(result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create bundle for verification")

	mockProvider.AssertExpectations(t)
}
