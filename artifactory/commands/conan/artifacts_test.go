package conan

import (
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
)

func TestParsePackageReference(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		expected    *ConanPackageInfo
		expectError bool
	}{
		{
			name: "Conan 2.x format - name/version",
			ref:  "zlib/1.3.1",
			expected: &ConanPackageInfo{
				Name:    "zlib",
				Version: "1.3.1",
				User:    "_",
				Channel: "_",
			},
			expectError: false,
		},
		{
			name: "Conan 1.x format - name/version@user/channel",
			ref:  "boost/1.82.0@myuser/stable",
			expected: &ConanPackageInfo{
				Name:    "boost",
				Version: "1.82.0",
				User:    "myuser",
				Channel: "stable",
			},
			expectError: false,
		},
		{
			name: "Package with underscore in name",
			ref:  "my_package/2.0.0",
			expected: &ConanPackageInfo{
				Name:    "my_package",
				Version: "2.0.0",
				User:    "_",
				Channel: "_",
			},
			expectError: false,
		},
		{
			name: "Package with complex version",
			ref:  "openssl/3.1.2",
			expected: &ConanPackageInfo{
				Name:    "openssl",
				Version: "3.1.2",
				User:    "_",
				Channel: "_",
			},
			expectError: false,
		},
		{
			name: "With whitespace - should be trimmed",
			ref:  "  fmt/10.2.1  ",
			expected: &ConanPackageInfo{
				Name:    "fmt",
				Version: "10.2.1",
				User:    "_",
				Channel: "_",
			},
			expectError: false,
		},
		{
			name:        "Invalid format - no slash",
			ref:         "invalid-package",
			expectError: true,
		},
		{
			name:        "Invalid format - empty string",
			ref:         "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePackageReference(tt.ref)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, tt.expected.Version, result.Version)
				assert.Equal(t, tt.expected.User, result.User)
				assert.Equal(t, tt.expected.Channel, result.Channel)
			}
		})
	}
}

