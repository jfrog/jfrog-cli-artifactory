package sigstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSubjectFromBundle_NilBundle(t *testing.T) {
	repoPath, sha256, err := ExtractSubjectFromBundle(nil)

	assert.Error(t, err)
	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
	assert.Contains(t, err.Error(), "bundle cannot be nil")
}

func TestExtractRepoPathFromStatement_NilStatement(t *testing.T) {
	repoPath, sha256 := extractRepoPathFromStatement(nil)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_EmptyStatement(t *testing.T) {
	statement := map[string]any{}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_NoSubject(t *testing.T) {
	statement := map[string]any{
		"other_field": "value",
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_EmptySubjectArray(t *testing.T) {
	statement := map[string]any{
		"subject": []any{},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_SubjectNotArray(t *testing.T) {
	statement := map[string]any{
		"subject": "not_an_array",
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_FirstSubjectNotMap(t *testing.T) {
	statement := map[string]any{
		"subject": []any{"not_a_map"},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_NoNameField(t *testing.T) {
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"other_field": "value",
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_NameNotString(t *testing.T) {
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": 123,
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_EmptyName(t *testing.T) {
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": "",
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Empty(t, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_ValidNameNoDigest(t *testing.T) {
	expectedName := "docker://nginx:latest"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_ValidNameWithDigest(t *testing.T) {
	expectedName := "docker://nginx:latest"
	expectedSHA256 := "sha256:1234567890abcdef"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
				"digest": map[string]any{
					"sha256": expectedSHA256,
				},
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Equal(t, expectedSHA256, sha256)
}

func TestExtractRepoPathFromStatement_DigestNotMap(t *testing.T) {
	expectedName := "docker://nginx:latest"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name":   expectedName,
				"digest": "not_a_map",
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_DigestNoSHA256(t *testing.T) {
	expectedName := "docker://nginx:latest"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
				"digest": map[string]any{
					"other_algorithm": "value",
				},
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_SHA256NotString(t *testing.T) {
	expectedName := "docker://nginx:latest"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
				"digest": map[string]any{
					"sha256": 123,
				},
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_EmptySHA256(t *testing.T) {
	expectedName := "docker://nginx:latest"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
				"digest": map[string]any{
					"sha256": "",
				},
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Empty(t, sha256)
}

func TestExtractRepoPathFromStatement_MultipleSubjects(t *testing.T) {
	// Should only use the first subject
	expectedName := "docker://nginx:latest"
	expectedSHA256 := "sha256:1234567890abcdef"
	statement := map[string]any{
		"subject": []any{
			map[string]any{
				"name": expectedName,
				"digest": map[string]any{
					"sha256": expectedSHA256,
				},
			},
			map[string]any{
				"name": "docker://other:latest",
				"digest": map[string]any{
					"sha256": "sha256:abcdef1234567890",
				},
			},
		},
	}
	repoPath, sha256 := extractRepoPathFromStatement(statement)

	assert.Equal(t, expectedName, repoPath)
	assert.Equal(t, expectedSHA256, sha256)
}
