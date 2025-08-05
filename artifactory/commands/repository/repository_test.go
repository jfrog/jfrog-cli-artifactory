package repository

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PerformRepoCmd_SingleRepository(t *testing.T) {
	tests := []struct {
		name           string
		templatePath   string
		vars           string
		isUpdate       bool
		expectedRepo   services.RepositoryBaseParams
		expectedStatus int
		expErr         error
	}{
		{
			name:         "Create Maven local repository with multiple environments",
			templatePath: createTempTemplate(t, singleRepoTemplate),
			vars:         `REPO_KEY=test-maven-local;RCLASS=local;PACKAGE_TYPE=maven;DESCRIPTION=Test Maven repo;ENVIRONMENTS=PROD,DEV`,
			isUpdate:     false,
			expectedRepo: services.RepositoryBaseParams{
				Key:          "test-maven-local",
				Rclass:       "local",
				PackageType:  "maven",
				Description:  "Test Maven repo",
				Environments: []string{"PROD", "DEV"},
			},
			expectedStatus: http.StatusOK,
			expErr:         nil,
		},
		{
			name:         "Create Maven local repository with single environment",
			templatePath: createTempTemplate(t, singleRepoTemplate),
			vars:         `REPO_KEY=test-maven-local-single;RCLASS=local;PACKAGE_TYPE=maven;DESCRIPTION=Test Maven repo;ENVIRONMENTS=PROD`,
			isUpdate:     false,
			expectedRepo: services.RepositoryBaseParams{
				Key:          "test-maven-local-single",
				Rclass:       "local",
				PackageType:  "maven",
				Description:  "Test Maven repo",
				Environments: []string{"PROD"},
			},
			expectedStatus: http.StatusOK,
			expErr:         nil,
		},
		{
			name:         "Update single Maven local repository",
			templatePath: createTempTemplate(t, singleRepoTemplate),
			vars:         `REPO_KEY=test-maven-local;RCLASS=local;PACKAGE_TYPE=maven;DESCRIPTION=Updated Maven repo;ENVIRONMENTS=PROD,DEV`,
			isUpdate:     true,
			expectedRepo: services.RepositoryBaseParams{
				Key:          "test-maven-local",
				Rclass:       "local",
				PackageType:  "maven",
				Description:  "Updated Maven repo",
				Environments: []string{"PROD", "DEV"},
			},
			expectedStatus: http.StatusOK,
			expErr:         nil,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.expectedStatus)

				content, err := io.ReadAll(r.Body)
				require.NoError(t, err)

				var actual services.RepositoryBaseParams
				err = json.Unmarshal(content, &actual)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedRepo.Key, actual.Key)
				assert.Equal(t, tt.expectedRepo.Rclass, actual.Rclass)
				assert.Equal(t, tt.expectedRepo.PackageType, actual.PackageType)
				assert.Equal(t, tt.expectedRepo.Description, actual.Description)
			}))
			defer testServer.Close()
			repoCmd := &RepoCommand{
				serverDetails: &config.ServerDetails{ArtifactoryUrl: testServer.URL + "/"},
				templatePath:  tt.templatePath,
				vars:          tt.vars,
			}

			err := repoCmd.PerformRepoCmd(tt.isUpdate)

			assert.Equal(t, tt.expErr, err, "Testcase %v failed , expected error: %v, got: %v", i+1, tt.expErr, err)
		})
	}
}

