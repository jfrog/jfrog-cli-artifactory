package dsse

import (
	"github.com/stretchr/testify/assert"
	"slices"
	"testing"
)

func TestVerify(t *testing.T) {
	env := &Envelope{
		Payload:     "payload",
		PayloadType: "payloadType",
		Signatures:  nil,
	}
	err := env.Verify()
	assert.NoError(t, err)
}

func TestPAE(t *testing.T) {
	result := PAE("payloadType", []byte("payload"))

	expectedResult := []byte("DSSEv1 11 payloadType 7 payload")
	res := slices.Equal(result, expectedResult)
	assert.True(t, res)
}
