package common

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	servicesutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusCodeError mimics jfrog-client-go response errors with HTTP status codes.
type statusCodeError struct{ code int }

func (e statusCodeError) Error() string   { return fmt.Sprintf("http status %d", e.code) }
func (e statusCodeError) StatusCode() int { return e.code }

var err404 = statusCodeError{code: 404}

// mockSkillsServicesManager mocks ListSkillVersions, SkillVersionExists, SkillExists, and FolderInfo for testing.
type mockSkillsServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	listSkillVersionsFunc  func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error)
	folderInfoFunc         func(path string) (*servicesutils.FolderInfo, error)
	skillVersionExistsFunc func(repoKey, slug, version string) (bool, error)
	skillExistsFunc        func(repoKey, slug string) (bool, error)
}

func (m *mockSkillsServicesManager) ListSkillVersions(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
	return m.listSkillVersionsFunc(repoKey, slug, limit, cursor)
}

func (m *mockSkillsServicesManager) SkillVersionExists(repoKey, slug, version string) (bool, error) {
	return m.skillVersionExistsFunc(repoKey, slug, version)
}

func (m *mockSkillsServicesManager) SkillExists(repoKey, slug string) (bool, error) {
	return m.skillExistsFunc(repoKey, slug)
}

func (m *mockSkillsServicesManager) FolderInfo(path string) (*servicesutils.FolderInfo, error) {
	if m.folderInfoFunc != nil {
		return m.folderInfoFunc(path)
	}
	return nil, nil
}

func TestListVersionsFromManager_SinglePage_OneCall(t *testing.T) {
	calls := 0
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			calls++
			assert.Equal(t, "my-repo", repoKey)
			assert.Equal(t, "my-skill", slug)
			assert.Equal(t, skillsAPIPageSize, limit)
			assert.Equal(t, "", cursor)
			return []services.SkillVersion{{Version: "1.0.1"}}, "", nil
		},
	}

	versions, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.NoError(t, err)
	assert.Equal(t, []services.SkillVersion{{Version: "1.0.1"}}, versions)
	assert.Equal(t, 1, calls, "a skill within the page limit must resolve in exactly one call")
}

func TestListVersionsFromManager_PaginatesUntilCursorEmpty(t *testing.T) {
	pages := [][]services.SkillVersion{
		{{Version: "1.0.3"}, {Version: "1.0.2"}},
		{{Version: "1.0.1"}},
	}
	nextCursors := []string{"1.0.2", ""}
	call := 0
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			if call == 0 {
				assert.Equal(t, "", cursor, "first call must start from the beginning")
			} else {
				assert.Equal(t, nextCursors[call-1], cursor, "must resume from the previous page's cursor")
			}
			page, next := pages[call], nextCursors[call]
			call++
			return page, next, nil
		},
	}

	versions, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.NoError(t, err)
	assert.Equal(t, []services.SkillVersion{{Version: "1.0.3"}, {Version: "1.0.2"}, {Version: "1.0.1"}}, versions)
	assert.Equal(t, 2, call, "must stop as soon as nextCursor is empty")
}

func TestListVersionsFromManager_StopsOnNonAdvancingCursor(t *testing.T) {
	// Guard against infinite loops on stuck cursors.
	calls := 0
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			calls++
			if calls > 3 {
				t.Fatalf("listVersionsFromManager did not stop on a non-advancing cursor")
			}
			return []services.SkillVersion{{Version: "1.0.1"}}, "stuck-cursor", nil
		},
	}

	_, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-advancing cursor")
	assert.LessOrEqual(t, calls, 2, "must stop as soon as the cursor repeats")
}

func TestListVersionsFromManager_NotFound_RepoMissing(t *testing.T) {
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			return nil, "", err404
		},
		folderInfoFunc: func(path string) (*servicesutils.FolderInfo, error) {
			assert.Equal(t, "my-repo", path)
			return nil, err404
		},
	}

	_, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository 'my-repo' not found")
}

func TestListVersionsFromManager_NotFound_SkillMissing(t *testing.T) {
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			return nil, "", err404
		},
		folderInfoFunc: func(path string) (*servicesutils.FolderInfo, error) {
			return &servicesutils.FolderInfo{}, nil
		},
	}

	_, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill 'my-skill' not found in repository 'my-repo'")
}

