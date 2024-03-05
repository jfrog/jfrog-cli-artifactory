package dsse

import (
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
)

type Envelope struct {
	Payload     string      `json:"payload"`
	PayloadType string      `json:"payloadType"`
	Signatures  []Signature `json:"signatures"`
}

type Signature struct {
	KeyId string `json:"keyid"`
	Sig   string `json:"sig"`
}

// type Verifier interface {
// 	Verify(pae, signature []byte) error
// }

type Erroneous struct {
	Error error
}

func (e Erroneous) Verify([]byte, []byte) error {
	return e.Error
}

type GetVerifier func(keyId string) Verifier

// Verify is a Go implementation of the DSSE verification protocol described in
// detail here: https://github.com/secure-systems-lab/dsse/blob/master/protocol.md
// Verify accepts a number of PublicKeys which should correspond to the signatures
// of the envelope.
func (e *Envelope) Verify(publicKeys ...Verifier) error {
	if len(publicKeys) != len(e.Signatures) {
		return errors.Errorf("envelope containes %d signatures, received %d keys", len(e.Signatures), len(publicKeys))
	}

	for i, publicKey := range publicKeys {
		signature := e.Signatures[i]
		decodedSig, err := base64.StdEncoding.DecodeString(signature.Sig)
		if err != nil {
			return errors.Wrap(err, "decode envelope signature")
		}
		pae := PAE(e.PayloadType, []byte(e.Payload))
		err = publicKey.Verify(pae, decodedSig)
		if err != nil {
			return errors.Wrap(err, "verify envelope signature")
		}
	}

	return nil
}

// PAE stands for "Pre-Authentication-Encoding"
func PAE(payloadType string, payload []byte) []byte {
	return []byte(fmt.Sprintf("DSSEv1 %d %s %d %s", len(payloadType), payloadType, len(payload), payload))
}
