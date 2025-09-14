package resolvers

import (
	"testing"
	"time"

	"github.com/jfrog/jfrog-client-go/auth"
	"github.com/jfrog/jfrog-client-go/http/jfroghttpclient"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/stretchr/testify/assert"
)

func TestBuildUrl(t *testing.T) {
	mockServiceDetails := &mockServiceDetails{
		url: "https://test.rt.io",
	}

	testCases := []struct {
		name     string
		subject  string
		checksum string
		expected string
	}{
		{
			name:     "Docker subject with explicit registry",
			subject:  "test.jfrog.io/local-test-repo/local-image-a:v2.0",
			checksum: "sha256:1234567890abcdef",
			expected: "https://test.jfrog.io/v2/local-test-repo/local-image-a/manifests/sha256:1234567890abcdef",
		},
		{
			name:     "Docker subject without registry (should use Artifactory domain)",
			subject:  "local-test-registry/local-image-a:v2.0",
			checksum: "sha256:1234567890abcdef",
			expected: "https://local-test-registry/v2/local-image-a/manifests/sha256:1234567890abcdef",
		},
		{
			name:     "Docker subject with reversed format (docker.jfrog.dev/sigs:tag)",
			subject:  "docker.jfrog.dev/sigs:07bfa335a98e6df33f4c5f933553cb4d2bc7c00211e6dc52341289645496dd9d",
			checksum: "sha256:1234567890abcdef",
			expected: "https://docker.jfrog.dev/v2/sigs/manifests/sha256:1234567890abcdef",
		},
		{
			name:     "Empty domain uses service details URL",
			subject:  "my-image:latest",
			checksum: "sha256:1234567890abcdef",
			expected: "https://test.rt.io/v2/my-image/manifests/sha256:1234567890abcdef",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			domain, path, err := parseOciSubject(tc.subject)
			assert.NoError(t, err, "should parse subject successfully")
			result, err := buildContainerUrl(domain, path, tc.checksum, mockServiceDetails)
			assert.NoError(t, err, "buildContainerUrl should not return error")
			assert.Equal(t, tc.expected, result, "should build expected URL")
		})
	}
}

type mockServiceDetails struct {
	url string
}

func (m *mockServiceDetails) GetUrl() string {
	return m.url
}

func (m *mockServiceDetails) GetUser() string                                              { return "" }
func (m *mockServiceDetails) GetPassword() string                                          { return "" }
func (m *mockServiceDetails) GetApiKey() string                                            { return "" }
func (m *mockServiceDetails) GetAccessToken() string                                       { return "" }
func (m *mockServiceDetails) GetPreRequestFunctions() []auth.ServiceDetailsPreRequestFunc  { return nil }
func (m *mockServiceDetails) GetClientCertPath() string                                    { return "" }
func (m *mockServiceDetails) GetClientCertKeyPath() string                                 { return "" }
func (m *mockServiceDetails) GetSshUrl() string                                            { return "" }
func (m *mockServiceDetails) GetSshKeyPath() string                                        { return "" }
func (m *mockServiceDetails) GetSshPassphrase() string                                     { return "" }
func (m *mockServiceDetails) GetSshAuthHeaders() map[string]string                         { return nil }
func (m *mockServiceDetails) GetClient() *jfroghttpclient.JfrogHttpClient                  { return nil }
func (m *mockServiceDetails) GetVersion() (string, error)                                  { return "", nil }
func (m *mockServiceDetails) SetUrl(url string)                                            { m.url = url }
func (m *mockServiceDetails) SetUser(user string)                                          {}
func (m *mockServiceDetails) SetPassword(password string)                                  {}
func (m *mockServiceDetails) SetApiKey(apiKey string)                                      {}
func (m *mockServiceDetails) SetAccessToken(accessToken string)                            {}
func (m *mockServiceDetails) AppendPreRequestFunction(auth.ServiceDetailsPreRequestFunc)   {}
func (m *mockServiceDetails) SetClientCertPath(certificatePath string)                     {}
func (m *mockServiceDetails) SetClientCertKeyPath(certificatePath string)                  {}
func (m *mockServiceDetails) SetSshUrl(url string)                                         {}
func (m *mockServiceDetails) SetSshKeyPath(sshKeyPath string)                              {}
func (m *mockServiceDetails) SetSshPassphrase(sshPassphrase string)                        {}
func (m *mockServiceDetails) SetSshAuthHeaders(sshAuthHeaders map[string]string)           {}
func (m *mockServiceDetails) SetClient(client *jfroghttpclient.JfrogHttpClient)            {}
func (m *mockServiceDetails) SetDialTimeout(dialTimeout time.Duration)                     {}
func (m *mockServiceDetails) SetOverallRequestTimeout(overallRequestTimeout time.Duration) {}
func (m *mockServiceDetails) IsSshAuthHeaderSet() bool                                     { return false }
func (m *mockServiceDetails) IsSshAuthentication() bool                                    { return false }
func (m *mockServiceDetails) AuthenticateSsh(sshKey, sshPassphrase string) error           { return nil }
func (m *mockServiceDetails) InitSsh() error                                               { return nil }
func (m *mockServiceDetails) RunPreRequestFunctions(httpClientDetails *httputils.HttpClientDetails) error {
	return nil
}
func (m *mockServiceDetails) CreateHttpClientDetails() httputils.HttpClientDetails {
	return httputils.HttpClientDetails{}
}
