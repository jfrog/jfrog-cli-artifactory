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

func TestNewUploadProcessor(t *testing.T) {
	workingDir := "/test/path"

	processor := NewUploadProcessor(workingDir, nil, nil)

	assert.NotNil(t, processor)
	assert.Equal(t, workingDir, processor.workingDir)
	assert.Nil(t, processor.buildConfiguration)
	assert.Nil(t, processor.serverDetails)
}
