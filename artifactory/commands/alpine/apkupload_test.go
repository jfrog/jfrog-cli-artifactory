package alpine

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	bibuild "github.com/jfrog/build-info-go/build"
	biUtils "github.com/jfrog/build-info-go/build/utils"
	specutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateArtifactoryPathSegment(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid branch", value: "main", wantErr: false},
		{name: "valid arch", value: "x86_64", wantErr: false},
		{name: "valid version", value: "v3.21", wantErr: false},
		{name: "empty", value: "", wantErr: true},
		{name: "slash", value: "main/../other", wantErr: true},
		{name: "backslash", value: `main\other`, wantErr: true},
		{name: "dotdot", value: "..", wantErr: true},
		{name: "contains dotdot", value: "foo..bar", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArtifactoryPathSegment("branch", tc.value)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAlpinePackageFromDepID(t *testing.T) {
	pkg := alpinePackageFromDepID("curl:8.5.0-r0")
	assert.Equal(t, "curl", pkg.Name)
	assert.Equal(t, "8.5.0-r0", pkg.Version)

	pkg = alpinePackageFromDepID("musl-1.2.4-r2")
	assert.Equal(t, "musl", pkg.Name)
	assert.Equal(t, "1.2.4-r2", pkg.Version)
}

func TestAlpinePackageFromDepID_ProviderTokenIsNotSplit(t *testing.T) {
	pkg := alpinePackageFromDepID("so:libc.musl-x86_64.so.1")
	assert.Equal(t, "so:libc.musl-x86_64.so.1", pkg.Name)
	assert.Empty(t, pkg.Version)

	pkg = alpinePackageFromDepID("cmd:curl")
	assert.Equal(t, "cmd:curl", pkg.Name)
	assert.Empty(t, pkg.Version)
}

func TestIsApkProviderToken(t *testing.T) {
	assert.True(t, isApkProviderToken("so:libz.so.1"))
	assert.True(t, isApkProviderToken("cmd:curl"))
	assert.True(t, isApkProviderToken("pc:openssl"))
	assert.False(t, isApkProviderToken("curl:8.5.0-r0"))
	assert.False(t, isApkProviderToken("musl"))
}

func TestResolveDepIDWithProviders(t *testing.T) {
	providers := map[string]string{
		"so:libcurl.so.4": "libcurl",
		"cmd:curl":        "curl",
		"musl":            "musl",
	}
	byName := map[string]biUtils.AlpinePackage{
		"libcurl": {Name: "libcurl", Version: "8.5.0-r0"},
		"curl":    {Name: "curl", Version: "8.5.0-r0"},
		"musl":    {Name: "musl", Version: "1.2.4-r2"},
	}

	assert.Equal(t, "libcurl:8.5.0-r0", resolveDepIDWithProviders("so:libcurl.so.4", providers, byName),
		"a shared-object dep must resolve to the package providing it")
	assert.Equal(t, "libcurl:8.5.0-r0", resolveDepIDWithProviders("so:libcurl.so.4=8.5.0", providers, byName),
		"the version constraint must be stripped before provider lookup")
	assert.Equal(t, "curl:8.5.0-r0", resolveDepIDWithProviders("cmd:curl", providers, byName))
	assert.Equal(t, "musl:1.2.4-r2", resolveDepIDWithProviders("musl>=1.2.3", providers, byName))
}

func TestResolveDepIDWithProviders_UnknownProviderFallsBackToToken(t *testing.T) {
	assert.Equal(t, "so:libunknown.so.9", resolveDepIDWithProviders("so:libunknown.so.9>=1", nil, nil))
}

func TestStripVersionConstraint(t *testing.T) {
	assert.Equal(t, "musl", stripVersionConstraint("musl>=1.2.3"))
	assert.Equal(t, "openssl", stripVersionConstraint("openssl<=3.0"))
	assert.Equal(t, "bash", stripVersionConstraint("bash"))
	assert.Equal(t, "so:libz.so.1", stripVersionConstraint("so:libz.so.1>=1.0"),
		"provider prefixes must survive so the token can be matched against provides")
}

func TestAlpineUploadTargetProps(t *testing.T) {
	propsStr := fmt.Sprintf(
		"os.name=alpine;os.version=%s;os.arch=%s;apk.name=%s;apk.version=%s",
		"v3.21", "x86_64", "curl", "8.5.0-r0",
	)
	props, err := specutils.ParseProperties(propsStr)
	require.NoError(t, err)
	assert.Equal(t, "alpine", props.ToMap()["os.name"][0])
	assert.Equal(t, "v3.21", props.ToMap()["os.version"][0])
	assert.Equal(t, "x86_64", props.ToMap()["os.arch"][0])
	assert.Equal(t, "curl", props.ToMap()["apk.name"][0])
	assert.Equal(t, "8.5.0-r0", props.ToMap()["apk.version"][0])
}

func TestResolveDepID_MissingApk(t *testing.T) {
	_, err := resolveDepID("this-package-definitely-does-not-exist-xyz")
	require.Error(t, err)
}

func writeTestApk(t *testing.T, pkgInfo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "testpkg-1.0.0-r0.x86_64.apk")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: ".PKGINFO",
		Mode: 0644,
		Size: int64(len(pkgInfo)),
	}))
	_, err = tw.Write([]byte(pkgInfo))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return path
}

func TestCollectApkDependencies_ScopesMatchAddFlow(t *testing.T) {
	pkgInfo := "pkgname = testpkg\npkgver = 1.0.0-r0\narch = x86_64\n" +
		"depend = musl>=1.2.3\ndepend = zlib\n"
	apkPath := writeTestApk(t, pkgInfo)

	apkCmd := NewApkUploadCommand(apkPath)
	uploadedID := "alpine-local/v3.21/main/x86_64/testpkg-1.0.0-r0.x86_64.apk"
	deps, err := apkCmd.collectApkDependencies("testpkg-does-not-exist-xyz", apkPath, uploadedID)
	require.NoError(t, err)
	require.NotEmpty(t, deps, "dependencies should be parsed from .PKGINFO")

	for _, dep := range deps {
		assert.Equal(t, []string{bibuild.AlpineScopeProd}, dep.Scopes,
			"dependency %s should be scoped %q like first-level deps in the add flow", dep.Id, bibuild.AlpineScopeProd)
		require.NotEmpty(t, dep.RequestedBy, "dependency %s should record the uploaded artifact as its parent", dep.Id)
		assert.Equal(t, uploadedID, dep.RequestedBy[0][len(dep.RequestedBy[0])-1])
	}
}

func TestAlpineScopeConstantsAreStable(t *testing.T) {
	assert.Equal(t, "prod", bibuild.AlpineScopeProd)
	assert.Equal(t, "transitive", bibuild.AlpineScopeTransitive)
}
