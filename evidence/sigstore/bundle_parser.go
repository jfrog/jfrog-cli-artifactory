package sigstore

import (
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

// ParseBundle reads and validates a sigstore bundle file using sigstore-go
func ParseBundle(bundlePath string) (*bundle.Bundle, error) {
	// Use sigstore-go to load the bundle
	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to parse sigstore bundle: %s", err.Error())
	}

	return b, nil
}

// GetDSSEEnvelope extracts the DSSE envelope from the bundle using sigstore types
func GetDSSEEnvelope(b *bundle.Bundle) (*protodsse.Envelope, error) {
	// Get the protobuf bundle
	pb := b.Bundle

	// Check if bundle contains DSSE envelope
	content := pb.GetContent()
	if content == nil {
		return nil, errorutils.CheckErrorf("bundle does not contain content")
	}

	// Extract DSSE envelope based on content type
	switch c := content.(type) {
	case *protobundle.Bundle_DsseEnvelope:
		if c.DsseEnvelope == nil {
			return nil, errorutils.CheckErrorf("DSSE envelope is nil")
		}
		return c.DsseEnvelope, nil
	default:
		return nil, errorutils.CheckErrorf("bundle does not contain a DSSE envelope")
	}
}
