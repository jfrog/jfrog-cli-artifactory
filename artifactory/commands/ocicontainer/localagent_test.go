package ocicontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// resolveAndVerifyManifest must accept the Artifactory manifest as-is when
// imageSha2 is empty (e.g. containerManager.Id() was short-circuited by the
// opt-in SkipDockerImageIdVerificationEnv on a push) and must adopt the
// manifest's Config.Digest into builder.imageSha2 so that downstream
// digestToLayer(imageSha2) lookups in buildinfo.go still resolve.
func TestResolveAndVerifyManifest_EmptyImageSha2_AdoptsFromManifest(t *testing.T) {
	const artifactoryDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	labib := &localAgentBuildInfoBuilder{
		buildInfoBuilder: &buildInfoBuilder{imageSha2: ""},
	}
	m := &manifest{Config: manifestConfig{Digest: artifactoryDigest}}

	ok := labib.resolveAndVerifyManifest(m)

	assert.True(t, ok, "verification should succeed when the local id was skipped")
	assert.Equal(t, artifactoryDigest, labib.buildInfoBuilder.imageSha2,
		"imageSha2 must be populated from the Artifactory manifest for downstream lookups")
}

// When imageSha2 is populated (verify path) and matches the Artifactory
// manifest, verification must succeed and imageSha2 must be left untouched.
func TestResolveAndVerifyManifest_MatchingDigest_ReturnsTrue(t *testing.T) {
	const digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	labib := &localAgentBuildInfoBuilder{
		buildInfoBuilder: &buildInfoBuilder{imageSha2: digest},
	}
	m := &manifest{Config: manifestConfig{Digest: digest}}

	ok := labib.resolveAndVerifyManifest(m)

	assert.True(t, ok)
	assert.Equal(t, digest, labib.buildInfoBuilder.imageSha2)
}

// When imageSha2 is populated and does NOT match the Artifactory manifest,
// the verify path must reject the manifest and leave imageSha2 alone.
func TestResolveAndVerifyManifest_MismatchingDigest_ReturnsFalse(t *testing.T) {
	const localDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	const remoteDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	labib := &localAgentBuildInfoBuilder{
		buildInfoBuilder: &buildInfoBuilder{imageSha2: localDigest},
	}
	m := &manifest{Config: manifestConfig{Digest: remoteDigest}}

	ok := labib.resolveAndVerifyManifest(m)

	assert.False(t, ok)
	assert.Equal(t, localDigest, labib.buildInfoBuilder.imageSha2,
		"imageSha2 must not be mutated when the manifest is rejected")
}
