package common

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
