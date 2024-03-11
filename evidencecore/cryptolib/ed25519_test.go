package cryptolib

import (
	"encoding/json"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestED25519SignerVerifierWithMetablockFileAndPEMKey(t *testing.T) {
	key, err := LoadKey(ed25519PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	sv, err := NewED25519SignerVerifierFromSSLibKey(key)
	if err != nil {
		t.Fatal(err)
	}

	metadataBytes, err := os.ReadFile(filepath.Join("test-data", "test-ed25519.52e3b8e7.link"))
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
	assert.Equal(t, sv.keyID, mb.Signatures[0].KeyID)

	encodedBytes, err := cjson.EncodeCanonical(mb.Signed)
	if err != nil {
		t.Fatal(err)
	}

	decodedSig := hexDecode(t, mb.Signatures[0].Sig)

	err = sv.Verify(encodedBytes, decodedSig)
	assert.Nil(t, err)
}
