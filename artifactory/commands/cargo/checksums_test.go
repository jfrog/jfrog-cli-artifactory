package cargo

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAql is a test double for aqlExecutor. It records each query issued and
// returns pre-configured JSON response bodies in order.
type fakeAql struct {
	queries   []string
	responses []string // one JSON body per call, in order
	err       error    // when set, every call returns this error
	errOnCall int      // when > 0, only the Nth call (1-based) returns f.err; earlier calls succeed
}

func (f *fakeAql) Aql(aql string) (io.ReadCloser, error) {
	f.queries = append(f.queries, aql)
	callNum := len(f.queries)
	if f.err != nil && (f.errOnCall == 0 || f.errOnCall == callNum) {
		return io.NopCloser(strings.NewReader("")), f.err
	}
	idx := callNum - 1
	body := `{"results":[]}`
	if idx < len(f.responses) {
		body = f.responses[idx]
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

// sampleAqlJSON is the VERIFIED AQL response shape from jfrog-client-go ResultItem json tags.
const sampleAqlJSON = `{"results":[{"repo":"cargo-remote-cache","path":"serde/1.0.197","name":"serde-1.0.197.crate","actual_sha1":"abc123","sha256":"def456","actual_md5":"ghi789"}]}`

func TestMissingChecksumNames(t *testing.T) {
	t.Run("nil BuildInfo returns empty", func(t *testing.T) {
		names := missingChecksumNames(nil)
		assert.Empty(t, names)
	})

	t.Run("returns names for any missing checksum field", func(t *testing.T) {
		// Any-field-missing detection (per Naveen's PR #510 review): a dep with only sha256
		// still needs enrichment for its sha1/md5, so it must appear in the missing list.
		// Only a dep with ALL three fields populated is complete.
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "fully-hashed-1.0.crate", Checksum: entities.Checksum{Sha1: "a", Sha256: "b", Md5: "c"}},
						{Id: "has-sha256-1.0.crate", Checksum: entities.Checksum{Sha256: "notempty"}},
						{Id: "empty-a-1.0.crate", Checksum: entities.Checksum{}},
						{Id: "empty-b-2.0.crate", Checksum: entities.Checksum{}},
					},
				},
			},
		}
		names := missingChecksumNames(bi)
		assert.ElementsMatch(t, []string{"has-sha256-1.0.crate", "empty-a-1.0.crate", "empty-b-2.0.crate"}, names)
	})

	t.Run("deduplicates repeated Id across modules", func(t *testing.T) {
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "dup-1.0.crate"},
					},
				},
				{
					Dependencies: []entities.Dependency{
						{Id: "dup-1.0.crate"},
					},
				},
			},
		}
		names := missingChecksumNames(bi)
		assert.Equal(t, 1, len(names))
		assert.Equal(t, "dup-1.0.crate", names[0])
	})

	t.Run("skips dep with empty Id", func(t *testing.T) {
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: ""},
					},
				},
			},
		}
		names := missingChecksumNames(bi)
		assert.Empty(t, names)
	})
}

func TestChunk(t *testing.T) {
	t.Run("5 names into pages of 2 gives [2,2,1]", func(t *testing.T) {
		names := []string{"a", "b", "c", "d", "e"}
		pages := chunk(names, 2)
		require.Len(t, pages, 3)
		assert.Equal(t, []string{"a", "b"}, pages[0])
		assert.Equal(t, []string{"c", "d"}, pages[1])
		assert.Equal(t, []string{"e"}, pages[2])
	})

	t.Run("nil input returns empty", func(t *testing.T) {
		pages := chunk(nil, 100)
		assert.Empty(t, pages)
	})

	t.Run("size 0 is treated as 1", func(t *testing.T) {
		names := []string{"x", "y", "z"}
		pages := chunk(names, 0)
		require.Len(t, pages, 3)
		for _, p := range pages {
			assert.Len(t, p, 1)
		}
	})
}

