package sign

import (
	"encoding/base64"
	"errors"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// ErrNoSigners indicates that no signer was provided.
var ErrNoSigners = errors.New("no signers provided")

// EnvelopeSigner creates signed Envelopes.
type EnvelopeSigner struct {
	providers []dsse.Signer
}

/*
NewEnvelopeSigner creates an EnvelopeSigner that uses 1+ Signer algorithms to
sign the data.
*/
func NewEnvelopeSigner(singer ...dsse.Signer) (*EnvelopeSigner, error) {
	var providers []dsse.Signer

	for _, s := range singer {
		if s != nil {
			providers = append(providers, s)
		}
	}

	if len(providers) == 0 {
		return nil, errorutils.CheckError(ErrNoSigners)
	}

	return &EnvelopeSigner{
		providers: providers,
	}, nil
}

/*
SignPayload signs a payload and payload type according to DSSE.
Returned is an envelope as defined here:
https://github.com/secure-systems-lab/dsse/blob/master/envelope.md
One signature will be added for each Signer in the EnvelopeSigner.
*/
func (es *EnvelopeSigner) SignPayload(payloadType string, body []byte) (*dsse.Envelope, error) {
	var e = dsse.Envelope{
		Payload:     base64.StdEncoding.EncodeToString(body),
		PayloadType: payloadType,
	}

	paeEnc := dsse.PAE(payloadType, body)

	for _, signer := range es.providers {
		sig, err := signer.Sign(paeEnc)
		if err != nil {
			return nil, err
		}
		keyID, err := signer.KeyID()
		if err != nil {
			keyID = ""
		}

		e.Signatures = append(e.Signatures, dsse.Signature{
			KeyId: keyID,
			Sig:   base64.StdEncoding.EncodeToString(sig),
		})
	}

	return &e, nil
}
