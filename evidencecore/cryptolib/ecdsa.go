package cryptolib

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

const (
	ECDSAKeyType   = "ecdsa"
	ECDSAKeyScheme = "ecdsa-sha2-nistp256"
)

// ECDSASignerVerifier is a dsse.SignerVerifier compliant interface to sign and
// verify signatures using ECDSA keys.
type ECDSASignerVerifier struct {
	keyID     string
	curveSize int
	private   *ecdsa.PrivateKey
	public    *ecdsa.PublicKey
}

// NewECDSASignerVerifierFromSSLibKey creates an ECDSASignerVerifier from an
// SSLibKey.
func NewECDSASignerVerifierFromSSLibKey(key *SSLibKey) (*ECDSASignerVerifier, error) {
	if len(key.KeyVal.Public) == 0 {
		return nil, errorutils.CheckError(ErrInvalidKey)
	}

	_, publicParsedKey, err := decodeAndParsePEM([]byte(key.KeyVal.Public))
	if err != nil {
		return nil, errorutils.CheckError(fmt.Errorf("unable to create ECDSA signerverifier: %w", err))
	}

	puk, ok := publicParsedKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errorutils.CheckError(fmt.Errorf("couldnt convert to ecdsa public key"))
	}
	sv := &ECDSASignerVerifier{
		keyID:     key.KeyID,
		curveSize: puk.Params().BitSize,
		public:    puk,
		private:   nil,
	}

	if len(key.KeyVal.Private) > 0 {
		_, privateParsedKey, err := decodeAndParsePEM([]byte(key.KeyVal.Private))
		if err != nil {
			return nil, errorutils.CheckError(fmt.Errorf("unable to create ECDSA signerverifier: %w", err))
		}

		pk, ok := privateParsedKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errorutils.CheckError(fmt.Errorf("couldnt convert to ecdsa private key"))
		}
		sv.private = pk
	}

	return sv, nil
}

// Sign creates a signature for `data`.
func (sv *ECDSASignerVerifier) Sign(data []byte) ([]byte, error) {
	if sv.private == nil {
		return nil, errorutils.CheckError(ErrNotPrivateKey)
	}

	hashedData := getECDSAHashedData(data, sv.curveSize)

	return ecdsa.SignASN1(rand.Reader, sv.private, hashedData)
}

// Verify verifies the `sig` value passed in against `data`.
func (sv *ECDSASignerVerifier) Verify(data []byte, sig []byte) error {
	hashedData := getECDSAHashedData(data, sv.curveSize)

	if ok := ecdsa.VerifyASN1(sv.public, hashedData, sig); !ok {
		return errorutils.CheckError(ErrSignatureVerificationFailed)
	}

	return nil
}

// KeyID returns the identifier of the key used to create the
// ECDSASignerVerifier instance.
func (sv *ECDSASignerVerifier) KeyID() (string, error) {
	return sv.keyID, nil
}

// Public returns the public portion of the key used to create the
// ECDSASignerVerifier instance.
func (sv *ECDSASignerVerifier) Public() crypto.PublicKey {
	return sv.public
}

func getECDSAHashedData(data []byte, curveSize int) []byte {
	switch {
	case curveSize <= 256:
		return hashBeforeSigning(data, sha256.New())
	case 256 < curveSize && curveSize <= 384:
		return hashBeforeSigning(data, sha512.New384())
	case curveSize > 384:
		return hashBeforeSigning(data, sha512.New())
	}
	return []byte{}
}
