package cryptolib

import (
	"encoding/json"
	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
)

func TestRsaSignerVerifierWithMetablockFileAndPEMKey(t *testing.T) {
	key, err := LoadKey(rsaPublicKey)
	if err != nil {
		t.Fatal(err)
	}

	sv, err := NewRSAPSSSignerVerifierFromSSLibKey(key)
	if err != nil {
		t.Fatal(err)
	}

	metadataBytes, err := os.ReadFile(filepath.Join("testdata", "test-rsa.4e8d20af.link"))
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

	assert.Equal(t, "8958e5be66ee4352880a531bd097d1727adcc78e66b4faeb4a2cd6ad073dcb84f9a34e8156af39a7144cb5cd925325a18ccd4f0b2f981d6ff82655a7d63210d36655c50a0bf24e4839c10430a040dd6189d04fabec90eae4314c75ae2d585da17a56aaf6755e613a3a6a471ad2eddbb24504848e34f9ac163660f8ab80d7701bfa1189578a59597b3809ee62a70a7cc9545cfa65e23018fa442a45279b9fcf9d80bc92df711bfcfe16e3eae1bcf61b3286c1f0bdda17bc28bfab5b736bdcac4a38e31db1d0e0f56a2853b1b451650305f040a3425c3be47125700e92ef82c5a91a040b5e70ab7f6ebbe037ae1a6835044b5699748037e2e39a55a420c41cd9fa6e16868776367e3620e7d28eb9d8a3d710bdc98d488df1a9947d2ec8400f3c6209e8ca587cbffa30ceb3be98105e03182aab1bbb3c4e2560d99f0b09c012df2271f273ac70a6abb185abe11d559b118dca616417fa9205e74ab58e89ffd8b965da304ae9dc9cf6ffac4838b7c5375d6c2057a61cb286f06ad3b02a49c3af6178", mb.Signatures[0].Sig)
	assert.Equal(t, sv.keyID, mb.Signatures[0].KeyID)

	_, err = cjson.EncodeCanonical(mb.Signed)
	assert.Nil(t, err)

}
