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

func TestHasAnyChecksum(t *testing.T) {
	tests := []struct {
		name     string
		checksum entities.Checksum
		expected bool
	}{
		{"All empty", entities.Checksum{}, false},
		{"SHA1 only", entities.Checksum{Sha1: "abc"}, true},
		{"SHA256 only", entities.Checksum{Sha256: "def"}, true},
		{"MD5 only", entities.Checksum{Md5: "ghi"}, true},
		{"All present", entities.Checksum{Sha1: "a", Sha256: "b", Md5: "c"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasAnyChecksum(tt.checksum))
		})
	}
}
