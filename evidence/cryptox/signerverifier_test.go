package cryptox

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed testdata/rsa-test-key
var rsaPrivateKey []byte

//go:embed testdata/rsa-test-key-pkcs8
var rsaPrivateKeyPKCS8 []byte

//go:embed testdata/rsa-test-key.pub
var rsaPublicKey []byte

//go:embed testdata/ed25519-test-key-pem
var ed25519PrivateKey []byte

//go:embed testdata/ed25519-test-key-pem.pub
var ed25519PublicKey []byte

//go:embed testdata/ecdsa-test-key-pem
var ecdsaPrivateKey []byte

//go:embed testdata/ecdsa-test-key-pem.pub
var ecdsaPublicKey []byte

//go:embed testdata/ssh-rsa-2048
var sshRSAPrivateKey []byte

//go:embed testdata/ssh-ecdsa-256
var sshEcdsaPrivateKey []byte

//go:embed testdata/ssh-ed25519-256
var sshEd25519PrivateKey []byte

func TestLoadKey(t *testing.T) {
	// RSA expected values
	expectedRSAPrivateKey := strings.TrimSpace(strings.ReplaceAll(string(rsaPrivateKey), "\r\n", "\n"))
	expectedRSAPrivateKeyPKCS8 := strings.TrimSpace(strings.ReplaceAll(string(rsaPrivateKeyPKCS8), "\r\n", "\n"))
	expectedRSAPublicKey := strings.TrimSpace(strings.ReplaceAll(string(rsaPublicKey), "\r\n", "\n"))

	// ED25519 expected values
	//#nosec G101 - dummy key for test
	expectedED25519PrivateKey := "66f6ebad4aeb949b91c84c9cfd6ee351fc4fd544744bab6e30fb400ba13c6e9a3f586ce67329419fb0081bd995914e866a7205da463d593b3b490eab2b27fd3f"
	//#nosec G101 - dummy key for test
	expectedED25519PublicKey := "3f586ce67329419fb0081bd995914e866a7205da463d593b3b490eab2b27fd3f"

	// ECDSA expected values
	expectedECDSAPrivateKey := strings.TrimSpace(strings.ReplaceAll(string(ecdsaPrivateKey), "\r\n", "\n"))
	expectedECDSAPublicKey := strings.TrimSpace(strings.ReplaceAll(string(ecdsaPublicKey), "\r\n", "\n"))

	// SSH private key expected values
	expectedSSHECDSAPrivateKey := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(sshEcdsaPrivateKey), "\r\n", "\n"), "\n", ""))
	expectedSSHED25519PrivateKey := "14d62acc5eabc0a430bb5eedfef691e0a8f57e03a7e618c7c980f89452ea231535f2ba08016fc052241cdf87ea0d632f777e99cb562bdf199317d06eb98781f2"
	expectedSSHRSAPrivateKey := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(sshRSAPrivateKey), "\r\n", "\n"), "\n", ""))

	tests := map[string]struct {
		keyBytes           []byte
		expectedPrivateKey string
		expectedPublicKey  string
		expectedKeyType    string
		expectedScheme     string
		removeNewLines     bool
	}{
		"RSA private key": {
			keyBytes:           rsaPrivateKey,
			expectedPrivateKey: expectedRSAPrivateKey,
			expectedPublicKey:  expectedRSAPublicKey,
			expectedKeyType:    RSAKeyType,
			expectedScheme:     RSAKeyScheme,
		},
		"RSA private key (PKCS8)": {
			keyBytes:           rsaPrivateKeyPKCS8,
			expectedPrivateKey: expectedRSAPrivateKeyPKCS8,
			expectedPublicKey:  expectedRSAPublicKey,
			expectedKeyType:    RSAKeyType,
			expectedScheme:     RSAKeyScheme,
		},
		"RSA public key": {
			keyBytes:           rsaPublicKey,
			expectedPrivateKey: "",
			expectedPublicKey:  expectedRSAPublicKey,
			expectedKeyType:    RSAKeyType,
			expectedScheme:     RSAKeyScheme,
		},
		"ED25519 private key": {
			keyBytes:           ed25519PrivateKey,
			expectedPrivateKey: expectedED25519PrivateKey,
			expectedPublicKey:  expectedED25519PublicKey,
			expectedKeyType:    ED25519KeyType,
			expectedScheme:     ED25519KeyType,
		},
		"ED25519 public key": {
			keyBytes:           ed25519PublicKey,
			expectedPrivateKey: "",
			expectedPublicKey:  expectedED25519PublicKey,
			expectedKeyType:    ED25519KeyType,
			expectedScheme:     ED25519KeyType,
		},
		"ECDSA private key": {
			keyBytes:           ecdsaPrivateKey,
			expectedPrivateKey: expectedECDSAPrivateKey,
			expectedPublicKey:  expectedECDSAPublicKey,
			expectedKeyType:    ECDSAKeyType,
			expectedScheme:     ECDSAKeyScheme,
		},
		"ECDSA public key": {
			keyBytes:           ecdsaPublicKey,
			expectedPrivateKey: "",
			expectedPublicKey:  expectedECDSAPublicKey,
			expectedKeyType:    ECDSAKeyType,
			expectedScheme:     ECDSAKeyScheme,
		},
		"SSH ECDSA private key": {
			keyBytes:           sshEcdsaPrivateKey,
			expectedPrivateKey: expectedSSHECDSAPrivateKey,
			expectedPublicKey:  "",
			expectedKeyType:    ECDSAKeyType,
			expectedScheme:     ECDSAKeyScheme,
			removeNewLines:     true,
		},
		"SSH ED25519 private key": {
			keyBytes:           sshEd25519PrivateKey,
			expectedPrivateKey: expectedSSHED25519PrivateKey,
			expectedPublicKey:  "",
			expectedKeyType:    ED25519KeyType,
			expectedScheme:     ED25519KeyType,
		},
		"SSH RSA private key": {
			keyBytes:           sshRSAPrivateKey,
			expectedPrivateKey: expectedSSHRSAPrivateKey,
			expectedPublicKey:  "",
			expectedKeyType:    RSAKeyType,
			expectedScheme:     RSAKeyScheme,
			removeNewLines:     true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			key, err := LoadKey(test.keyBytes)
			assert.Nil(t, err, fmt.Sprintf("unexpected error in test '%s'", name))
			if test.expectedPublicKey != "" {
				assert.Equal(t, test.expectedPublicKey, key.KeyVal.Public)
			}
			if test.removeNewLines {
				assert.Equal(t, test.expectedPrivateKey, strings.ReplaceAll(key.KeyVal.Private, "\n", ""))
			} else {
				assert.Equal(t, test.expectedPrivateKey, key.KeyVal.Private)
			}
			assert.Equal(t, test.expectedScheme, key.Scheme)
			assert.Equal(t, test.expectedKeyType, key.KeyType)
		})
	}
}