func TestBuildChecksumAql(t *testing.T) {
	result := buildChecksumAql("cargo-local", []string{"a-1.0.crate", "b-2.0.crate"})
	expected := `items.find({"repo":"cargo-local","$or":[{"name":"a-1.0.crate"},{"name":"b-2.0.crate"}]}).include("name","actual_sha1","sha256","actual_md5")`
	assert.Equal(t, expected, result)
}

func TestParseChecksumResults(t *testing.T) {
	t.Run("parses verified sample JSON correctly", func(t *testing.T) {
		r := strings.NewReader(sampleAqlJSON)
		m, err := parseChecksumResults(r)
		require.NoError(t, err)
		cs, ok := m["serde-1.0.197.crate"]
		require.True(t, ok, "expected key serde-1.0.197.crate in result map")
		assert.NotEmpty(t, cs.Sha256, "sha256 should be non-empty")
		assert.Equal(t, "abc123", cs.Sha1, "sha1 from actual_sha1")
		assert.Equal(t, "ghi789", cs.Md5, "md5 from actual_md5")
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		r := strings.NewReader("{not valid json}")
		_, err := parseChecksumResults(r)
		assert.Error(t, err)
	})
}

func TestApplyChecksums(t *testing.T) {
	t.Run("fills only missing checksums", func(t *testing.T) {
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "a-1.0.crate"},
						{Id: "b-2.0.crate"},
					},
				},
			},
		}
		byName := map[string]entities.Checksum{
			"a-1.0.crate": {Sha1: "sha1a", Sha256: "sha256a", Md5: "md5a"},
		}
		filled := applyChecksums(bi, byName)
		assert.Equal(t, 1, filled)
		assert.Equal(t, "sha1a", bi.Modules[0].Dependencies[0].Sha1)
		assert.Equal(t, "sha256a", bi.Modules[0].Dependencies[0].Sha256)
		assert.Empty(t, bi.Modules[0].Dependencies[1].Sha1)
		assert.Empty(t, bi.Modules[0].Dependencies[1].Sha256)
	})

	t.Run("does not overwrite existing checksum", func(t *testing.T) {
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "a-1.0.crate", Checksum: entities.Checksum{Sha256: "existing"}},
					},
				},
			},
		}
		byName := map[string]entities.Checksum{
			"a-1.0.crate": {Sha256: "new-value"},
		}
		filled := applyChecksums(bi, byName)
		assert.Equal(t, 0, filled)
		assert.Equal(t, "existing", bi.Modules[0].Dependencies[0].Sha256)
	})
}

func TestEnrichMissingChecksums_FillsFromFake(t *testing.T) {
	bi := &entities.BuildInfo{
		Modules: []entities.Module{
			{
				Dependencies: []entities.Dependency{
					{Id: "serde-1.0.197.crate"},
				},
			},
		},
	}
	fake := &fakeAql{
		responses: []string{sampleAqlJSON},
	}
	err := enrichMissingChecksums(bi, "cargo-remote-cache", fake)
	require.NoError(t, err)

	// Exactly one AQL query should have been issued
	assert.Len(t, fake.queries, 1)
	assert.Contains(t, fake.queries[0], "serde-1.0.197.crate")

	// The dependency should now have its checksum populated
	dep := bi.Modules[0].Dependencies[0]
	assert.NotEmpty(t, dep.Sha256, "sha256 should be filled from AQL response")
}

func TestEnrichMissingChecksums_AqlError(t *testing.T) {
	bi := &entities.BuildInfo{
		Modules: []entities.Module{
			{Dependencies: []entities.Dependency{{Id: "serde-1.0.197.crate"}}},
		},
	}
	fake := &fakeAql{err: fmt.Errorf("network down")}
	err := enrichMissingChecksums(bi, "cargo-remote-cache", fake)
	require.Error(t, err, "transport error should surface")
	assert.Contains(t, err.Error(), "network down")
	assert.Len(t, fake.queries, 1, "the single page should have been attempted")
	// No checksum could be filled.
	assert.Empty(t, bi.Modules[0].Dependencies[0].Sha256)
}

