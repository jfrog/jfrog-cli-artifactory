package stats

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/access"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/jpd"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Errors []APIError `json:"errors"`
}

type JPD struct {
	ID       string    `json:"id"`
	Name     string    `json:"name" log:"Name"`
	URL      string    `json:"base_url"`
	Status   Status    `json:"status"`
	Local    bool      `json:"local"`
	Services []Service `json:"services"`
	Licenses []License `json:"licenses"`
}

type Status struct {
	Code    string `json:"code" log:"Code"`
	Message string `json:"message"`
}

type Service struct {
	Type   string `json:"type"`
	Status Status `json:"status"`
}

type License struct {
	Type       string `json:"type"`
	Expired    bool   `json:"expired"`
	LicensedTo string `json:"licensed_to"`
}

type ArtifactoryStatsSummary struct {
	ProjectsCount       int                          `log:"Total Projects"`
	TotalBinariesCount  string                       `log:"Total No of Binaries"`
	TotalBinariesSize   string                       `log:"Total Binaries Size"`
	TotalArtifactsCount string                       `log:"Total No of Artifacts"`
	TotalArtifactsSize  string                       `log:"Total Artifacts Size"`
	StorageType         string                       `log:"Storage Type"`
	RepositoriesDetails []services.RepositoryDetails `json:"-"`
}

type ReleaseBundleResponse struct {
	ReleaseBundles []ReleaseBundleInfo `json:"release_bundles"`
}

type ReleaseBundleInfo struct {
	RepositoryKey     string `json:"repository_key" log:"Repository Key"`
	ReleaseBundleName string `json:"release_bundle_name" log:"Release Bundle Name"`
	ProjectKey        string `json:"project_key" log:"Project Key"`
}

type ArtifactoryStats struct {
	ServicesManager         artifactory.ArtifactoryServicesManager
	AccessManager           access.AccessServicesManager
	LifecycleServiceManager lifecycle.LifecycleServicesManager
	JPDServicesManager      jpd.JPDServicesManager
	Format                  string
	AccessToken             string
	ServerId                string
	ServerUrl               string
	DisplayLimit            int
	ProjectCount            int
}

func NewArtifactoryStatsCommand() *ArtifactoryStats {
	return &ArtifactoryStats{}
}

func (sa *ArtifactoryStats) SetFormat(format string) *ArtifactoryStats {
	sa.Format = format
	return sa
}

func (sa *ArtifactoryStats) SetAccessToken(token string) *ArtifactoryStats {
	sa.AccessToken = token
	return sa
}

func (sa *ArtifactoryStats) SetServerId(id string) *ArtifactoryStats {
	sa.ServerId = id
	return sa
}

func (sa *ArtifactoryStats) SetDisplayLimit(displayLimit int) *ArtifactoryStats {
	sa.DisplayLimit = displayLimit
	return sa
}

func (sa *ArtifactoryStats) Run() error {
	serverDetails, err := config.GetSpecificConfig(sa.ServerId, true, false)
	if err != nil {
		return err
	}
	if sa.AccessToken != "" {
		serverDetails.AccessToken = sa.AccessToken
	}
	servicesManager, err := utils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return err
	}
	sa.ServicesManager = servicesManager
	accessManager, err := utils.CreateAccessServiceManager(serverDetails, false)
	if err != nil {
		return err
	}
	sa.AccessManager = *accessManager
	lifecycleServicesManager, err := utils.CreateLifecycleServiceManager(serverDetails, false)
	if err != nil {
		return err
	}
	sa.LifecycleServiceManager = *lifecycleServicesManager
	jpdServiceManager, err := utils.CreateJPDServiceManager(serverDetails, false)
	if err != nil {
		return err
	}
	sa.JPDServicesManager = *jpdServiceManager
	sa.ServerUrl = serverDetails.Url
	err = sa.GetStats()
	if err != nil {
		return err
	}
	return nil
}

type StatsFunc func() interface{}

func (sa *ArtifactoryStats) GetCommandList() map[string]StatsFunc {
	return map[string]StatsFunc{
		"project": sa.GetProjectsStats,
		"rb":      sa.GetReleaseBundlesStats,
		"jpd":     sa.GetJPDsStats,
		"rt":      sa.GetArtifactoryStats,
	}
}