func TestBuildArtifactQuery(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		pkgInfo  *ConanPackageInfo
		expected string
	}{
		{
			name: "Conan 2.x path format",
			repo: "conan-local",
			pkgInfo: &ConanPackageInfo{
				Name:    "zlib",
				Version: "1.3.1",
				User:    "_",
				Channel: "_",
			},
			expected: `{"repo": "conan-local", "path": {"$match": "_/zlib/1.3.1/_/*"}}`,
		},
		{
			name: "Conan 1.x path format",
			repo: "conan-local",
			pkgInfo: &ConanPackageInfo{
				Name:    "boost",
				Version: "1.82.0",
				User:    "myuser",
				Channel: "stable",
			},
			expected: `{"repo": "conan-local", "path": {"$match": "myuser/boost/1.82.0/stable/*"}}`,
		},
		{
			name: "Different repository name",
			repo: "my-conan-repo",
			pkgInfo: &ConanPackageInfo{
				Name:    "fmt",
				Version: "10.2.1",
				User:    "_",
				Channel: "_",
			},
			expected: `{"repo": "my-conan-repo", "path": {"$match": "_/fmt/10.2.1/_/*"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildArtifactQuery(tt.repo, tt.pkgInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPropertySetter_FormatBuildProperties(t *testing.T) {
	tests := []struct {
		name        string
		buildName   string
		buildNumber string
		projectKey  string
		timestamp   string
		expected    string
	}{
		{
			name:        "Without project key",
			buildName:   "my-build",
			buildNumber: "123",
			projectKey:  "",
			timestamp:   "1234567890",
			expected:    "build.name=my-build;build.number=123;build.timestamp=1234567890",
		},
		{
			name:        "With project key",
			buildName:   "my-build",
			buildNumber: "456",
			projectKey:  "myproject",
			timestamp:   "9876543210",
			expected:    "build.name=my-build;build.number=456;build.timestamp=9876543210;build.project=myproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setter := &BuildPropertySetter{
				buildName:   tt.buildName,
				buildNumber: tt.buildNumber,
				projectKey:  tt.projectKey,
			}
			result := setter.formatBuildProperties(tt.timestamp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewArtifactCollector(t *testing.T) {
	targetRepo := "conan-local"

	collector := NewArtifactCollector(nil, targetRepo)

	assert.NotNil(t, collector)
	assert.Equal(t, targetRepo, collector.targetRepo)
	assert.Nil(t, collector.serverDetails)
}

func TestNewBuildPropertySetter(t *testing.T) {
	buildName := "test-build"
	buildNumber := "1"
	projectKey := "test-project"
	targetRepo := "conan-local"

	setter := NewBuildPropertySetter(nil, targetRepo, buildName, buildNumber, projectKey)

	assert.NotNil(t, setter)
	assert.Equal(t, buildName, setter.buildName)
	assert.Equal(t, buildNumber, setter.buildNumber)
	assert.Equal(t, projectKey, setter.projectKey)
	assert.Equal(t, targetRepo, setter.targetRepo)
}

func TestConvertToResultItems_NoPathDuplication(t *testing.T) {
	setter := &BuildPropertySetter{targetRepo: "401004-conan"}

	artifacts := []entities.Artifact{
		{
			Name: "conanfile.py",
			Path: "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/export/conanfile.py",
			Checksum: entities.Checksum{
				Sha1: "abc123",
				Md5:  "def456",
			},
		},
		{
			Name: "conanmanifest.txt",
			Path: "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/export/conanmanifest.txt",
			Checksum: entities.Checksum{
				Sha1: "ghi789",
				Md5:  "jkl012",
			},
		},
		{
			Name: "conan_package.tgz",
			Path: "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/package/da39a3ee5e6b4b0d3255bfef95601890afd80709/0ba8627bd47edc3a501e8f0eb9a79e5e/conan_package.tgz",
			Checksum: entities.Checksum{
				Sha1: "mno345",
				Md5:  "pqr678",
			},
		},
	}

	items := setter.convertToResultItems(artifacts)
	assert.Len(t, items, 3)

	// Verify Path contains only the directory, not the filename
	assert.Equal(t, "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/export", items[0].Path)
	assert.Equal(t, "conanfile.py", items[0].Name)

	assert.Equal(t, "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/export", items[1].Path)
	assert.Equal(t, "conanmanifest.txt", items[1].Name)

	assert.Equal(t, "_/democconan/1.0.1000/_/72c46d08de471b9f67d565d90163d5c1/package/da39a3ee5e6b4b0d3255bfef95601890afd80709/0ba8627bd47edc3a501e8f0eb9a79e5e", items[2].Path)
	assert.Equal(t, "conan_package.tgz", items[2].Name)

	// Verify GetItemRelativePath produces a correct URL (no duplicated filename)
	for _, item := range items {
		relPath := item.GetItemRelativePath()
		assert.NotContains(t, relPath, item.Name+"/"+item.Name,
			"Path should not contain duplicated filename: %s", relPath)
	}
}

func TestBuildPropertySetter_ConvertToResultItems_UsesArtifactRepo(t *testing.T) {
	setter := &BuildPropertySetter{
		targetRepo: "conan-virtual",
	}

	artifacts := []entities.Artifact{
		{
			Path:                   "anders/pkg/1.0.0/test/rev/export",
			Name:                   "conanfile.py",
			OriginalDeploymentRepo: "conan-snapshot-local",
			Checksum: entities.Checksum{
				Sha1:   "sha1-1",
				Md5:    "md5-1",
				Sha256: "sha256-1",
			},
		},
		{
			Path: "anders/pkg/1.0.0/test/rev/export",
			Name: "conanmanifest.txt",
			Checksum: entities.Checksum{
				Sha1:   "sha1-2",
				Md5:    "md5-2",
				Sha256: "sha256-2",
			},
		},
	}

	items := setter.convertToResultItems(artifacts)
	if assert.Len(t, items, 2) {
		assert.Equal(t, "conan-snapshot-local", items[0].Repo)
		assert.Equal(t, "conan-virtual", items[1].Repo)
		assert.Equal(t, artifacts[0].Path, items[0].Path)
		assert.Equal(t, artifacts[1].Path, items[1].Path)
	}
}
