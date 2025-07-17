package sigstore

import (
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

func ParseBundle(bundlePath string) (*bundle.Bundle, error) {
	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to parse sigstore bundle: %s", err.Error())
	}

	return b, nil
}

func GetDSSEEnvelope(b *bundle.Bundle) (*protodsse.Envelope, error) {
	pb := b.Bundle

	content := pb.GetContent()
	if content == nil {
		return nil, errorutils.CheckErrorf("bundle does not contain content")
	}

	switch c := content.(type) {
	case *protobundle.Bundle_DsseEnvelope:
		if c.DsseEnvelope == nil {
			return nil, errorutils.CheckErrorf("DSSE envelope is empty")
		}
		return c.DsseEnvelope, nil
	default:
		return nil, errorutils.CheckErrorf("bundle does not contain a DSSE envelope")
	}
}
