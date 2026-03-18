package conan

import (
	"testing"

	conanflex "github.com/jfrog/build-info-go/flexpack/conan"
	"github.com/stretchr/testify/assert"
)

func TestUploadProcessor_ExtractUploadInfo(t *testing.T) {
	tests := []struct {
		name             string
		output           ConanUploadOutput
		expectedRemote   string
		expectedPkgCount int
	}{
		{
			name: "Single package uploaded",
			output: ConanUploadOutput{
				"conan-local": {
					"simplelib/1.0.0": ConanUploadRecipe{
						Revisions: map[string]ConanUploadRecipeRevision{
							"86deb56ab95f8fe27d07debf8a6ee3f9": {},
						},
					},
				},
			},
			expectedRemote:   "conan-local",
			expectedPkgCount: 1,
		},
		{
			name: "Multiple packages uploaded",
			output: ConanUploadOutput{
				"my-remote": {
					"zlib/1.2.13": ConanUploadRecipe{
						Revisions: map[string]ConanUploadRecipeRevision{
							"abc123": {},
						},
					},
					"openssl/3.1.2": ConanUploadRecipe{
						Revisions: map[string]ConanUploadRecipeRevision{
							"def456": {},
						},
					},
				},
			},
			expectedRemote:   "my-remote",
			expectedPkgCount: 2,
		},
		{
			name:             "Empty output",
			output:           ConanUploadOutput{},
			expectedRemote:   "",
			expectedPkgCount: 0,
		},
		{
			name: "Remote with no packages",
			output: ConanUploadOutput{
				"conan-local": {},
			},
			expectedRemote:   "",
			expectedPkgCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &UploadProcessor{}
			remote, packages := processor.extractUploadInfo(tt.output)
			assert.Equal(t, tt.expectedRemote, remote)
			assert.Len(t, packages, tt.expectedPkgCount)
		})
	}
}

func TestUploadProcessor_BuildArtifactPathsFromJSON(t *testing.T) {
	tests := []struct {
		name          string
		packages      map[string]ConanUploadRecipe
		expectedPaths []string
	}{
		{
			name: "Recipe only (no binary packages)",
			packages: map[string]ConanUploadRecipe{
				"simplelib/1.0.0": {
					Revisions: map[string]ConanUploadRecipeRevision{
						"86deb56ab95f8fe27d07debf8a6ee3f9": {},
					},
				},
			},
			expectedPaths: []string{
				"_/simplelib/1.0.0/_/86deb56ab95f8fe27d07debf8a6ee3f9/export",
			},
		},
		{
			name: "Recipe with one binary package",
			packages: map[string]ConanUploadRecipe{
				"multideps/1.0.0": {
					Revisions: map[string]ConanUploadRecipeRevision{
						"797d134a8590a1bfa06d846768443f48": {
							Packages: map[string]ConanUploadPackageEntry{
								"594ed0eb2e9dfcc60607438924c35871514e6c2a": {
									Revisions: map[string]interface{}{
										"ca858ea14c32f931e49241df0b52bec9": nil,
									},
								},
							},
						},
					},
				},
			},
			expectedPaths: []string{
				"_/multideps/1.0.0/_/797d134a8590a1bfa06d846768443f48/export",
				"_/multideps/1.0.0/_/797d134a8590a1bfa06d846768443f48/package/594ed0eb2e9dfcc60607438924c35871514e6c2a/ca858ea14c32f931e49241df0b52bec9",
			},
		},
		{
			name: "Recipe with multiple binary packages",
			packages: map[string]ConanUploadRecipe{
				"mylib/2.0.0": {
					Revisions: map[string]ConanUploadRecipeRevision{
						"aaa111": {
							Packages: map[string]ConanUploadPackageEntry{
								"pkg_id_1": {
									Revisions: map[string]interface{}{
										"rev_a": nil,
									},
								},
								"pkg_id_2": {
									Revisions: map[string]interface{}{
										"rev_b": nil,
									},
								},
							},
						},
					},
				},
			},
			expectedPaths: []string{
				"_/mylib/2.0.0/_/aaa111/export",
				"_/mylib/2.0.0/_/aaa111/package/pkg_id_1/rev_a",
				"_/mylib/2.0.0/_/aaa111/package/pkg_id_2/rev_b",
			},
		},
		{
			name:          "Empty packages",
			packages:      map[string]ConanUploadRecipe{},
			expectedPaths: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &UploadProcessor{}
			paths := processor.buildArtifactPathsFromJSON(tt.packages)

			if tt.expectedPaths == nil {
				assert.Nil(t, paths)
			} else {
				assert.Len(t, paths, len(tt.expectedPaths))
				for _, expected := range tt.expectedPaths {
					assert.Contains(t, paths, expected)
				}
			}
		})
	}
}

