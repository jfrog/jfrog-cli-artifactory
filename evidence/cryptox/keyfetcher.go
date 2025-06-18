package cryptox

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

type trustedKeysResponse struct {
	Keys []struct {
		PublicKey string `json:"key"`
	} `json:"keys"`
}

type keypairResponseItem struct {
	PublicKey string `json:"publicKey"`
}

// FetchTrustedKeys fetches public keys from the trusted keys endpoint.
func FetchTrustedKeys(client artifactory.ArtifactoryServicesManager) ([]dsse.Verifier, error) {
	response, err := client.GetTrustedKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trusted keys: %v", err)
	}
	verifiers := []dsse.Verifier{}
	for _, k := range response.Keys {
		key, _ := LoadKey([]byte(k.PublicKey))
		if key == nil {
			continue
		}
		v, _ := createVerifier(key)
		verifiers = append(verifiers, v...)
	}
	return verifiers, nil
}

// FetchKeyPairs fetches public keys from the keypair endpoint.
func FetchKeyPairs(client artifactory.ArtifactoryServicesManager) ([]dsse.Verifier, error) {
	keyPairs, err := client.GetKeyPairs()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch key pairs: %v", err)
	}
	verifiers := []dsse.Verifier{}
	for _, k := range *keyPairs {
		key, _ := LoadKey([]byte(k.PublicKey))
		if key == nil {
			continue
		}
		v, _ := createVerifier(key)
		verifiers = append(verifiers, v...)
	}
	return verifiers, nil
}
