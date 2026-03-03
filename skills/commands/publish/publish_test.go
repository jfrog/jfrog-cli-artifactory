package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	defer os.Remove(zipPath)

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

func TestEscapePropertyValue(t *testing.T) {
	assert.Equal(t, "simple", escapePropertyValue("simple"))
	assert.Equal(t, `a\;b`, escapePropertyValue("a;b"))
	assert.Equal(t, `x\=y`, escapePropertyValue("x=y"))
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