func TestNewUploadProcessor(t *testing.T) {
	workingDir := "/test/path"

	processor := NewUploadProcessor(workingDir, nil, nil, conanflex.ConanConfig{WorkingDirectory: workingDir})

	assert.NotNil(t, processor)
	assert.Equal(t, workingDir, processor.workingDir)
	assert.Equal(t, workingDir, processor.conanConfig.WorkingDirectory)
	assert.Nil(t, processor.buildConfiguration)
	assert.Nil(t, processor.serverDetails)
}

func TestHasFormatFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "No format flag",
			args:     []string{"pkg/1.0", "-r", "remote", "-c"},
			expected: false,
		},
		{
			name:     "Long form with equals",
			args:     []string{"pkg/1.0", "--format=json", "-r", "remote"},
			expected: true,
		},
		{
			name:     "Long form without value",
			args:     []string{"pkg/1.0", "--format", "json", "-r", "remote"},
			expected: true,
		},
		{
			name:     "Short form with equals",
			args:     []string{"pkg/1.0", "-f=json", "-r", "remote"},
			expected: true,
		},
		{
			name:     "Short form without value",
			args:     []string{"pkg/1.0", "-f", "json", "-r", "remote"},
			expected: true,
		},
		{
			name:     "Empty args",
			args:     []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasFormatFlag(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "No format flag",
			args:     []string{"pkg/1.0", "-r", "remote"},
			expected: "",
		},
		{
			name:     "Long form with equals (json)",
			args:     []string{"pkg/1.0", "--format=json", "-r", "remote"},
			expected: "json",
		},
		{
			name:     "Long form space-separated (json)",
			args:     []string{"pkg/1.0", "--format", "json", "-r", "remote"},
			expected: "json",
		},
		{
			name:     "Short form with equals",
			args:     []string{"pkg/1.0", "-f=json", "-r", "remote"},
			expected: "json",
		},
		{
			name:     "Short form space-separated",
			args:     []string{"pkg/1.0", "-f", "json", "-r", "remote"},
			expected: "json",
		},
		{
			name:     "Non-json format",
			args:     []string{"pkg/1.0", "--format=text", "-r", "remote"},
			expected: "text",
		},
		{
			name:     "Empty args",
			args:     []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFormatValue(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOutFilePath(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "No out-file flag",
			args:     []string{"pkg/1.0", "-r", "remote", "--format=json"},
			expected: "",
		},
		{
			name:     "Equals form",
			args:     []string{"pkg/1.0", "--out-file=/tmp/output.json", "-r", "remote"},
			expected: "/tmp/output.json",
		},
		{
			name:     "Space-separated form",
			args:     []string{"pkg/1.0", "--out-file", "/tmp/output.json", "-r", "remote"},
			expected: "/tmp/output.json",
		},
		{
			name:     "Out-file as last arg (equals)",
			args:     []string{"pkg/1.0", "-r", "remote", "--out-file=build-output.json"},
			expected: "build-output.json",
		},
		{
			name:     "Out-file as last arg (space-separated, no value)",
			args:     []string{"pkg/1.0", "--out-file"},
			expected: "",
		},
		{
			name:     "Empty args",
			args:     []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOutFilePath(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}
