package publish

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkillMeta(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: my-test-skill
description: A test skill for unit testing
version: 1.0.0
---

# My Test Skill

This is a test.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "my-test-skill", meta.Name)
	assert.Equal(t, "A test skill for unit testing", meta.Description)
	assert.Equal(t, "1.0.0", meta.Version)
}

func TestParseSkillMeta_MissingName(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
description: No name here
version: 1.0.0
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	_, err := ParseSkillMeta(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'name' field")
}

func TestParseSkillMeta_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# No frontmatter"), 0644))

	_, err := ParseSkillMeta(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "frontmatter delimiter")
}

func TestParseSkillMeta_FileNotFound(t *testing.T) {
	_, err := ParseSkillMeta("/nonexistent/dir")
	assert.Error(t, err)
}

func TestStripQuotes(t *testing.T) {
	assert.Equal(t, "1.0.0", stripQuotes(`"1.0.0"`))
	assert.Equal(t, "1.0.0", stripQuotes(`'1.0.0'`))
	assert.Equal(t, "no-quotes", stripQuotes("no-quotes"))
	assert.Equal(t, "", stripQuotes(`""`))
	assert.Equal(t, "", stripQuotes(`''`))
	assert.Equal(t, "a", stripQuotes("a"))
	assert.Equal(t, `"mismatched'`, stripQuotes(`"mismatched'`))
}

func TestParseSkillMeta_QuotedVersion(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: quoted-skill
description: "A skill with quoted values"
version: "2.3.4"
---

# Quoted Skill
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "quoted-skill", meta.Name)
	assert.Equal(t, "A skill with quoted values", meta.Description)
	assert.Equal(t, "2.3.4", meta.Version)
}

func TestParseSkillMeta_SingleQuotedVersion(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: single-quoted-skill
version: '1.5.0'
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "single-quoted-skill", meta.Name)
	assert.Equal(t, "1.5.0", meta.Version)
}

func TestParseSkillMeta_FoldedBlockDescription(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: my-skill
version: 1.0.0
description: >
  Generates a structured test plan for a new package type.
  Use this skill whenever a developer wants test coverage.
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "Generates a structured test plan for a new package type. Use this skill whenever a developer wants test coverage.", meta.Description)
}

func TestParseSkillMeta_LiteralBlockDescription(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: my-skill
version: 1.0.0
description: |-
  First line.
  Second line.
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "First line. Second line.", meta.Description)
}

func TestUpdateSkillMetaVersion_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: my-skill
description: A great skill
version: 1.0.0
---

# My Skill

Body content here.
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	err := UpdateSkillMetaVersion(dir, "2.0.0")
	require.NoError(t, err)

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", meta.Version)
	assert.Equal(t, "my-skill", meta.Name)
	assert.Equal(t, "A great skill", meta.Description)

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Body content here.")
}

func TestUpdateSkillMetaVersion_QuotedVersion(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: quoted-skill
version: "1.0.0"
---

# Quoted
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	err := UpdateSkillMetaVersion(dir, "3.0.0")
	require.NoError(t, err)

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", meta.Version)
}

func TestUpdateSkillMetaVersion_SingleQuotedVersion(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: sq-skill
version: '1.5.0'
---
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	err := UpdateSkillMetaVersion(dir, "1.6.0")
	require.NoError(t, err)

	meta, err := ParseSkillMeta(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.6.0", meta.Version)
}

func TestUpdateSkillMetaVersion_NoVersionField(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: no-version-skill
description: No version here
---

# Content
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	err := UpdateSkillMetaVersion(dir, "1.0.0")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, skillMD, string(data))
}

func TestUpdateSkillMetaVersion_PreservesBody(t *testing.T) {
	dir := t.TempDir()
	skillMD := `---
name: body-skill
version: 0.1.0
---

# My Skill

Some **markdown** content with [links](https://example.com).

` + "```python\nprint('hello')\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0644))

	err := UpdateSkillMetaVersion(dir, "0.2.0")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "version: 0.2.0")
	assert.Contains(t, content, "Some **markdown** content with [links](https://example.com).")
	assert.Contains(t, content, "print('hello')")
}

func TestValidateSlug(t *testing.T) {
	assert.NoError(t, ValidateSlug("my-skill"))
	assert.NoError(t, ValidateSlug("skill123"))
	assert.NoError(t, ValidateSlug("a"))
	assert.NoError(t, ValidateSlug("4chan-reader"))

	assert.Error(t, ValidateSlug("My-Skill"))
	assert.Error(t, ValidateSlug("-invalid"))
	assert.Error(t, ValidateSlug("has space"))
	assert.Error(t, ValidateSlug(""))
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		version string
		wantErr bool
	}{
		{"1.0.0", false},
		{"0.1.0", false},
		{"2.3.4-beta", false},
		{"1.0.0-rc.1", false},
		{"1.0.0+build.123", false},
		{"v1.0.0", false},

		{"", true},
		{"../etc/passwd", true},
		{"1.0.0/../../etc", true},
		{"1.0.0\\..\\etc", true},
		{"..", true},
		{"valid..version", true},
		{"has space", true},
		{"has\x00null", true},
		{"/leading-slash", true},
		{"-leading-hyphen", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if tt.wantErr {
				assert.Error(t, err, "expected error for version %q", tt.version)
			} else {
				assert.NoError(t, err, "expected no error for version %q", tt.version)
			}
		})
	}
}

