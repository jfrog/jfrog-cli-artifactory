package apmcommon

import (
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
)

func TestSelectCachedAndUncached(t *testing.T) {
	deps := []ResolvedDep{
		{ID: "a/b:1.0.0"},
		{ID: "c/d:2.0.0"},
		{ID: "e/f:3.0.0"},
	}
	cachedChecksums := map[string]entities.Checksum{
		"a/b:1.0.0": {Sha256: "cached-sha256"},
	}

	cached, uncached := selectCachedAndUncached(deps, cachedChecksums)

	assert.Equal(t, map[string]entities.Checksum{"a/b:1.0.0": {Sha256: "cached-sha256"}}, cached)
	assert.Equal(t, []ResolvedDep{{ID: "c/d:2.0.0"}, {ID: "e/f:3.0.0"}}, uncached)
}

func TestSelectCachedAndUncached_NoneCached(t *testing.T) {
	deps := []ResolvedDep{{ID: "a/b:1.0.0"}, {ID: "c/d:2.0.0"}}

	cached, uncached := selectCachedAndUncached(deps, map[string]entities.Checksum{})

	assert.Empty(t, cached)
	assert.Equal(t, deps, uncached)
}

func TestSelectCachedAndUncached_AllCached(t *testing.T) {
	deps := []ResolvedDep{{ID: "a/b:1.0.0"}, {ID: "c/d:2.0.0"}}
	cachedChecksums := map[string]entities.Checksum{
		"a/b:1.0.0": {Sha256: "s1"},
		"c/d:2.0.0": {Sha256: "s2"},
	}

	cached, uncached := selectCachedAndUncached(deps, cachedChecksums)

	assert.Len(t, cached, 2)
	assert.Empty(t, uncached)
}

func TestApplyHeadResultsOrLockfileFallback_HeadHit(t *testing.T) {
	uncached := []ResolvedDep{{ID: "a/b:1.0.0", SHA256: "lockfile-sha256"}}
	headResults := map[string]entities.Checksum{
		"a/b:1.0.0": {Sha1: "head-sha1", Sha256: "head-sha256", Md5: "head-md5"},
	}

	resolved := applyHeadResultsOrLockfileFallback(uncached, headResults)

	// HEAD result wins outright over the lockfile's own SHA-256 when both are available.
	assert.Equal(t, entities.Checksum{Sha1: "head-sha1", Sha256: "head-sha256", Md5: "head-md5"}, resolved["a/b:1.0.0"])
}

func TestApplyHeadResultsOrLockfileFallback_FallsBackToLockfileSHA256(t *testing.T) {
	uncached := []ResolvedDep{{ID: "a/b:1.0.0", SHA256: "lockfile-sha256"}}

	resolved := applyHeadResultsOrLockfileFallback(uncached, map[string]entities.Checksum{})

	// No HEAD result at all -> lockfile SHA-256 only, sha1/md5 stay empty.
	assert.Equal(t, entities.Checksum{Sha256: "lockfile-sha256"}, resolved["a/b:1.0.0"])
}

func TestApplyHeadResultsOrLockfileFallback_NoChecksumAtAll(t *testing.T) {
	uncached := []ResolvedDep{{ID: "a/b:1.0.0"}} // no SHA256 from the lockfile either

	resolved := applyHeadResultsOrLockfileFallback(uncached, map[string]entities.Checksum{})

	// Neither tier has anything - dependency is simply omitted, not recorded with a zero-value checksum.
	_, found := resolved["a/b:1.0.0"]
	assert.False(t, found)
}
