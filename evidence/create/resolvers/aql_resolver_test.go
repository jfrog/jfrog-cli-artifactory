package resolvers

import (
	"errors"
	"io"
	"testing"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockArtifactoryServicesManager is a mock implementation of artifactory.ArtifactoryServicesManager
type MockArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

func (m *MockArtifactoryServicesManager) Aql(query string) (io.ReadCloser, error) {
	args := m.Called(query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	readCloser, ok := args.Get(0).(io.ReadCloser)
	if !ok {
		return nil, args.Error(1)
	}
	return readCloser, args.Error(1)
}

// MockReadCloser is a mock implementation of io.ReadCloser
type MockReadCloser struct {
	mock.Mock
	data     []byte
	position int
}

func (m *MockReadCloser) Read(p []byte) (n int, err error) {
	if m.position >= len(m.data) {
		return 0, io.EOF
	}

	n = copy(p, m.data[m.position:])
	m.position += n
	return n, nil
}

func (m *MockReadCloser) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewAqlSubjectResolver(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	assert.NotNil(t, resolver)
	assert.Equal(t, mockClient, resolver.client)
}

func TestAqlSubjectResolver_Resolve_EmptyRepoName(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	subjects, err := resolver.Resolve("", "some/path", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "repository name and checksum must be provided")
}

func TestAqlSubjectResolver_Resolve_EmptyChecksum(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	subjects, err := resolver.Resolve("test-repo", "some/path", "")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "repository name and checksum must be provided")
}

func TestAqlSubjectResolver_Resolve_EmptyPath(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock the AQL query execution
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`

	mockClient.On("Aql", expectedQuery).Return(nil, errors.New("aql error"))

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_WithPath_NoRepoPrefix(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock the AQL query execution
	expectedQuery := `items.find({"repo": "test-repo", "path": {"$match" : "some/path*"},"sha256": "sha256:1234567890abcdef"})`

	mockClient.On("Aql", expectedQuery).Return(nil, errors.New("aql error"))

	subjects, err := resolver.Resolve("test-repo", "some/path", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_WithPath_WithRepoPrefix(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock the AQL query execution - path contains repo name, so wildcard should be added
	expectedQuery := `items.find({"repo": "test-repo", "path": {"$match" : "*some/path*"},"sha256": "sha256:1234567890abcdef"})`

	mockClient.On("Aql", expectedQuery).Return(nil, errors.New("aql error"))

	subjects, err := resolver.Resolve("test-repo", "test-repo/some/path", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_Success_EmptyPath(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock successful AQL query execution
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"results":[{"repo":"test-repo","path":"path/to","name":"file.txt"}]}`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.NoError(t, err)
	assert.Len(t, subjects, 1)
	assert.Equal(t, "test-repo/path/to/file.txt", subjects[0])

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_Success_WithPath(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock successful AQL query execution
	expectedQuery := `items.find({"repo": "test-repo", "path": {"$match" : "some/path*"},"sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"results":[{"repo":"test-repo","path":"some/path","name":"file.txt"},{"repo":"test-repo","path":"some/path/subdir","name":"another.txt"}]}`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "some/path", "sha256:1234567890abcdef")

	assert.NoError(t, err)
	assert.Len(t, subjects, 2)
	assert.Equal(t, "test-repo/some/path/file.txt", subjects[0])
	assert.Equal(t, "test-repo/some/path/subdir/another.txt", subjects[1])

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_Success_WithRepoPrefix(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock successful AQL query execution - path contains repo name
	expectedQuery := `items.find({"repo": "test-repo", "path": {"$match" : "*some/path*"},"sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"results":[{"repo":"test-repo","path":"test-repo/some/path","name":"file.txt"}]}`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "test-repo/some/path", "sha256:1234567890abcdef")

	assert.NoError(t, err)
	assert.Len(t, subjects, 1)
	assert.Equal(t, "test-repo/test-repo/some/path/file.txt", subjects[0])

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_NoResults(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock AQL query execution with no results
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"results":[]}`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "no subject found for repository test-repo and checksum sha256:1234567890abcdef and path ")

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_AqlError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock AQL query execution with error
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`

	mockClient.On("Aql", expectedQuery).Return(nil, errors.New("artifactory connection error"))

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_ReadError(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock AQL query execution with read error
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`

	// Create a mock stream that returns an error on read
	mockStream := &MockReadCloser{
		data: []byte("invalid data"),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_InvalidJson(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock AQL query execution with invalid JSON response
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"invalid json`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.Error(t, err)
	assert.Nil(t, subjects)
	assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		repoName    string
		path        string
		checksum    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Empty repo name",
			repoName:    "",
			path:        "some/path",
			checksum:    "sha256:1234567890abcdef",
			expectError: true,
			errorMsg:    "repository name and checksum must be provided",
		},
		{
			name:        "Empty checksum",
			repoName:    "test-repo",
			path:        "some/path",
			checksum:    "",
			expectError: true,
			errorMsg:    "repository name and checksum must be provided",
		},
		{
			name:        "Both empty",
			repoName:    "",
			path:        "some/path",
			checksum:    "",
			expectError: true,
			errorMsg:    "repository name and checksum must be provided",
		},
		{
			name:        "Path with trailing slash",
			repoName:    "test-repo",
			path:        "some/path/",
			checksum:    "sha256:1234567890abcdef",
			expectError: false,
		},
		{
			name:        "Path with leading slash",
			repoName:    "test-repo",
			path:        "/some/path",
			checksum:    "sha256:1232567890abcdef",
			expectError: false,
		},
		{
			name:        "Repo name with special characters",
			repoName:    "test-repo-123",
			path:        "some/path",
			checksum:    "sha256:1234567890abcdef",
			expectError: false,
		},
		{
			name:        "Checksum without sha256 prefix",
			repoName:    "test-repo",
			path:        "some/path",
			checksum:    "1234567890abcdef",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockArtifactoryServicesManager{}
			resolver := NewAqlSubjectResolver(mockClient)

			if !tc.expectError {
				// Mock successful response for non-error cases
				mockResponse := `{"results":[{"repo":"test-repo","path":"some/path","name":"file.txt"}]}`
				mockStream := &MockReadCloser{
					data: []byte(mockResponse),
				}
				mockStream.On("Close").Return(nil)

				mockClient.On("Aql", mock.Anything).Return(mockStream, nil)
			}

			subjects, err := resolver.Resolve(tc.repoName, tc.path, tc.checksum)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, subjects)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				// For non-error cases, we expect the mock to be called
				mockClient.AssertExpectations(t)
			}
		})
	}
}

