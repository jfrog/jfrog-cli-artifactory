package cryptox

import (
	"encoding/base64"
	"encoding/json"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestED25519SignerVerifierWithMetablockFileAndPEMKey(t *testing.T) {
	key, err := LoadKey(ed25519PublicKey)
	assert.NoError(t, err)

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)

	metadataBytes, err := os.ReadFile(filepath.Join("testdata", "test-ed25519.52e3b8e7.link"))
	if err != nil {
		t.Fatal(err)
	}

	mb := struct {
		Signatures []struct {
			KeyID string `json:"keyid"`
			Sig   string `json:"sig"`
		} `json:"signatures"`
		Signed any `json:"signed"`
	}{}

	if err := json.Unmarshal(metadataBytes, &mb); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "4c8b7605a9195d4ddba54493bbb5257a9836c1d16056a027fd77e97b95a4f3e36f8bc3c9c9960387d68187760b3072a30c44f992c5bf8f7497c303a3b0a32403", mb.Signatures[0].Sig)

	encodedBytes, err := cjson.EncodeCanonical(mb.Signed)
	if err != nil {
		t.Fatal(err)
	}

	decodedSig, err := hexDecode(t, mb.Signatures[0].Sig)
	assert.Nil(t, err)

	err = sv.Verify(encodedBytes, decodedSig)
	assert.Nil(t, err)
}

func TestSignED25519(t *testing.T) {
	key, err := LoadKey(ed25519PrivateKey)
	assert.NoError(t, err)

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)

	signed, err := sv.Sign([]byte("data"))
	assert.NoError(t, err)
	assert.NotNil(t, signed)
}

func TestVerifyED25519(t *testing.T) {
	key, err := LoadKey(ed25519PublicKey)
	assert.NoError(t, err)

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)

	sig := "lMUNogZzwUQwo1FX7mv00H61rgKvVXwyLDBlLsfjgj0YS9LVVp7kMO+VbEOEvVTA3w5yPDVfwBqLyXfYmFFXCw=="
	decodedSig, _ := base64.StdEncoding.DecodeString(sig)
	err = sv.Verify([]byte("data"), decodedSig)
	assert.NoError(t, err)
}

func TestKeyIDED25519(t *testing.T) {
	key, err := LoadKey(ed25519PublicKey)
	assert.NoError(t, err)

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)
	id, err := sv.KeyID()
	assert.NoError(t, err)
	assert.NotNil(t, id)
}

func TestPublicED25519(t *testing.T) {
	key, err := LoadKey(ed25519PublicKey)
	assert.NoError(t, err)

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)

	pk := sv.Public()
	assert.NotNil(t, pk)
}
