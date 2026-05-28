package common

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnzipFile(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"SKILL.md":        "---\nname: test\n---",
		"main.py":         "print('hello')",
		"utils/helper.py": "pass",
	})

	err := UnzipFile(zipPath, destDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(destDir, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: test")

	data, err = os.ReadFile(filepath.Join(destDir, "main.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('hello')", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "utils", "helper.py"))
	require.NoError(t, err)
	assert.Equal(t, "pass", string(data))
}

func TestUnzipFile_RejectsPathTraversal(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()

	zipPath := filepath.Join(srcDir, "evil.zip")
	createTestZip(t, zipPath, map[string]string{
		"../../outside.txt": "escaped",
	})

	err := UnzipFile(zipPath, destDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "illegal file path in zip")
	assert.Contains(t, err.Error(), "../../outside.txt")

	_, err = os.Stat(filepath.Join(filepath.Dir(destDir), "outside.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
	subDir := filepath.Join(srcDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644))

	err := CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content1", string(data))

	data, err = os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content2", string(data))
}

func TestEnsureDestinationDir_CreatesUnderExistingParent(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "install-x")
	require.NoError(t, EnsureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_CreatesNestedPath(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, ".cursor", "plugins", "alpha")
	require.NoError(t, EnsureDestinationDir(dest))
	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDestinationDir_RejectsFileAtDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(dest, []byte("hi"), 0o644))
	err := EnsureDestinationDir(dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestWriteReadInstallInfoManifest(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "my-package")
	require.NoError(t, os.MkdirAll(installDir, InstallDirMode))

	manifest := InstallInfoManifest{
		Repo:             "local",
		Slug:             "my-package",
		InstalledVersion: "1.0.3",
		Scope:            "project",
		Agent:            "cursor",
	}
	require.NoError(t, WriteInstallInfoManifest(installDir, "package-info.json", manifest))

	got, err := ReadInstallInfoManifest(installDir, "package-info.json")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, InstallInfoManifestSchemaVersion, got.SchemaVersion)
	assert.Equal(t, "my-package", got.Slug)
}

func TestVerifyPackageEvidence_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		repoKey string
		slug    string
		version string
	}{
		{name: "missing repo", slug: "my-skill", version: "1.0.0"},
		{name: "missing slug", repoKey: "local", version: "1.0.0"},
		{name: "missing version", repoKey: "local", slug: "my-skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPackageEvidence(&config.ServerDetails{}, tt.repoKey, tt.slug, tt.version)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot verify evidence")
		})
	}
}

func TestVerifyPackageEvidence_DelegatesSubjectPath(t *testing.T) {
	restore := verifyEvidenceForPackageZip
	verifyEvidenceForPackageZip = func(_ *config.ServerDetails, opts VerifyEvidenceOpts) error {
		assert.Equal(t, "skills-local/my-skill/2.0.0/my-skill-2.0.0.zip", opts.SubjectRepoPath)
		return nil
	}
	t.Cleanup(func() { verifyEvidenceForPackageZip = restore })

	err := VerifyPackageEvidence(&config.ServerDetails{}, "skills-local", "my-skill", "2.0.0")
	require.NoError(t, err)
}

type fakePackageZipDownloader struct {
	downloaded int
	failed     int
	err        error
}

func (f *fakePackageZipDownloader) DownloadFiles(_ ...services.DownloadParams) (int, int, error) {
	return f.downloaded, f.failed, f.err
}

func TestDownloadPackageZip_CreateServiceManagerError(t *testing.T) {
	restore := createDownloadServiceManagerForPackageZip
	createDownloadServiceManagerForPackageZip = func(_ *config.ServerDetails) (packageZipDownloadManager, error) {
		return nil, errors.New("auth failed")
	}
	t.Cleanup(func() { createDownloadServiceManagerForPackageZip = restore })

	_, err := DownloadPackageZip(&config.ServerDetails{}, "local", "my-skill", "1.0.0", t.TempDir(), "skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create download service manager")
	assert.Contains(t, err.Error(), "auth failed")
}

func TestDownloadPackageZip_NotFound(t *testing.T) {
	restore := createDownloadServiceManagerForPackageZip
	createDownloadServiceManagerForPackageZip = func(_ *config.ServerDetails) (packageZipDownloadManager, error) {
		return &fakePackageZipDownloader{}, nil
	}
	t.Cleanup(func() { createDownloadServiceManagerForPackageZip = restore })

	_, err := DownloadPackageZip(&config.ServerDetails{}, "local", "my-skill", "1.0.0", t.TempDir(), "skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill 'my-skill' version '1.0.0' not found in repository 'local'")
}

func TestDownloadPackageZip_Success(t *testing.T) {
	restore := createDownloadServiceManagerForPackageZip
	createDownloadServiceManagerForPackageZip = func(_ *config.ServerDetails) (packageZipDownloadManager, error) {
		return &fakePackageZipDownloader{downloaded: 1}, nil
	}
	t.Cleanup(func() { createDownloadServiceManagerForPackageZip = restore })

	tmpDir := t.TempDir()
	got, err := DownloadPackageZip(&config.ServerDetails{}, "local", "my-skill", "1.0.0", tmpDir, "skill")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "my-skill-1.0.0.zip"), got)
}

func TestDownloadPackageZip_DownloadError(t *testing.T) {
	restore := createDownloadServiceManagerForPackageZip
	downloadErr := errors.New("network timeout")
	createDownloadServiceManagerForPackageZip = func(_ *config.ServerDetails) (packageZipDownloadManager, error) {
		return &fakePackageZipDownloader{err: downloadErr}, nil
	}
	t.Cleanup(func() { createDownloadServiceManagerForPackageZip = restore })

	_, err := DownloadPackageZip(&config.ServerDetails{}, "local", "my-plugin", "2.0.0", t.TempDir(), "plugin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download local/my-plugin/2.0.0/my-plugin-2.0.0.zip")
	assert.ErrorIs(t, err, downloadErr)
}

func TestDownloadPackageZip_DownloadFailed(t *testing.T) {
	restore := createDownloadServiceManagerForPackageZip
	createDownloadServiceManagerForPackageZip = func(_ *config.ServerDetails) (packageZipDownloadManager, error) {
		return &fakePackageZipDownloader{failed: 1}, nil
	}
	t.Cleanup(func() { createDownloadServiceManagerForPackageZip = restore })

	_, err := DownloadPackageZip(&config.ServerDetails{}, "local", "my-plugin", "2.0.0", t.TempDir(), "plugin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download failed for local/my-plugin/2.0.0/my-plugin-2.0.0.zip")
}

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	w := zip.NewWriter(f)
	defer func() {
		_ = w.Close()
	}()

	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}