func TestGeneratePredicateFile(t *testing.T) {
	dir := t.TempDir()
	path, err := GeneratePredicateFile(dir, "test-skill", "1.0.0")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var p predicate
	require.NoError(t, json.Unmarshal(data, &p))
	assert.Equal(t, "test-skill", p.Skill)
	assert.Equal(t, "1.0.0", p.Version)
	assert.NotEmpty(t, p.PublishedAt)
	assert.True(t, strings.HasSuffix(p.PublishedAt, "Z"))
}

func TestGenerateMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path, err := GenerateMarkdownFile(dir, "test-skill", "2.0.0")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "# Publish Attestation")
	assert.Contains(t, content, "| Skill | test-skill |")
	assert.Contains(t, content, "| Version | 2.0.0 |")
	assert.Contains(t, content, "| Published at |")
}

func TestZipSkillFolder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0644))

	subDir := filepath.Join(dir, "utils")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "helper.py"), []byte("pass"), 0644))

	// Create excludable files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.pyc"), []byte("compiled"), 0644))

	zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath) }()

	info, err := os.Stat(zipPath)
	require.NoError(t, err)
	assert.True(t, info.Size() > 0)
}

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello world"), 0644))

	hash, err := computeSHA256(testFile)
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	// SHA256 of "hello world"
	assert.Equal(t, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", hash)
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		isDir   bool
		exclude bool
	}{
		{"git dir", ".git", true, true},
		{"pycache", "__pycache__", true, true},
		{"node_modules", "node_modules", true, true},
		{"pyc file", "module.pyc", false, true},
		{"normal file", "main.py", false, false},
		{"ds store", ".DS_Store", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := fakeFileInfo{name: filepath.Base(tt.relPath), dir: tt.isDir}
			assert.Equal(t, tt.exclude, shouldExclude(tt.relPath, info))
		})
	}
}

func TestIsEvidenceLicenseError(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		isLicenseErr bool
	}{
		{
			name:         "403 Forbidden with Enterprise+ message",
			errMsg:       `upload failed for subject 'repo/skill/1.0.0/skill-1.0.0.zip': server response: 403 Forbidden\n{"errors":[{"message":"evidence deployment requires an Enterprise+ license"}]}`,
			isLicenseErr: true,
		},
		{
			name:         "Enterprise+ only",
			errMsg:       "evidence deployment requires an Enterprise+ license",
			isLicenseErr: true,
		},
		{
			name:         "403 Forbidden only",
			errMsg:       "server response: 403 Forbidden",
			isLicenseErr: false,
		},
		{
			name:         "network error",
			errMsg:       "connection refused",
			isLicenseErr: false,
		},
		{
			name:         "401 unauthorized",
			errMsg:       "server response: 401 Unauthorized",
			isLicenseErr: false,
		},
		{
			name:         "generic 403 without Forbidden",
			errMsg:       "got status 403",
			isLicenseErr: false,
		},
		{
			name:         "signing key error",
			errMsg:       "failed to read signing key: no such file",
			isLicenseErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errMsg)
			assert.Equal(t, tt.isLicenseErr, isEvidenceLicenseError(err), "for error: %s", tt.errMsg)
		})
	}
}

type fakeFileInfo struct {
	name string
	dir  bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }

// ── Deterministic zip tests ──

func createSkillDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: test\n---"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0644))
	sub := filepath.Join(dir, "utils")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "helper.py"), []byte("pass"), 0644))
	return dir
}

func zipAndHash(t *testing.T, dir string) string {
	t.Helper()
	zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	// Clean up both the zip file and its parent temp directory.
	defer func() { _ = os.RemoveAll(filepath.Dir(zipPath)) }()
	hash, err := computeSHA256(zipPath)
	require.NoError(t, err)
	return hash
}

func TestZipDeterministic_SameContentSameHash(t *testing.T) {
	dir := createSkillDir(t)
	hash1 := zipAndHash(t, dir)
	hash2 := zipAndHash(t, dir)
	assert.Equal(t, hash1, hash2, "zipping the same directory twice should produce identical checksums")
}

func TestZipDeterministic_ContentChangeDifferentHash(t *testing.T) {
	dir := createSkillDir(t)
	hash1 := zipAndHash(t, dir)

	// Change file content
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('changed')"), 0644))
	hash2 := zipAndHash(t, dir)

	assert.NotEqual(t, hash1, hash2, "different content should produce different checksums")
}