func TestAqlSubjectResolver_Resolve_MultipleResults(t *testing.T) {
	mockClient := &MockArtifactoryServicesManager{}
	resolver := NewAqlSubjectResolver(mockClient)

	// Mock successful AQL query execution with multiple results
	expectedQuery := `items.find({"repo": "test-repo","sha256": "sha256:1234567890abcdef"})`
	mockResponse := `{"results":[
		{"repo":"test-repo","path":"path1","name":"file1.txt"},
		{"repo":"test-repo","path":"path2","name":"file2.txt"},
		{"repo":"test-repo","path":"path3","name":"file3.txt"}
	]}`

	mockStream := &MockReadCloser{
		data: []byte(mockResponse),
	}
	mockStream.On("Close").Return(nil)

	mockClient.On("Aql", expectedQuery).Return(mockStream, nil)

	subjects, err := resolver.Resolve("test-repo", "", "sha256:1234567890abcdef")

	assert.NoError(t, err)
	assert.Len(t, subjects, 3)
	assert.Equal(t, "test-repo/path1/file1.txt", subjects[0])
	assert.Equal(t, "test-repo/path2/file2.txt", subjects[1])
	assert.Equal(t, "test-repo/path3/file3.txt", subjects[2])

	mockClient.AssertExpectations(t)
	mockStream.AssertExpectations(t)
}

func TestAqlSubjectResolver_Resolve_PathNormalization(t *testing.T) {
	testCases := []struct {
		name          string
		repoName      string
		path          string
		expectedQuery string
		description   string
	}{
		{
			name:          "Path without repo prefix",
			repoName:      "test-repo",
			path:          "some/path",
			expectedQuery: `items.find({"repo": "test-repo", "path": {"$match" : "some/path*"},"sha256": "sha256:1234567890abcdef"})`,
			description:   "Path should be used as-is when it doesn't start with repo name",
		},
		{
			name:          "Path with repo prefix",
			repoName:      "test-repo",
			path:          "test-repo/some/path",
			expectedQuery: `items.find({"repo": "test-repo", "path": {"$match" : "*some/path*"},"sha256": "sha256:1234567890abcdef"})`,
			description:   "Path should have wildcard prefix when it starts with repo name",
		},
		{
			name:          "Path with repo prefix and trailing slash",
			repoName:      "test-repo",
			path:          "test-repo/some/path/",
			expectedQuery: `items.find({"repo": "test-repo", "path": {"$match" : "*some/path/*"},"sha256": "sha256:1234567890abcdef"})`,
			description:   "Path should have wildcard prefix and preserve trailing slash",
		},
		{
			name:          "Path exactly matching repo name",
			repoName:      "test-repo",
			path:          "test-repo",
			expectedQuery: `items.find({"repo": "test-repo", "path": {"$match" : "test-repo*"},"sha256": "sha256:1234567890abcdef"})`,
			description:   "Path should be used as-is when it exactly matches repo name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockArtifactoryServicesManager{}
			resolver := NewAqlSubjectResolver(mockClient)

			// Mock AQL query execution
			mockClient.On("Aql", tc.expectedQuery).Return(nil, errors.New("aql error"))

			subjects, err := resolver.Resolve(tc.repoName, tc.path, "sha256:1234567890abcdef")

			assert.Error(t, err)
			assert.Nil(t, subjects)
			assert.Contains(t, err.Error(), "failed to resolve subject, aql error")

			mockClient.AssertExpectations(t)
		})
	}
}

// Test interface implementation
func TestAqlSubjectResolver_ImplementsInterface(t *testing.T) {
	var _ AqlResolver = (*AqlSubjectResolver)(nil)
}
