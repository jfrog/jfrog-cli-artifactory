package cryptox

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"testing"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"golang.org/x/crypto/ssh"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
)

/*
Credits: Parts of this file were originally authored for in-toto-golang.
*/

var (
	// ErrNoPEMBlock gets triggered when there is no PEM block in the provided file
	ErrNoPEMBlock = errors.New("failed to decode the data as PEM block (are you sure this is a pem file?)")
	// ErrFailedPEMParsing gets returned when PKCS1, PKCS8 or PKIX key parsing fails
	ErrFailedPEMParsing = errors.New("failed parsing the PEM block: unsupported PEM type")
)

func calculateKeyID(k *SSLibKey) (string, error) {
	key := map[string]any{
		"keytype":               k.KeyType,
		"scheme":                k.Scheme,
		"keyid_hash_algorithms": k.KeyIDHashAlgorithms,
		"keyval": map[string]string{
			"public": k.KeyVal.Public,
		},
	}
	canonical, err := cjson.EncodeCanonical(key)
	if err != nil {
		return "", errorutils.CheckError(err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

/*
generatePEMBlock creates a PEM block from scratch via the keyBytes and the pemType.
If successful it returns a PEM block as []byte slice. This function should always
succeed, if keyBytes is empty the PEM block will have an empty byte block.
Therefore only header and footer will exist.
*/
func generatePEMBlock(keyBytes []byte, pemType string) []byte {
	// construct PEM block
	pemBlock := &pem.Block{
		Type:    pemType,
		Headers: nil,
		Bytes:   keyBytes,
	}
	return pem.EncodeToMemory(pemBlock)
}

/*
decodeAndParsePEM receives potential PEM bytes decodes them via pem.Decode
and pushes them to parseKey. If any error occurs during this process,
the function will return nil and an error (either ErrFailedPEMParsing
or ErrNoPEMBlock). On success it will return the decoded pemData, the
key object interface and nil as error. We need the decoded pemData,
because LoadKey relies on decoded pemData for operating system
interoperability.
*/
func decodeAndParsePEM(pemBytes []byte) (*pem.Block, any, error) {
	// Attempt to decode PEM block
	data, _ := pem.Decode(pemBytes)
	if data == nil {
		return nil, nil, errorutils.CheckError(ErrNoPEMBlock)
	}

	if data.Type == "OPENSSH PRIVATE KEY" {
		key, err := parseSSHKey(pemBytes)
		if err != nil {
			return nil, nil, errorutils.CheckError(ErrNoPEMBlock)
		}
		return data, key, nil
	}
	// Try to load private key, if this fails try to load key as public key
	key, err := parsePEMKey(data.Bytes)
	if err != nil {
		return nil, nil, errorutils.CheckError(err)
	}
	return data, key, nil
}

func parseSSHKey(keyBytes []byte) (any, error) {
	key, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		return nil, errorutils.CheckError(ErrNoPEMBlock)
	}
	switch k := key.(type) {
	case *ecdsa.PrivateKey, *ed25519.PrivateKey, *rsa.PrivateKey:
		return k, nil
	}
	return nil, errorutils.CheckErrorf("PEM parsing failed %w", ErrFailedPEMParsing)
}

/*
parseKey tries to parse a PEM []byte slice. Using the following standards
in the given order:

  - PKCS8
  - PKCS1
  - PKIX
  - ECDSA
  - SSH

On success it returns the parsed key and nil.
On failure it returns nil and the error ErrFailedPEMParsing
*/
func parsePEMKey(data []byte) (any, error) {
	key, err := x509.ParsePKCS8PrivateKey(data)
	if err == nil {
		return key, nil
	}
	key, err = x509.ParsePKCS1PrivateKey(data)
	if err == nil {
		return key, nil
	}
	key, err = x509.ParsePKIXPublicKey(data)
	if err == nil {
		return key, nil
	}
	key, err = x509.ParseECPrivateKey(data)
	if err == nil {
		return key, nil
	}

	key, err = parseSSHKey(data)
	if err == nil {
		return key, nil
	}

	return nil, errorutils.CheckErrorf("PEM parsing failed %w", ErrFailedPEMParsing)
}

func hashBeforeSigning(data []byte, h hash.Hash) []byte {
	h.Write(data)
	return h.Sum(nil)
}

func hexDecode(t *testing.T, data string) ([]byte, error) {
	t.Helper()
	b, err := hex.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// СreateVerifier creates dsse.Verifier(s) from an SSLibKey.
func CreateVerifier(publicKey *SSLibKey) ([]dsse.Verifier, error) {
	var verifiers []dsse.Verifier

	switch publicKey.KeyType {
	case ECDSAKeyType:
		ecdsaSinger, err := NewECDSASignerVerifierFromSSLibKey(publicKey)
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, ecdsaSinger)
	case RSAKeyType:
		rsaSinger, err := NewRSAPSSSignerVerifierFromSSLibKey(publicKey)
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, rsaSinger)
	case ED25519KeyType:
		ed25519Singer, err := NewED25519SignerVerifierFromSSLibKey(publicKey)
		if err != nil {
			return nil, err
		}
		verifiers = append(verifiers, ed25519Singer)
	default:
		return nil, errors.New("unsupported key type")
	}
	return verifiers, nil
}

func GenerateFingerprint(pub crypto.PublicKey) (string, error) {
	if pub == nil {
		return "", errorutils.CheckError(fmt.Errorf("public key not available"))
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)

	if err != nil {
		return "", errorutils.CheckError(fmt.Errorf("failed to marshal public key: %w", err))
	}

	sum256 := sha256.Sum256(pubBytes)
	return base64.StdEncoding.EncodeToString(sum256[:]), nil
}
