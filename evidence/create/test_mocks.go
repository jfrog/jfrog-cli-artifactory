package create

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/http/jfroghttpclient"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// BaseMockArtifactoryServicesManager provides a base mock implementation
// that can be embedded and selectively overridden for specific test cases
type BaseMockArtifactoryServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

// SimpleMockServicesManager is a simplified mock that only implements commonly used methods
type SimpleMockServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	// Define fields for common return values
	FileInfoFunc     func(string) (*utils.FileInfo, error)
	GetBuildInfoFunc func(services.BuildInfoParams) (*entities.PublishedBuildInfo, bool, error)
	AqlFunc          func(string) (io.ReadCloser, error)
}

func (m *SimpleMockServicesManager) FileInfo(path string) (*utils.FileInfo, error) {
	if m.FileInfoFunc != nil {
		return m.FileInfoFunc(path)
	}
	// Default implementation
	return &utils.FileInfo{
		Checksums: struct {
			Sha1   string `json:"sha1,omitempty"`
			Sha256 string `json:"sha256,omitempty"`
			Md5    string `json:"md5,omitempty"`
		}{Sha256: "default-sha256"},
		Uri: path,
	}, nil
}

func (m *SimpleMockServicesManager) GetBuildInfo(params services.BuildInfoParams) (*entities.PublishedBuildInfo, bool, error) {
	if m.GetBuildInfoFunc != nil {
		return m.GetBuildInfoFunc(params)
	}
	// Default implementation
	return &entities.PublishedBuildInfo{
		BuildInfo: entities.BuildInfo{
			Started: time.Now().Format(time.RFC3339),
		},
	}, true, nil
}

func (m *SimpleMockServicesManager) Aql(query string) (io.ReadCloser, error) {
	if m.AqlFunc != nil {
		return m.AqlFunc(query)
	}
	// Default empty result
	return io.NopCloser(strings.NewReader(`{"results":[]}`)), nil
}

// SimpleServiceDetails provides a minimal implementation of auth.ServiceDetails
type SimpleServiceDetails struct {
	url         string
	user        string
	password    string
	accessToken string
	client      *jfroghttpclient.JfrogHttpClient
}

func (s *SimpleServiceDetails) GetUrl() string                                                    { return s.url }
func (s *SimpleServiceDetails) GetUser() string                                                   { return s.user }
func (s *SimpleServiceDetails) GetPassword() string                                               { return s.password }
func (s *SimpleServiceDetails) GetAccessToken() string                                            { return s.accessToken }
func (s *SimpleServiceDetails) IsSshAuthHeaderSet() bool                                          { return false }
func (s *SimpleServiceDetails) GetSshPassphrase() string                                          { return "" }
func (s *SimpleServiceDetails) GetSshKeyPath() string                                             { return "" }
func (s *SimpleServiceDetails) GetSshAuthHeaders() map[string]string                              { return nil }
func (s *SimpleServiceDetails) GetClientCertPath() string                                         { return "" }
func (s *SimpleServiceDetails) GetClientCertKeyPath() string                                      { return "" }
func (s *SimpleServiceDetails) SetSshAuthHeaders(map[string]string)                               {}
func (s *SimpleServiceDetails) SetUrl(url string)                                                 { s.url = url }
func (s *SimpleServiceDetails) SetUser(user string)                                               { s.user = user }
func (s *SimpleServiceDetails) SetPassword(password string)                                       { s.password = password }
func (s *SimpleServiceDetails) SetAccessToken(token string)                                       { s.accessToken = token }
func (s *SimpleServiceDetails) SetSshPassphrase(string)                                           {}
func (s *SimpleServiceDetails) SetSshKeyPath(string)                                              {}
func (s *SimpleServiceDetails) SetClientCertPath(string)                                          {}
func (s *SimpleServiceDetails) SetClientCertKeyPath(string)                                       {}
func (s *SimpleServiceDetails) AppendPreRequestInterceptor(interceptor func(*http.Request) error) {}
func (s *SimpleServiceDetails) RunPreRequestInterceptors(*http.Request) error                     { return nil }
func (s *SimpleServiceDetails) CreateHttpClientDetails() httputils.HttpClientDetails {
	return httputils.HttpClientDetails{User: s.user, Password: s.password}
}
func (s *SimpleServiceDetails) GetClient() (*jfroghttpclient.JfrogHttpClient, error) {
	return s.client, nil
}
func (s *SimpleServiceDetails) SetDialTimeout(time.Duration)           {}
func (s *SimpleServiceDetails) SetOverallRequestTimeout(time.Duration) {}

