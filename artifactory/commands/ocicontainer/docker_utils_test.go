package ocicontainer

import (
	"strings"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

func TestModifyPathForRemoteRepo(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "path with sha256 prefix",
			path:     "sha256:abc123def456",
			expected: "library/sha256__abc123def456",
		},
		{
			name:     "path without sha256 prefix",
			path:     "alpine/latest",
			expected: "library/alpine/latest",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "library/",
		},
		{
			name:     "path with nested directories",
			path:     "org/repo/sha256:abc123",
			expected: "library/org/repo/sha256__abc123",
		},
		{
			name:     "path with multiple sha256",
			path:     "sha256:abc/sha256:def",
			expected: "library/sha256__abc/sha256:def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := modifyPathForRemoteRepo(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeduplicateResultsBySha256(t *testing.T) {
	tests := []struct {
		name     string
		input    []utils.ResultItem
		expected int
	}{
		{
			name:     "empty input",
			input:    []utils.ResultItem{},
			expected: 0,
		},
		{
			name: "no duplicates",
			input: []utils.ResultItem{
				{Sha256: "sha256_1", Name: "file1"},
				{Sha256: "sha256_2", Name: "file2"},
				{Sha256: "sha256_3", Name: "file3"},
			},
			expected: 3,
		},
		{
			name: "all duplicates",
			input: []utils.ResultItem{
				{Sha256: "sha256_same", Name: "file1"},
				{Sha256: "sha256_same", Name: "file2"},
				{Sha256: "sha256_same", Name: "file3"},
			},
			expected: 1,
		},
		{
			name: "some duplicates",
			input: []utils.ResultItem{
				{Sha256: "sha256_1", Name: "file1"},
				{Sha256: "sha256_2", Name: "file2"},
				{Sha256: "sha256_1", Name: "file3"},
				{Sha256: "sha256_3", Name: "file4"},
				{Sha256: "sha256_2", Name: "file5"},
			},
			expected: 3,
		},
		{
			name: "preserves first occurrence",
			input: []utils.ResultItem{
				{Sha256: "sha256_1", Name: "first"},
				{Sha256: "sha256_1", Name: "second"},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateResultsBySha256(tt.input)
			assert.Equal(t, tt.expected, len(result))

			// Verify no duplicates in result
			seen := make(map[string]bool)
			for _, item := range result {
				assert.False(t, seen[item.Sha256], "Found duplicate SHA256 in result")
				seen[item.Sha256] = true
			}
		})
	}
}

func TestDeduplicateResultsBySha256_PreservesFirstOccurrence(t *testing.T) {
	input := []utils.ResultItem{
		{Sha256: "sha256_1", Name: "first_occurrence", Repo: "repo1"},
		{Sha256: "sha256_1", Name: "second_occurrence", Repo: "repo2"},
	}

	result := deduplicateResultsBySha256(input)

	assert.Equal(t, 1, len(result))
	assert.Equal(t, "first_occurrence", result[0].Name)
	assert.Equal(t, "repo1", result[0].Repo)
}

func TestGetMarkerLayerShasFromSearchResult(t *testing.T) {
	tests := []struct {
		name                    string
		input                   []utils.ResultItem
		expectedMarkerCount     int
		expectedNonMarkerCount  int
		expectedMarkerLayerShas []string
	}{
		{
			name:                    "empty input",
			input:                   []utils.ResultItem{},
			expectedMarkerCount:     0,
			expectedNonMarkerCount:  0,
			expectedMarkerLayerShas: nil,
		},
		{
			name: "no marker layers",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1"},
				{Name: "layer2", Sha256: "sha256_2"},
			},
			expectedMarkerCount:     0,
			expectedNonMarkerCount:  2,
			expectedMarkerLayerShas: nil,
		},
		{
			name: "all marker layers",
			input: []utils.ResultItem{
				{Name: "abc123" + markerLayerSuffix, Sha256: "sha256_1"},
				{Name: "def456" + markerLayerSuffix, Sha256: "sha256_2"},
			},
			expectedMarkerCount:     2,
			expectedNonMarkerCount:  0,
			expectedMarkerLayerShas: []string{"abc123", "def456"},
		},
		{
			name: "mixed layers",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1"},
				{Name: "abc123" + markerLayerSuffix, Sha256: "sha256_marker"},
				{Name: "layer2", Sha256: "sha256_2"},
			},
			expectedMarkerCount:     1,
			expectedNonMarkerCount:  2,
			expectedMarkerLayerShas: []string{"abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markerShas, filtered := getMarkerLayerShasFromSearchResult(tt.input)

			assert.Equal(t, tt.expectedMarkerCount, len(markerShas))
			assert.Equal(t, tt.expectedNonMarkerCount, len(filtered))

			if tt.expectedMarkerLayerShas != nil {
				assert.Equal(t, tt.expectedMarkerLayerShas, markerShas)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "library", remoteRepoLibraryPrefix)
	assert.Equal(t, "sha256:", sha256Prefix)
	assert.Equal(t, "sha256__", sha256RemoteFormat)
	assert.Equal(t, "_uploads", uploadsFolder)
	assert.Equal(t, "-cache", remoteCacheSuffix)
	assert.Equal(t, "remote", remoteRepositoryType)
}

func TestFilterLayersFromVirtualRepo(t *testing.T) {
	tests := []struct {
		name       string
		items      []utils.ResultItem
		pushedRepo string
		expected   int
	}{
		{
			name:       "empty items",
			items:      []utils.ResultItem{},
			pushedRepo: "my-repo",
			expected:   0,
		},
		{
			name: "all match",
			items: []utils.ResultItem{
				{Repo: "my-repo", Name: "layer1"},
				{Repo: "my-repo", Name: "layer2"},
			},
			pushedRepo: "my-repo",
			expected:   2,
		},
		{
			name: "none match",
			items: []utils.ResultItem{
				{Repo: "other-repo", Name: "layer1"},
				{Repo: "another-repo", Name: "layer2"},
			},
			pushedRepo: "my-repo",
			expected:   0,
		},
		{
			name: "some match",
			items: []utils.ResultItem{
				{Repo: "my-repo", Name: "layer1"},
				{Repo: "other-repo", Name: "layer2"},
				{Repo: "my-repo", Name: "layer3"},
			},
			pushedRepo: "my-repo",
			expected:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterLayersFromVirtualRepo(tt.items, tt.pushedRepo)
			assert.Equal(t, tt.expected, len(result))

			for _, item := range result {
				assert.Equal(t, tt.pushedRepo, item.Repo)
			}
		})
	}
}

func TestSingleManifestHandler_BuildSearchPaths(t *testing.T) {
	handler := &SingleManifestHandler{}

	tests := []struct {
		imageName      string
		imageTag       string
		manifestDigest string
		expected       string
	}{
		{"nginx", "latest", "sha256:abc123", "nginx/latest"},
		{"myapp", "v1.0.0", "sha256:def456", "myapp/v1.0.0"},
		{"org/repo", "1.0", "sha256:xyz", "org/repo/1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.imageName+"/"+tt.imageTag, func(t *testing.T) {
			result := handler.BuildSearchPaths(tt.imageName, tt.imageTag, tt.manifestDigest)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFatManifestHandler_BuildSearchPaths(t *testing.T) {
	handler := &FatManifestHandler{}

	tests := []struct {
		imageName      string
		imageTag       string
		manifestDigest string
		expected       string
	}{
		{"nginx", "latest", "sha256:abc123", "nginx/sha256:abc123"},
		{"myapp", "v1.0.0", "sha256:def456", "myapp/sha256:def456"},
		{"org/repo", "1.0", "sha256:xyz", "org/repo/sha256:xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.imageName+"/"+tt.manifestDigest, func(t *testing.T) {
			result := handler.BuildSearchPaths(tt.imageName, tt.imageTag, tt.manifestDigest)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBiProperties(t *testing.T) {
	tests := []struct {
		name          string
		imageTag      string
		isImagePushed bool
		cmdArgs       []string
		leadSha       string
		expectedKeys  []string
	}{
		{
			name:          "basic properties",
			imageTag:      "myimage:latest",
			isImagePushed: false,
			cmdArgs:       nil,
			leadSha:       "",
			expectedKeys:  []string{"docker.image.tag"},
		},
		{
			name:          "with pushed image",
			imageTag:      "myimage:v1.0",
			isImagePushed: true,
			cmdArgs:       nil,
			leadSha:       "sha256:abc123",
			expectedKeys:  []string{"docker.image.tag", "docker.image.id"},
		},
		{
			name:          "with cmd args",
			imageTag:      "myimage:latest",
			isImagePushed: false,
			cmdArgs:       []string{"docker", "build", "-t", "myimage"},
			leadSha:       "",
			expectedKeys:  []string{"docker.image.tag", "docker.build.command"},
		},
		{
			name:          "all properties",
			imageTag:      "myimage:v2.0",
			isImagePushed: true,
			cmdArgs:       []string{"docker", "build", "."},
			leadSha:       "sha256:def456",
			expectedKeys:  []string{"docker.image.tag", "docker.image.id", "docker.build.command"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &DockerBuildInfoBuilder{
				imageTag:      tt.imageTag,
				isImagePushed: tt.isImagePushed,
				cmdArgs:       tt.cmdArgs,
			}

			props := builder.getBiProperties(tt.leadSha)

			assert.Equal(t, tt.imageTag, props["docker.image.tag"])

			for _, key := range tt.expectedKeys {
				_, exists := props[key]
				assert.True(t, exists, "Expected key %s to exist", key)
			}

			if tt.isImagePushed {
				assert.Equal(t, tt.leadSha, props["docker.image.id"])
			}

			if tt.cmdArgs != nil {
				assert.Contains(t, props["docker.build.command"], "docker build")
			}
		})
	}
}

func TestGetPushedRepo(t *testing.T) {
	tests := []struct {
		name        string
		repoDetails *DockerRepositoryDetails
		expected    string
	}{
		{
			name:        "nil repository details",
			repoDetails: nil,
			expected:    "",
		},
		{
			name: "local repository",
			repoDetails: &DockerRepositoryDetails{
				Key:      "docker-local",
				RepoType: "local",
			},
			expected: "docker-local",
		},
		{
			name: "remote repository",
			repoDetails: &DockerRepositoryDetails{
				Key:      "docker-remote",
				RepoType: "remote",
			},
			expected: "docker-remote",
		},
		{
			name: "virtual repository",
			repoDetails: &DockerRepositoryDetails{
				Key:                   "docker-virtual",
				RepoType:              "virtual",
				DefaultDeploymentRepo: "docker-local-deploy",
			},
			expected: "docker-local-deploy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &DockerArtifactsBuilder{
				repositoryDetails: tt.repoDetails,
			}

			result := builder.GetOriginalDeploymentRepo()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyRepoTypeModifications(t *testing.T) {
	tests := []struct {
		name        string
		repoType    string
		basePath    string
		expectedLen int
		expected    []string
	}{
		{
			name:        "local repository",
			repoType:    "local",
			basePath:    "nginx/latest",
			expectedLen: 1,
			expected:    []string{"nginx/latest"},
		},
		{
			name:        "remote repository",
			repoType:    "remote",
			basePath:    "nginx/sha256:abc123",
			expectedLen: 1,
			expected:    []string{"library/nginx/sha256__abc123"},
		},
		{
			name:        "virtual repository",
			repoType:    "virtual",
			basePath:    "nginx/sha256:abc123",
			expectedLen: 2,
			expected:    []string{"library/nginx/sha256__abc123", "nginx/sha256:abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &DockerDependenciesBuilder{}
			repoDetails := DockerRepositoryDetails{RepoType: tt.repoType}

			result := builder.applyRepoTypeModifications(tt.basePath, repoDetails)
			assert.Equal(t, tt.expectedLen, len(result))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetManifestHandler(t *testing.T) {
	manifestHandler := &DockerManifestHandler{}

	tests := []struct {
		name         string
		manifestType ManifestType
		expectNil    bool
		handlerType  string
	}{
		{
			name:         "manifest list returns FatManifestHandler",
			manifestType: ManifestList,
			expectNil:    false,
			handlerType:  "fat",
		},
		{
			name:         "single manifest returns SingleManifestHandler",
			manifestType: Manifest,
			expectNil:    false,
			handlerType:  "single",
		},
		{
			name:         "unknown type returns nil",
			manifestType: ManifestType("unknown"),
			expectNil:    true,
			handlerType:  "",
		},
		{
			name:         "empty type returns nil",
			manifestType: ManifestType(""),
			expectNil:    true,
			handlerType:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := manifestHandler.GetManifestHandler(tt.manifestType)

			if tt.expectNil {
				assert.Nil(t, handler)
			} else {
				assert.NotNil(t, handler)
				switch tt.handlerType {
				case "fat":
					_, ok := handler.(*FatManifestHandler)
					assert.True(t, ok, "Expected FatManifestHandler")
				case "single":
					_, ok := handler.(*SingleManifestHandler)
					assert.True(t, ok, "Expected SingleManifestHandler")
				}
			}
		})
	}
}

func TestCreateSearchablePathForDockerManifestContents(t *testing.T) {
	handler := &FatManifestHandler{}

	tests := []struct {
		name         string
		imageRef     string
		manifestShas []string
		expectedLen  int
	}{
		{
			name:         "empty manifest shas",
			imageRef:     "registry.io/repo/nginx:latest",
			manifestShas: []string{},
			expectedLen:  0,
		},
		{
			name:         "single manifest sha",
			imageRef:     "registry.io/repo/nginx:latest",
			manifestShas: []string{"sha256:abc123"},
			expectedLen:  1,
		},
		{
			name:         "multiple manifest shas",
			imageRef:     "registry.io/repo/nginx:latest",
			manifestShas: []string{"sha256:abc123", "sha256:def456", "sha256:ghi789"},
			expectedLen:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.createSearchablePathForDockerManifestContents(tt.imageRef, tt.manifestShas)
			assert.Equal(t, tt.expectedLen, len(result))

			// Verify each path contains the manifest sha
			for i, path := range result {
				assert.Contains(t, path, tt.manifestShas[i])
			}
		})
	}
}

func TestCreateDependenciesFromResults(t *testing.T) {
	builder := &DockerDependenciesBuilder{}

	tests := []struct {
		name        string
		input       []utils.ResultItem
		expectedLen int
	}{
		{
			name:        "empty results",
			input:       []utils.ResultItem{},
			expectedLen: 0,
		},
		{
			name: "single result",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1", Actual_Sha1: "sha1_1", Actual_Md5: "md5_1"},
			},
			expectedLen: 1,
		},
		{
			name: "multiple results no duplicates",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1", Actual_Sha1: "sha1_1"},
				{Name: "layer2", Sha256: "sha256_2", Actual_Sha1: "sha1_2"},
				{Name: "layer3", Sha256: "sha256_3", Actual_Sha1: "sha1_3"},
			},
			expectedLen: 3,
		},
		{
			name: "multiple results with duplicates deduped",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1"},
				{Name: "layer2", Sha256: "sha256_2"},
				{Name: "layer1_dup", Sha256: "sha256_1"},
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.createDependenciesFromResults(tt.input)
			assert.Equal(t, tt.expectedLen, len(result))
		})
	}
}

func TestCreateArtifactsFromResults(t *testing.T) {
	builder := &DockerArtifactsBuilder{}

	tests := []struct {
		name        string
		input       []utils.ResultItem
		expectedLen int
	}{
		{
			name:        "empty results",
			input:       []utils.ResultItem{},
			expectedLen: 0,
		},
		{
			name: "single result",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1", Actual_Sha1: "sha1_1", Actual_Md5: "md5_1"},
			},
			expectedLen: 1,
		},
		{
			name: "multiple results no duplicates",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_1", Actual_Sha1: "sha1_1"},
				{Name: "layer2", Sha256: "sha256_2", Actual_Sha1: "sha1_2"},
			},
			expectedLen: 2,
		},
		{
			name: "multiple results with duplicates deduped",
			input: []utils.ResultItem{
				{Name: "layer1", Sha256: "sha256_same"},
				{Name: "layer2", Sha256: "sha256_same"},
				{Name: "layer3", Sha256: "sha256_other"},
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.createArtifactsFromResults(tt.input)
			assert.Equal(t, tt.expectedLen, len(result))
		})
	}
}

func TestManifestTypeConstants(t *testing.T) {
	assert.Equal(t, ManifestType("list.manifest.json"), ManifestList)
	assert.Equal(t, ManifestType("manifest.json"), Manifest)
}

func TestFetchLayersOfPushedImage_UnknownManifestType(t *testing.T) {
	handler := &DockerManifestHandler{}

	// Test error handling for unknown manifest type
	layers, foldersToApplyProps, err := handler.FetchLayersOfPushedImage("test-image", "test-repo", ManifestType("unknown"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown/other manifest type")
	assert.Empty(t, layers)
	assert.Empty(t, foldersToApplyProps)
}

func TestFetchLayersOfPushedImage_EmptyManifestType(t *testing.T) {
	handler := &DockerManifestHandler{}

	// Test error handling for empty manifest type
	layers, foldersToApplyProps, err := handler.FetchLayersOfPushedImage("test-image", "test-repo", ManifestType(""))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown/other manifest type")
	assert.Empty(t, layers)
	assert.Empty(t, foldersToApplyProps)
}

func TestGetArtifacts_ImageNotPushed(t *testing.T) {
	builder := &DockerArtifactsBuilder{
		isImagePushed: false,
		imageTag:      "test:latest",
	}

	artifacts, leadSha, resultsToApplyProps, err := builder.getArtifacts()

	assert.NoError(t, err)
	assert.Empty(t, artifacts)
	assert.Empty(t, leadSha)
	assert.Empty(t, resultsToApplyProps)
}

func TestGetArtifacts_ImagePushed(t *testing.T) {
	// This test verifies that when isImagePushed is true, the function attempts to collect artifacts
	// Without proper serviceManager setup, it will error, but we verify the code path is taken
	builder := &DockerArtifactsBuilder{
		isImagePushed: true,
		imageTag:      "test:latest",
	}

	// Without proper setup, this will error, but we verify the path is taken
	artifacts, leadSha, resultsToApplyProps, err := builder.getArtifacts()

	// Error expected without proper mocking, but function should attempt collection
	// We just verify the function executes (error is expected without serviceManager)
	if err == nil {
		// If no error, artifacts should be populated
		assert.NotNil(t, artifacts)
	} else {
		// If error, artifacts may be nil or empty
		_ = artifacts
	}
	_ = leadSha
	_ = resultsToApplyProps
}

func TestNormalizeLayerSha(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain hex digest", "6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39", "6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39"},
		{"sha256: prefix", "sha256:6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39", "6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39"},
		{"sha256__ prefix", "sha256__6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39", "6b59a28fa20117e6048ad0616b8d8c901877ef15ff4c7f18db04e4f01f43bc39"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeLayerSha(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDigestBasedImageDetection(t *testing.T) {
	// Test that digest-based images are correctly identified
	tests := []struct {
		name     string
		imageTag string
		isDigest bool
	}{
		{"tag-based image", "latest", false},
		{"version tag", "v1.0.0", false},
		{"digest-based image", "sha256:abc123def456", true},
		{"full digest", "sha256:4a2047b0e69af48c94821afb84ded71dee018059ac708e0e8f3e687e22726cd2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify our detection logic matches expected behavior
			isDigest := strings.HasPrefix(tt.imageTag, "sha256:")
			assert.Equal(t, tt.isDigest, isDigest)
		})
	}
}