func Test_PerformRepoCmd_MultipleRepositories(t *testing.T) {
	tests := []struct {
		name           string
		templatePath   string
		vars           string
		isUpdate       bool
		expectedRepos  []services.RepositoryBaseParams
		expectedStatus int
		expErr         error
	}{
		{
			name:         "Create multiple repositories",
			templatePath: createTempTemplate(t, multipleReposTemplate),
			vars:         "MAVEN_REPO=test-maven;DOCKER_REPO=test-docker;NPM_REPO=test-npm",
			isUpdate:     false,
			expectedRepos: []services.RepositoryBaseParams{
				{
					Key:         "test-maven",
					Rclass:      "local",
					PackageType: "maven",
					Description: "Maven repository",
				},
				{
					Key:         "test-docker",
					Rclass:      "local",
					PackageType: "docker",
					Description: "Docker repository",
				},
				{
					Key:         "test-npm",
					Rclass:      "local",
					PackageType: "npm",
					Description: "NPM repository",
				},
			},
			expectedStatus: http.StatusOK,
			expErr:         nil,
		},
		{
			name:         "Update multiple repositories",
			templatePath: createTempTemplate(t, multipleReposTemplate),
			vars:         "MAVEN_REPO=test-maven;DOCKER_REPO=test-docker;NPM_REPO=test-npm",
			isUpdate:     true,
			expectedRepos: []services.RepositoryBaseParams{
				{
					Key:         "test-maven",
					Rclass:      "local",
					PackageType: "maven",
					Description: "Maven repository",
				},
				{
					Key:         "test-docker",
					Rclass:      "local",
					PackageType: "docker",
					Description: "Docker repository",
				},
				{
					Key:         "test-npm",
					Rclass:      "local",
					PackageType: "npm",
					Description: "NPM repository",
				},
			},
			expectedStatus: http.StatusOK,
			expErr:         nil,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/system/version":
					w.WriteHeader(http.StatusOK)
					_, err := w.Write([]byte(`{"version":"7.104.2"}`))
					require.NoError(t, err)
				case "/api/v2/repositories/batch":
					if r.Method == http.MethodPut {
						w.WriteHeader(http.StatusCreated)
					} else {
						w.WriteHeader(http.StatusOK)
					}

					content, err := io.ReadAll(r.Body)
					require.NoError(t, err)

					var actualRepos []services.RepositoryBaseParams
					err = json.Unmarshal(content, &actualRepos)
					require.NoError(t, err)

					assert.Len(t, actualRepos, len(tt.expectedRepos))
					for i, expected := range tt.expectedRepos {
						assert.Equal(t, expected.Key, actualRepos[i].Key)
						assert.Equal(t, expected.Rclass, actualRepos[i].Rclass)
						assert.Equal(t, expected.PackageType, actualRepos[i].PackageType)
						assert.Equal(t, expected.Description, actualRepos[i].Description)
					}
				default:
					w.WriteHeader(tt.expectedStatus)

					content, err := io.ReadAll(r.Body)
					require.NoError(t, err)

					var actual services.RepositoryBaseParams
					err = json.Unmarshal(content, &actual)
					require.NoError(t, err)
				}
			}))
			defer testServer.Close()

			repoCmd := &RepoCommand{
				serverDetails: &config.ServerDetails{ArtifactoryUrl: testServer.URL + "/"},
				templatePath:  tt.templatePath,
				vars:          tt.vars,
			}

			err := repoCmd.PerformRepoCmd(tt.isUpdate)

			assert.Equal(t, tt.expErr, err, "Testcase %v failed , expected error: %v, got: %v", i+1, tt.expErr, err)
		})
	}
}

func Test_PerformRepoCmd_ErrorCases(t *testing.T) {
	tests := []struct {
		name         string
		templatePath string
		vars         string
		isUpdate     bool
		expErr       string
	}{
		{
			name:         "Invalid JSON template",
			templatePath: createTempTemplate(t, invalidJsonTemplate),
			vars:         "REPO_KEY=test-repo",
			isUpdate:     false,
			expErr:       "invalid character",
		},
		{
			name:         "Missing required fields",
			templatePath: createTempTemplate(t, missingFieldsTemplate),
			vars:         "REPO_KEY=test-repo",
			isUpdate:     false,
			expErr:       "'key' is missing in the following configs",
		},
		{
			name:         "Unsupported package type",
			templatePath: createTempTemplate(t, unsupportedPackageTemplate),
			vars:         "REPO_KEY=test-repo",
			isUpdate:     false,
			expErr:       "unsupported package type",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoCmd := &RepoCommand{
				serverDetails: &config.ServerDetails{ArtifactoryUrl: "http://invalid-server:8081/"},
				templatePath:  tt.templatePath,
				vars:          tt.vars,
			}

			err := repoCmd.PerformRepoCmd(tt.isUpdate)

			if err != nil {
				assert.Contains(t, err.Error(), tt.expErr, "Testcase %v failed , expected error message to contain: %v, got: %v", i+1, tt.expErr, err.Error())
			} else {
				assert.Fail(t, "Testcase %v failed, expected error but got nil", i+1)
			}
		})
	}
}

func createTempTemplate(t *testing.T, content string) string {
	tmpFile, err := os.CreateTemp("", "repo-template-*.json")
	require.NoError(t, err)
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	return tmpFile.Name()
}

const singleRepoTemplate = `{
  "key": "${REPO_KEY}",
  "rclass": "${RCLASS}",
  "packageType": "${PACKAGE_TYPE}",
  "description": "${DESCRIPTION}",
  "environments": "${ENVIRONMENTS}"
}`

const multipleReposTemplate = `[
  {
    "key": "${MAVEN_REPO}",
    "rclass": "local",
    "packageType": "maven",
    "description": "Maven repository"
  },
  {
    "key": "${DOCKER_REPO}",
    "rclass": "local",
    "packageType": "docker",
    "description": "Docker repository"
  },
  {
    "key": "${NPM_REPO}",
    "rclass": "local",
    "packageType": "npm",
    "description": "NPM repository"
  }
]`

const invalidJsonTemplate = `{
  "key": "${REPO_KEY}",
  "rclass": "${RCLASS}",
  "packageType": "${PACKAGE_TYPE}",
  "description": "${DESCRIPTION}",
  invalid json here
}`

const missingFieldsTemplate = `{
  "description": "${DESCRIPTION}"
}`

const unsupportedPackageTemplate = `{
  "key": "${REPO_KEY}",
  "rclass": "local",
  "packageType": "unsupported",
  "description": "${DESCRIPTION}"
}`