// FileInfoBuilder helps create FileInfo objects for tests
type FileInfoBuilder struct {
	fileInfo *utils.FileInfo
}

func NewFileInfoBuilder() *FileInfoBuilder {
	return &FileInfoBuilder{
		fileInfo: &utils.FileInfo{
			Checksums: struct {
				Sha1   string `json:"sha1,omitempty"`
				Sha256 string `json:"sha256,omitempty"`
				Md5    string `json:"md5,omitempty"`
			}{},
		},
	}
}

func (b *FileInfoBuilder) WithPath(path string) *FileInfoBuilder {
	b.fileInfo.Uri = path
	return b
}

func (b *FileInfoBuilder) WithSha256(sha256 string) *FileInfoBuilder {
	b.fileInfo.Checksums.Sha256 = sha256
	return b
}

func (b *FileInfoBuilder) Build() *utils.FileInfo {
	return b.fileInfo
}

// BuildInfoBuilder helps create BuildInfo objects for tests
type BuildInfoBuilder struct {
	buildInfo *entities.PublishedBuildInfo
}

func NewBuildInfoBuilder() *BuildInfoBuilder {
	return &BuildInfoBuilder{
		buildInfo: &entities.PublishedBuildInfo{
			BuildInfo: entities.BuildInfo{},
		},
	}
}

func (b *BuildInfoBuilder) WithStarted(started string) *BuildInfoBuilder {
	b.buildInfo.BuildInfo.Started = started
	return b
}

func (b *BuildInfoBuilder) Build() *entities.PublishedBuildInfo {
	return b.buildInfo
}

// PrepareMockArtifactoryManagerForBuildTests creates a mock with standard build test behavior
func PrepareMockArtifactoryManagerForBuildTests() *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		FileInfoFunc: func(_ string) (*utils.FileInfo, error) {
			return NewFileInfoBuilder().
				WithSha256("dummy_sha256").
				Build(), nil
		},
		GetBuildInfoFunc: func(services.BuildInfoParams) (*entities.PublishedBuildInfo, bool, error) {
			return NewBuildInfoBuilder().
				WithStarted("2024-01-17T15:04:05.000-0700").
				Build(), true, nil
		},
	}
}

// PrepareMockWithBuildError creates a mock that returns errors for build operations
func PrepareMockWithBuildError(buildErr error, ok bool, started string) *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		GetBuildInfoFunc: func(services.BuildInfoParams) (*entities.PublishedBuildInfo, bool, error) {
			if buildErr != nil {
				return nil, false, buildErr
			}
			return NewBuildInfoBuilder().
				WithStarted(started).
				Build(), ok, nil
		},
		FileInfoFunc: func(_ string) (*utils.FileInfo, error) {
			return &utils.FileInfo{}, nil
		},
	}
}

// PrepareMockWithFileInfoError creates a mock that returns error for FileInfo
func PrepareMockWithFileInfoError() *SimpleMockServicesManager {
	return &SimpleMockServicesManager{
		GetBuildInfoFunc: func(services.BuildInfoParams) (*entities.PublishedBuildInfo, bool, error) {
			return NewBuildInfoBuilder().
				WithStarted("2024-01-17T15:04:05.000-0700").
				Build(), true, nil
		},
		FileInfoFunc: func(_ string) (*utils.FileInfo, error) {
			return nil, assert.AnError
		},
	}
}
