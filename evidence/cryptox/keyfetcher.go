package cryptox

import (
	"fmt"
	clientLog "github.com/jfrog/jfrog-client-go/utils/log"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/dsse"
	"github.com/jfrog/jfrog-client-go/artifactory"
)

// FetchTrustedKeys fetches public keys from the trusted keys endpoint.
func FetchTrustedKeys(client artifactory.ArtifactoryServicesManager) ([]dsse.Verifier, error) {
	clientLog.Debug("Fetching trusted keys")
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
	clientLog.Debug("Fetching trusted keys")
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
