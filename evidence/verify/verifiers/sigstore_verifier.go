package verifiers

import (
	"encoding/hex"
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/verify/verifiers/ca"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/pkg/errors"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const sigstoreKeySource = "Sigstore Bundle Key"

type sigstoreVerifierInterface interface {
	verify(subjectSha256 string, result *model.EvidenceVerification) error
}

type sigstoreVerifier struct {
	rootCertificateProvider ca.TUFRootCertificateProvider
}

func newSigstoreVerifier() sigstoreVerifierInterface {
	return &sigstoreVerifier{}
}

func (v *sigstoreVerifier) verify(subjectSha256 string, result *model.EvidenceVerification) error {
	if result == nil || result.SigstoreBundle == nil {
		return fmt.Errorf("empty evidence verification or Sigstore bundle provided for verification")
	}

	if v.rootCertificateProvider == nil {
		v.rootCertificateProvider = ca.NewTUFRootCertificateProvider()
	}
	certificate, err := v.rootCertificateProvider.LoadTUFRootCertificate()
	if err != nil {
		return fmt.Errorf("failed to load TUF root certificate: %v", err)
	}

	verifierConfig := []verify.VerifierOption{
		verify.WithSignedCertificateTimestamps(1),
		verify.WithObserverTimestamps(1),
		verify.WithTransparencyLog(1),
	}

	verifier, err := verify.NewVerifier(certificate, verifierConfig...)
	if err != nil {
		return fmt.Errorf("failed to create signature verifier: %v", err)
	}

	protoBundle := result.SigstoreBundle.Bundle
	if protoBundle == nil {
		return errors.New("invalid bundle: missing protobuf bundle")
	}

	bundleToVerify, err := bundle.NewBundle(protoBundle)
	if err != nil {
		return errors.Wrap(err, "failed to create bundle for verification")
	}

	digestBytes, err := hex.DecodeString(subjectSha256)
	if err != nil {
		return fmt.Errorf("invalid hex digest: %w", err)
	}
	policy := verify.NewPolicy(
		verify.WithArtifactDigest("sha256", digestBytes), // Use digest for artifact verification
		verify.WithoutIdentitiesUnsafe(),                 // Skip identity verification for now
	)

	verificationResult, err := verifier.Verify(bundleToVerify, policy)
	if err != nil {
		result.VerificationResult.SigstoreBundleVerificationStatus = model.Failed
		result.VerificationResult.FailureReason = err.Error()
		return nil //nolint:nilerr
	}
	result.VerificationResult.KeySource = sigstoreKeySource
	result.VerificationResult.SigstoreBundleVerificationStatus = model.Success
	result.VerificationResult.SigstoreBundleVerificationResult = verificationResult
	return nil
}