func TestEnrichMissingChecksums_AppliesPartialOnMidBatchError(t *testing.T) {
	// 150 missing crates -> 2 AQL pages (100 + 50). Page 1 succeeds and fills crate-0;
	// page 2 errors. The partial page-1 result must still be applied, and the error surfaced.
	deps := make([]entities.Dependency, 150)
	for i := range deps {
		deps[i] = entities.Dependency{Id: fmt.Sprintf("crate-%d.crate", i)}
	}
	bi := &entities.BuildInfo{Modules: []entities.Module{{Dependencies: deps}}}
	page1 := `{"results":[{"name":"crate-0.crate","actual_sha1":"s1","sha256":"s256","actual_md5":"m5"}]}`
	fake := &fakeAql{responses: []string{page1}, err: fmt.Errorf("timeout on page 2"), errOnCall: 2}

	err := enrichMissingChecksums(bi, "cargo-remote-cache", fake)
	require.Error(t, err, "mid-batch error should surface")
	assert.Contains(t, err.Error(), "timeout on page 2")
	assert.Len(t, fake.queries, 2, "both pages should have been attempted")
	// Partial result from page 1 must be applied despite the page-2 error.
	assert.Equal(t, "s256", bi.Modules[0].Dependencies[0].Sha256, "crate-0 should be filled from page 1")
}

func TestEnrichMissingChecksums_NoOps(t *testing.T) {
	t.Run("nil BuildInfo is no-op", func(t *testing.T) {
		fake := &fakeAql{}
		err := enrichMissingChecksums(nil, "cargo-remote-cache", fake)
		require.NoError(t, err)
		assert.Empty(t, fake.queries)
	})

	t.Run("empty repo is no-op", func(t *testing.T) {
		fake := &fakeAql{}
		bi := &entities.BuildInfo{Modules: []entities.Module{{Dependencies: []entities.Dependency{{Id: "a-1.0.crate"}}}}}
		err := enrichMissingChecksums(bi, "", fake)
		require.NoError(t, err)
		assert.Empty(t, fake.queries)
	})

	t.Run("nil executor is no-op", func(t *testing.T) {
		bi := &entities.BuildInfo{Modules: []entities.Module{{Dependencies: []entities.Dependency{{Id: "a-1.0.crate"}}}}}
		err := enrichMissingChecksums(bi, "cargo-remote-cache", nil)
		require.NoError(t, err)
	})

	t.Run("all deps fully hashed (sha1+sha256+md5) issues zero queries", func(t *testing.T) {
		// Per the corrected missingChecksumNames semantics: only a dep with EVERY hash field
		// populated is complete. A dep with sha256 alone would still count as missing sha1/md5.
		fake := &fakeAql{}
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "a-1.0.crate", Checksum: entities.Checksum{Sha1: "s1", Sha256: "s256", Md5: "m5"}},
					},
				},
			},
		}
		err := enrichMissingChecksums(bi, "cargo-remote-cache", fake)
		require.NoError(t, err)
		assert.Empty(t, fake.queries)
	})
}

func TestQueryChecksumsPaginates(t *testing.T) {
	// 150 missing names should result in 2 AQL queries (100 + 50)
	names := make([]string, 150)
	for i := range names {
		names[i] = fmt.Sprintf("crate-%d.crate", i)
	}

	// Two response bodies covering 5 crates each to verify merging
	resp1 := `{"results":[{"name":"crate-0.crate","actual_sha1":"sha1-0","sha256":"sha256-0","actual_md5":"md5-0"}]}`
	resp2 := `{"results":[{"name":"crate-100.crate","actual_sha1":"sha1-100","sha256":"sha256-100","actual_md5":"md5-100"}]}`

	fake := &fakeAql{
		responses: []string{resp1, resp2},
	}

	merged, err := queryChecksums(fake, "cargo-remote", names)
	require.NoError(t, err)

	// Should have issued exactly 2 queries
	assert.Len(t, fake.queries, 2)

	// Merged map should cover entries from both responses
	cs0, ok := merged["crate-0.crate"]
	assert.True(t, ok)
	assert.Equal(t, "sha256-0", cs0.Sha256)

	cs100, ok := merged["crate-100.crate"]
	assert.True(t, ok)
	assert.Equal(t, "sha256-100", cs100.Sha256)
}
