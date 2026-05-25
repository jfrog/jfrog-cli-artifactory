package common

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
)

func TestEnsureServiceUrls_UsesServerDetailsPlatformUrl(t *testing.T) {
	sd := &config.ServerDetails{
		ArtifactoryUrl: "https://acme.jfrog.io/artifactory/",
	}
	ensureServiceUrls(sd)
	assert.Equal(t, "https://acme.jfrog.io/", sd.GetUrl())
	assert.Equal(t, "https://acme.jfrog.io/evidence/", sd.GetEvidenceUrl())
	assert.Equal(t, "https://acme.jfrog.io/onemodel/", sd.GetOnemodelUrl())
}

func TestEnsureServiceUrls_RespectsConfiguredPlatformUrl(t *testing.T) {
	sd := &config.ServerDetails{
		Url:         "https://acme.jfrog.io/",
		EvidenceUrl: "https://acme.jfrog.io/custom-evidence/",
	}
	ensureServiceUrls(sd)
	assert.Equal(t, "https://acme.jfrog.io/custom-evidence/", sd.GetEvidenceUrl())
	assert.Equal(t, "https://acme.jfrog.io/onemodel/", sd.GetOnemodelUrl())
}