func TestListVersionsFromManager_NotFoundMidPagination_NotDisambiguated(t *testing.T) {
	// 404 mid-pagination should not trigger disambiguation (only first-page 404s do).
	call := 0
	folderInfoCalled := false
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			call++
			if call == 1 {
				return []services.SkillVersion{{Version: "1.0.2"}}, "1.0.2", nil
			}
			return nil, "", err404
		},
		folderInfoFunc: func(path string) (*servicesutils.FolderInfo, error) {
			folderInfoCalled = true
			return &servicesutils.FolderInfo{}, nil
		},
	}

	_, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "not found in repository")
	assert.False(t, folderInfoCalled, "disambiguation must only run on the first page's 404")
}

func TestListVersionsFromManager_NonNotFoundError_Propagated(t *testing.T) {
	boom := errors.New("network exploded")
	mock := &mockSkillsServicesManager{
		listSkillVersionsFunc: func(repoKey, slug string, limit int, cursor string) ([]services.SkillVersion, string, error) {
			return nil, "", boom
		},
	}

	_, err := listVersionsFromManager(mock, "my-repo", "my-skill")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestListVersions_ValidationErrors(t *testing.T) {
	t.Run("nil server details", func(t *testing.T) {
		_, err := ListVersions(nil, "repo", "slug")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server details are required")
	})

	t.Run("empty repo key", func(t *testing.T) {
		_, err := ListVersions(&config.ServerDetails{Url: "https://example.com/"}, "  ", "slug")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repository is required")
	})

	t.Run("empty slug", func(t *testing.T) {
		_, err := ListVersions(&config.ServerDetails{Url: "https://example.com/"}, "repo", "  ")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skill name is required")
	})
}

func TestVersionExistsFromManager(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		mock := &mockSkillsServicesManager{
			skillVersionExistsFunc: func(repoKey, slug, version string) (bool, error) { return true, nil },
		}
		exists, err := versionExistsFromManager(mock, "repo", "my-skill", "1.0.1")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockSkillsServicesManager{
			skillVersionExistsFunc: func(repoKey, slug, version string) (bool, error) { return false, nil },
		}
		exists, err := versionExistsFromManager(mock, "repo", "my-skill", "9.9.9")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("error propagated", func(t *testing.T) {
		boom := errors.New("network exploded")
		mock := &mockSkillsServicesManager{
			skillVersionExistsFunc: func(repoKey, slug, version string) (bool, error) { return false, boom },
		}
		_, err := versionExistsFromManager(mock, "repo", "my-skill", "1.0.1")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestVersionExists_ValidationErrors(t *testing.T) {
	t.Run("nil server details", func(t *testing.T) {
		_, err := VersionExists(nil, "repo", "slug", "1.0.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server details are required")
	})

	t.Run("empty repo key", func(t *testing.T) {
		_, err := VersionExists(&config.ServerDetails{Url: "https://example.com/"}, "  ", "slug", "1.0.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repository is required")
	})

	t.Run("empty slug", func(t *testing.T) {
		_, err := VersionExists(&config.ServerDetails{Url: "https://example.com/"}, "repo", "  ", "1.0.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skill name is required")
	})
}

func TestSkillExistsFromManager(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		mock := &mockSkillsServicesManager{
			skillExistsFunc: func(repoKey, slug string) (bool, error) { return true, nil },
		}
		exists, err := skillExistsFromManager(mock, "repo", "my-skill")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockSkillsServicesManager{
			skillExistsFunc: func(repoKey, slug string) (bool, error) { return false, nil },
		}
		exists, err := skillExistsFromManager(mock, "repo", "missing-skill")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("error propagated", func(t *testing.T) {
		boom := errors.New("network exploded")
		mock := &mockSkillsServicesManager{
			skillExistsFunc: func(repoKey, slug string) (bool, error) { return false, boom },
		}
		_, err := skillExistsFromManager(mock, "repo", "my-skill")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestSkillExists_ValidationErrors(t *testing.T) {
	t.Run("nil server details", func(t *testing.T) {
		_, err := SkillExists(nil, "repo", "slug")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server details are required")
	})

	t.Run("empty repo key", func(t *testing.T) {
		_, err := SkillExists(&config.ServerDetails{Url: "https://example.com/"}, "  ", "slug")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "repository is required")
	})

	t.Run("empty slug", func(t *testing.T) {
		_, err := SkillExists(&config.ServerDetails{Url: "https://example.com/"}, "repo", "  ")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "skill name is required")
	})
}
