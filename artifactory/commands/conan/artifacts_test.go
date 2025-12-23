package conan

import (
	"testing"

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