func (sa *ArtifactoryStats) GetStats() error {
	commandList := sa.GetCommandList()
	allResultsMap := make(map[string]interface{}, 0)
	for name, statsFunc := range commandList {
		allResultsMap[name] = statsFunc()
	}
	return sa.PrintAllResults(allResultsMap)
}

func (sa *ArtifactoryStats) PrintAllResults(results map[string]interface{}) error {
	printOrder := []string{"rt", "jpd", "rb", "project"}
	for _, stats := range printOrder {
		err := NewGenericResultsWriter(results[stats], sa.Format, sa.DisplayLimit).Print()
		if err != nil {
			log.Error("Failed to print result:", err)
		}
	}
	return nil
}

func (sa *ArtifactoryStats) GetArtifactoryStats() interface{} {
	var artifactoryStatsSummary ArtifactoryStatsSummary
	storageInfo, err := sa.ServicesManager.GetStorageInfo()
	if err != nil {
		wrappedError := fmt.Errorf("failed to build ARTIFACTORY API endpoint: %w", err)
		return jpd.NewGenericError("ARTIFACTORY", wrappedError)
	}
	artifactoryStatsSummary.TotalArtifactsCount = storageInfo.ArtifactsCount
	artifactoryStatsSummary.TotalArtifactsSize = storageInfo.ArtifactsSize
	artifactoryStatsSummary.TotalBinariesCount = storageInfo.BinariesCount
	artifactoryStatsSummary.TotalBinariesSize = storageInfo.BinariesSize
	artifactoryStatsSummary.StorageType = storageInfo.StorageType
	repositoriesDetails, err := sa.ServicesManager.GetAllRepositories()
	if err != nil {
		wrappedError := fmt.Errorf("failed to call ARTIFACTORY API: %w", err)
		return jpd.NewGenericError("ARTIFACTORY", wrappedError)
	}
	artifactoryStatsSummary.RepositoriesDetails = *repositoriesDetails
	artifactoryStatsSummary.ProjectsCount = sa.ProjectCount
	return &artifactoryStatsSummary
}

func (sa *ArtifactoryStats) GetProjectsStats() interface{} {
	projects, err := sa.AccessManager.GetAllProjects()
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			wrappedError := fmt.Errorf("Need Admin privileges")
			return jpd.NewGenericError("PROJECTS", wrappedError)
		}
		wrappedError := fmt.Errorf("failed to call PROJECTS API: %w", err)
		return jpd.NewGenericError("PROJECTS", wrappedError)
	}
	sa.ProjectCount = len(projects)
	return projects
}

func (sa *ArtifactoryStats) GetJPDsStats() interface{} {
	body, err := sa.JPDServicesManager.GetJPDsStats(sa.ServerUrl)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			wrappedError := fmt.Errorf("Need Admin privileges")
			return jpd.NewGenericError("JPDs", wrappedError)
		}
		wrappedError := fmt.Errorf("failed to call JPDs API: %w", err)
		return jpd.NewGenericError("JPDs", wrappedError)
	}

	var errorResp ErrorResponse
	if unmarshalErr := json.Unmarshal(body, &errorResp); unmarshalErr == nil {
		if len(errorResp.Errors) > 0 {
			// It is an API error response, convert to GenericError
			wrappedError := fmt.Errorf("%s: %s", errorResp.Errors[0].Type, errorResp.Errors[0].Message)
			return jpd.NewGenericError("JPDs", wrappedError)
		}
	}

	var jpdList []JPD
	if err := json.Unmarshal(body, &jpdList); err != nil {
		wrappedError := fmt.Errorf("error parsing JPDs JSON: %w", err)
		return jpd.NewGenericError("JPDs", wrappedError)
	}
	return &jpdList
}

func (sa *ArtifactoryStats) GetReleaseBundlesStats() interface{} {
	body, err := sa.LifecycleServiceManager.GetReleaseBundlesStats(sa.ServerUrl)
	if err != nil {
		wrappedError := fmt.Errorf("failed to call RELEASE-BUNDLES API endpoint: %w", err)
		return jpd.NewGenericError("RELEASE-BUNDLES", wrappedError)
	}
	var releaseBundles ReleaseBundleResponse // ReleaseBundleResponse is in model.go
	if err := json.Unmarshal(body, &releaseBundles); err != nil {
		wrappedError := fmt.Errorf("error parsing RELEASE-BUNDLES JSON: %w", err)
		return jpd.NewGenericError("RELEASE-BUNDLES", wrappedError)
	}
	return &releaseBundles
}