func TestZipDeterministic_AllEntriesShareMaxMtime(t *testing.T) {
	dir := createSkillDir(t)

	// Set different mtimes on files
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "SKILL.md"), past, past))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "main.py"), newest, newest))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "utils", "helper.py"), past, past))

	zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath) }()

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		assert.Equal(t, newest.Unix(), f.Modified.Unix(),
			"entry %s should have max mtime, got %v", f.Name, f.Modified)
	}
}

func TestZipDeterministic_ExtraFieldConsistent(t *testing.T) {
	dir := createSkillDir(t)

	// Zip twice and verify the Extra fields are identical across runs.
	// Go's zip writer adds a UT (Unix Timestamp) extra field via CreateHeader;
	// since our timestamps are uniform (max mtime), the extra bytes are deterministic.
	zipPath1, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath1) }()
	zipPath2, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath2) }()

	r1, err := zip.OpenReader(zipPath1)
	require.NoError(t, err)
	defer func() { _ = r1.Close() }()
	r2, err := zip.OpenReader(zipPath2)
	require.NoError(t, err)
	defer func() { _ = r2.Close() }()

	require.Equal(t, len(r1.File), len(r2.File))
	for i := range r1.File {
		assert.Equal(t, r1.File[i].Extra, r2.File[i].Extra,
			"Extra field for %s should be identical across runs", r1.File[i].Name)
	}
}

func TestZipDeterministic_PermissionsPreserved(t *testing.T) {
	dir := createSkillDir(t)

	if runtime.GOOS == "windows" {
		// Windows normalizes all files to 0644 since it doesn't support Unix permissions.
		zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
		require.NoError(t, err)
		defer func() { _ = os.Remove(zipPath) }()
		r, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer func() { _ = r.Close() }()
		for _, f := range r.File {
			if f.Name == "main.py" {
				assert.Equal(t, os.FileMode(0644), f.Mode().Perm(),
					"main.py should have default 0644 on Windows")
			}
		}
		return
	}

	require.NoError(t, os.Chmod(filepath.Join(dir, "main.py"), 0755))
	zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath) }()
	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		if f.Name == "main.py" {
			assert.Equal(t, os.FileMode(0755), f.Mode().Perm(),
				"main.py should preserve 0755 permission")
		}
	}
}

func TestZipDeterministic_PermissionChangeDifferentHash(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, chmod is a no-op so hashes stay the same.
		dir := createSkillDir(t)
		hash1 := zipAndHash(t, dir)
		_ = os.Chmod(filepath.Join(dir, "main.py"), 0755)
		hash2 := zipAndHash(t, dir)
		assert.Equal(t, hash1, hash2, "on Windows, chmod is a no-op so hashes should match")
		return
	}

	dir := createSkillDir(t)
	hash1 := zipAndHash(t, dir)
	require.NoError(t, os.Chmod(filepath.Join(dir, "main.py"), 0755))
	hash2 := zipAndHash(t, dir)
	assert.NotEqual(t, hash1, hash2, "permission change should produce different checksum")
}

func TestZipDeterministic_FilesSorted(t *testing.T) {
	dir := createSkillDir(t)
	// Add files that sort differently than filesystem order
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zzz.py"), []byte("last"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aaa.py"), []byte("first"), 0644))

	zipPath, err := zipSkillFolder(dir, "test", "1.0.0")
	require.NoError(t, err)
	defer func() { _ = os.Remove(zipPath) }()

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	assert.True(t, sort.StringsAreSorted(names), "zip entries should be sorted, got: %v", names)
}

func TestCollectFiles_ExcludesCorrectly(t *testing.T) {
	dir := createSkillDir(t)
	// Add excluded files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cached.pyc"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("x"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "__pycache__"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "__pycache__", "mod.pyc"), []byte("x"), 0644))

	files, _, err := collectFiles(dir)
	require.NoError(t, err)

	var names []string
	for _, f := range files {
		names = append(names, f.relPath)
		assert.NotContains(t, f.relPath, ".pyc", "should exclude .pyc files")
		assert.NotContains(t, f.relPath, ".DS_Store", "should exclude .DS_Store")
		assert.NotContains(t, f.relPath, "__pycache__", "should exclude __pycache__")
	}
	assert.Contains(t, names, "SKILL.md")
	assert.Contains(t, names, "main.py")
}

func TestCollectFiles_Sorted(t *testing.T) {
	dir := createSkillDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zzz.py"), []byte("z"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aaa.py"), []byte("a"), 0644))

	files, _, err := collectFiles(dir)
	require.NoError(t, err)
	for i := 1; i < len(files); i++ {
		assert.True(t, files[i-1].relPath < files[i].relPath,
			"files should be sorted: %s should come before %s", files[i-1].relPath, files[i].relPath)
	}
}
