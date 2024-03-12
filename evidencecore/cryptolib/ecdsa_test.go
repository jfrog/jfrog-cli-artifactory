package cryptolib

import (
	"encoding/json"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestECDSASignerVerifierWithMetablockFileAndPEMKey(t *testing.T) {
	key, err := LoadKey(ecdsaPublicKey)
	assert.NoError(t, err)

	sv, err := NewECDSASignerVerifierFromSSLibKey(key)
	assert.NoError(t, err)

	metadataBytes, err := os.ReadFile(filepath.Join("testdata", "test-ecdsa.98adf386.link"))
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

	assert.NoError(t, json.Unmarshal(metadataBytes, &mb))

	assert.Equal(t, "304502201fbb03c0937504182a48c66f9218bdcb2e99a07ada273e92e5e543867f98c8d7022100dbfa7bbf74fd76d76c1d08676419cba85bbd81dfb000f3ac6a786693ddc508f5", mb.Signatures[0].Sig)
	assert.Equal(t, sv.keyID, mb.Signatures[0].KeyID)

	encodedBytes, err := cjson.EncodeCanonical(mb.Signed)
	if err != nil {
		t.Fatal(err)
	}

	decodedSig, err := hexDecode(t, mb.Signatures[0].Sig)
	assert.Nil(t, err)

	err = sv.Verify(encodedBytes, decodedSig)
	assert.Nil(t, err)
}
