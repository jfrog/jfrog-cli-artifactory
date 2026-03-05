package conan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractRemoteNameFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "Standard upload summary",
			output: `
======== Uploading to remote conan-local ========

-------- Checking server for existing packages --------
simplelib/1.0.0: Checking which revisions exist in the remote server

-------- Upload summary --------
conan-local
  simplelib/1.0.0
    revisions
      86deb56ab95f8fe27d07debf8a6ee3f9 (Uploaded)
`,
			expected: "conan-local",
		},
		{
			name: "Different remote name",
			output: `
-------- Upload summary --------
my-remote-repo
  mypackage/2.0.0
    revisions
      abc123 (Uploaded)
`,
			expected: "my-remote-repo",
		},
		{
			name: "No upload summary",
			output: `
Some other conan output
without upload summary
`,
			expected: "",
		},
		{
			name:     "Empty output",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRemoteNameFromOutput(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUploadProcessor_ParsePackageReference(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "Standard upload summary with package reference",
			output: `
-------- Upload summary --------
conan-local
  simplelib/1.0.0
    revisions
      86deb56ab95f8fe27d07debf8a6ee3f9 (Uploaded)
`,
			expected: "simplelib/1.0.0",
		},
		{
			name: "Upload summary with Conan 1.x format",
			output: `
-------- Upload summary --------
conan-local
  boost/1.82.0@myuser/stable
    revisions
      abc123 (Uploaded)
`,
			expected: "boost/1.82.0@myuser/stable",
		},
		{
			name: "Fallback to Uploading recipe pattern",
			output: `
simplelib/1.0.0: Uploading recipe 'simplelib/1.0.0#86deb56ab95f8fe27d07debf8a6ee3f9' (1.6KB)
Upload completed in 3s
`,
			expected: "simplelib/1.0.0",
		},
		{
			name:     "No package reference found",
			output:   "Some random output",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &UploadProcessor{}
			result := processor.parsePackageReference(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUploadProcessor_ParseUploadedArtifactPaths(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name: "Conan 2.x upload summary with recipe and package revision",
			output: `
======== Uploading to remote conan-local ========

-------- Upload summary --------
conan-local
  multideps/1.0.0
    revisions
      797d134a8590a1bfa06d846768443f48 (Uploaded)
        packages
          594ed0eb2e9dfcc60607438924c35871514e6c2a
            revisions
              ca858ea14c32f931e49241df0b52bec9 (Uploaded)
`,
			expected: []string{
				"_/multideps/1.0.0/_/797d134a8590a1bfa06d846768443f48/export",
				"_/multideps/1.0.0/_/797d134a8590a1bfa06d846768443f48/package/594ed0eb2e9dfcc60607438924c35871514e6c2a/ca858ea14c32f931e49241df0b52bec9",
			},
		},
		{
			name: "includes skipped artifacts that already exist in server",
			output: `
-------- Upload summary --------
conan-local
  zlib/1.2.13
    revisions
      11111111111111111111111111111111 (Skipped, already in server)
        packages
          aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
            revisions
              22222222222222222222222222222222 (Skipped, package is up to date)
`,
			expected: []string{
				"_/zlib/1.2.13/_/11111111111111111111111111111111/export",
				"_/zlib/1.2.13/_/11111111111111111111111111111111/package/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/22222222222222222222222222222222",
			},
		},
		{
			name: "Conan 1.x upload summary with user and channel",
			output: `
-------- Upload summary --------
conan-local
  boost/1.82.0@myuser/stable
    revisions
      33333333333333333333333333333333 (Uploaded)
        packages
          bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
            revisions
              44444444444444444444444444444444 (Uploaded)
`,
			expected: []string{
				"myuser/boost/1.82.0/stable/33333333333333333333333333333333/export",
				"myuser/boost/1.82.0/stable/33333333333333333333333333333333/package/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/44444444444444444444444444444444",
			},
		},
		{
			name: "no upload section returns no paths",
			output: `
Checking server for existing packages
No changes detected
`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &UploadProcessor{}
			result := processor.parseUploadedArtifactPaths(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewUploadProcessor(t *testing.T) {
	workingDir := "/test/path"

	processor := NewUploadProcessor(workingDir, nil, nil)

	assert.NotNil(t, processor)
	assert.Equal(t, workingDir, processor.workingDir)
	assert.Nil(t, processor.buildConfiguration)
	assert.Nil(t, processor.serverDetails)
}
