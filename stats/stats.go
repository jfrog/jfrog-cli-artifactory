package stats

import (
	"encoding/json"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/access"
	"github.com/jfrog/jfrog-client-go/access/services"
	"github.com/jfrog/jfrog-client-go/artifactory"
	clientServices "github.com/jfrog/jfrog-client-go/artifactory/services"
	clientUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/jpd"
	"github.com/jfrog/jfrog-client-go/lifecycle"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"strings"
)

var displayLimit int

type StatsArtifactory struct {
	ServicesManager         artifactory.ArtifactoryServicesManager
	AccessManager           access.AccessServicesManager
	LifecycleServiceManager lifecycle.LifecycleServicesManager
	JPDServicesManager      jpd.JPDServicesManager
	FilterName              string
	FormatOutput            string
	AccessToken             string
	ServerId                string
	ServerUrl               string
	DisplayLimit            int
}

func NewArtifactoryStatsCommand() *StatsArtifactory {
	return &StatsArtifactory{}
}

func (sa *StatsArtifactory) SetFilterName(name string) *StatsArtifactory {
	sa.FilterName = name
	return sa
}

func (sa *StatsArtifactory) SetFormatOutput(format string) *StatsArtifactory {
	sa.FormatOutput = format
	return sa
}

func (sa *StatsArtifactory) SetAccessToken(token string) *StatsArtifactory {
	sa.AccessToken = token
	return sa
}

func (sa *StatsArtifactory) SetServerId(id string) *StatsArtifactory {
	sa.ServerId = id
	return sa
}

func (sa *StatsArtifactory) SetDisplayLimit(displayLimit int) *StatsArtifactory {
	sa.DisplayLimit = displayLimit
	return sa
}

func (sa *StatsArtifactory) Run() error {
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

	displayLimit = sa.DisplayLimit

	sa.ServerUrl = serverDetails.Url

	err = sa.GetStats()
	if err != nil {
		return err
	}
	return nil
}

type ArtifactoryInfo struct {
	StorageInfo         clientUtils.StorageInfo
	RepositoriesDetails []clientServices.RepositoryDetails `json:"-"`
	ProjectsCount       int
}

type ProjectResources struct {
	Resources []Resource `json:"resources"`
}

type Resource struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	BinMgrID string `json:"bin_mgr_id"`
}

type AdminPrivileges struct {
	ManageMembers   bool `json:"manage_members"`
	ManageResources bool `json:"manage_resources"`
	IndexResources  bool `json:"index_resources"`
}

type JPD struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	URL      string    `json:"base_url"`
	Status   Status    `json:"status"`
	Local    bool      `json:"local"`
	Services []Service `json:"services"`
	Licenses []License `json:"licenses"`
}

func (j JPD) String() string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Name: %s\n", j.Name))
	builder.WriteString(fmt.Sprintf("URL: %s\n", j.URL))
	builder.WriteString(fmt.Sprintf("Status: %s\n", j.Status.Code))
	builder.WriteString(fmt.Sprintf("Detailed Status: %s\n", j.Status.Message))
	builder.WriteString(fmt.Sprintf("Local: %t\n", j.Local))
	return builder.String()
}

type Status struct {
	Code    string `json:"code"`
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

type RepositoryDetails struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	PackageType string `json:"packageType"`
}

type ReleaseBundleResponse struct {
	ReleaseBundles []ReleaseBundleInfo `json:"release_bundles"`
}

type ReleaseBundleInfo struct {
	RepositoryKey     string `json:"repository_key"`
	ReleaseBundleName string `json:"release_bundle_name"`
	ProjectKey        string `json:"project_key"`
}

func (rb ReleaseBundleInfo) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("ReleaseBundleName: %s\n", rb.ReleaseBundleName))
	builder.WriteString(fmt.Sprintf("RepositoryKey: %s\n", rb.RepositoryKey))
	builder.WriteString(fmt.Sprintf("ProjectKey: %s\n", rb.ProjectKey))

	return builder.String()
}

type GenericResultsWriter struct {
	data   interface{}
	format string
}

func NewGenericResultsWriter(data interface{}, format string) *GenericResultsWriter {
	return &GenericResultsWriter{
		data:   data,
		format: format,
	}
}

type StatsFunc func() (interface{}, error)

func (sa *StatsArtifactory) GetCommandMap() map[string]StatsFunc {
	return map[string]StatsFunc{
		"rb":  sa.GetReleaseBundlesStats,
		"jpd": sa.GetJPDsStats,
		"rt":  sa.GetArtifactoryStats,
		"pj":  sa.GetProjectsStats,
	}
}

var processingOrders = []string{"pj", "rt", "jpd", "rb"}

var printingOrders = []string{"rt", "pj", "jpd", "rb"}

func (sa *StatsArtifactory) GetStats() error {

	commandMap := sa.GetCommandMap()

	allResultsMap := make(map[string]interface{})

	filter := sa.FilterName

	if filter != "" {
		_, ok := commandMap[filter]
		if !ok {
			return fmt.Errorf("unknown filter: %s", filter)
		}
	}

	if filter != "" {
		allResultsMap[filter] = GetStatsUsingFilter(commandMap[filter])
		if filter == "rt" {
			allResultsMap["pj"] = GetStatsUsingFilter(commandMap["pj"])
			updateProjectInArtifactoryInfo(&allResultsMap)
			delete(allResultsMap, "pj")
		}
	} else {
		for _, filter = range processingOrders {
			allResultsMap[filter] = GetStatsUsingFilter(commandMap[filter])
		}
		updateProjectInArtifactoryInfo(&allResultsMap)
	}
	return sa.PrintAllResults(allResultsMap)
}

func (sa *StatsArtifactory) PrintAllResults(results map[string]interface{}) error {
	for _, product := range printingOrders {
		result, ok := results[product]
		if ok {
			err := NewGenericResultsWriter(result, sa.FormatOutput).Print()
			if err != nil {
				log.Error("Failed to print result:", err)
			}
		}
	}
	return nil
}

func GetStatsUsingFilter(commandAPI StatsFunc) interface{} {
	body, err := commandAPI()
	if err != nil {
		return err
	}
	return body
}

func (rw *GenericResultsWriter) Print() error {
	switch rw.format {
	case "json", "simplejson":
		return rw.PrintJson()
	case "table":
		return rw.PrintDashboard()
	default:
		return rw.PrintSimple()
	}
}

func (rw *GenericResultsWriter) PrintJson() error {
	if rw.data == nil {
		return nil
	}

	jsonBytes, err := json.MarshalIndent(rw.data, "", "  ")
	if err != nil {
		return err
	}
	result := string(jsonBytes)
	if len(result) <= 2 {
		msg := ""
		switch rw.data.(type) {
		case *ArtifactoryInfo:
			msg = "Artifacts: No Artifacts Available"
		case []services.Project:
			msg = "Projects: No Project Available"
		case *[]JPD:
			msg = "JPDs: No JPD Available"
		case *ReleaseBundleResponse:
			msg = "Release Bundles: No Release Bundle Info Available"
		case jpd.GenericError:
			if err, ok := rw.data.(error); ok {
				msg = fmt.Sprintf("Errors: %s", err.Error())
			} else {
				msg = "Errors: An unknown data type was received."
			}
		}
		jsonBytes, err = json.MarshalIndent(msg, "", "  ")
		if err != nil {
			return err
		}
		result = string(jsonBytes)
	}
	fmt.Println(result)
	return nil
}

func (rw *GenericResultsWriter) PrintDashboard() error {
	if rw.data == nil {
		return nil
	}

	switch v := rw.data.(type) {
	case *ArtifactoryInfo:
		PrintArtifactoryDashboard(v)
	case []services.Project:
		PrintProjectsDashboard(v)
	case *[]JPD:
		PrintJPDsDashboard(*v)
	case *ReleaseBundleResponse:
		PrintReleaseBundlesDashboard(v)
	case *jpd.GenericError:
		PrintErrorsDashboard(*v)
	}
	return nil
}

type TableRow struct {
	Metric string `col-name:"Metric"`
	Value  string `col-name:"Value"`
}

func PrintArtifactoryDashboard(stats *ArtifactoryInfo) {
	summarySlice := []TableRow{
		{Metric: text.FgHiBlue.Sprint("Total Projects"), Value: text.FgGreen.Sprint(stats.ProjectsCount)},
		{Metric: text.FgHiBlue.Sprint("Total Binaries"), Value: text.FgGreen.Sprint(stats.StorageInfo.BinariesCount)},
		{Metric: text.FgHiBlue.Sprint("Total Binaries Size"), Value: text.FgGreen.Sprint(stats.StorageInfo.BinariesSize)},
		{Metric: text.FgHiBlue.Sprint("Total Artifacts"), Value: text.FgGreen.Sprint(stats.StorageInfo.ArtifactsCount)},
		{Metric: text.FgHiBlue.Sprint("Total Artifacts Size"), Value: text.FgGreen.Sprint(stats.StorageInfo.ArtifactsSize)},
		{Metric: text.FgHiBlue.Sprint("Storage Type"), Value: text.FgGreen.Sprint(stats.StorageInfo.StorageType)},
	}

	err := coreutils.PrintTableWithBorderless(summarySlice, text.FgCyan.Sprint("Artifacts Summary"), "", "No data found", false)
	if err != nil {
		return
	}
	log.Output()

	repoTypeCounts := make(map[string]int)

	for _, repo := range stats.RepositoriesDetails {
		if repo.Type != "TOTAL" && repo.Type != "NA" {
			repoTypeCounts[repo.Type]++
		}
	}
	summarySlice = []TableRow{
		{Metric: text.FgCyan.Sprint("Repository Type"), Value: text.FgCyan.Sprint("Count")},
	}
	for repoType, count := range repoTypeCounts {
		summarySlice = append(summarySlice, TableRow{Metric: text.FgHiBlue.Sprint(repoType), Value: text.FgGreen.Sprint(count)})
	}

	err = coreutils.PrintTableWithBorderless(summarySlice, text.FgCyan.Sprint(""), "", "No data found", false)
	if err != nil {
		return
	}
	log.Output()
}

func PrintProjectsDashboard(projects []services.Project) {
	loopRange := len(projects)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}

	actualProjectsCount := len(projects)
	tableData := []TableRow{{"Project Key", "Display Name"}}

	for i := 0; i < loopRange; i++ {
		project := (projects)[i]
		tableData = append(tableData, TableRow{Metric: text.FgHiBlue.Sprint(project.ProjectKey), Value: text.FgGreen.Sprint(project.DisplayName)})
	}

	footer := ""
	if actualProjectsCount > displayLimit {
		footer = text.FgYellow.Sprintf("\n...and %d more projects. Refer JSON output format for complete list.", actualProjectsCount-displayLimit)
	}
	err := coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("Projects Summary"), footer, "No Projects Found", false)
	if err != nil {
		return
	}
	log.Output()
}

func PrintJPDsDashboard(jpdList []JPD) {
	loopRange := len(jpdList)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualCount := len(jpdList)

	tableData := []TableRow{{"Name", "Status"}}
	for i := 0; i < loopRange; i++ {
		jpd := jpdList[i]
		var status string
		if jpd.Status.Code == "ONLINE" || jpd.Status.Code == "Healthy" {
			status = text.FgGreen.Sprint(jpd.Status.Code)
		} else {
			status = text.FgRed.Sprint(jpd.Status.Code)
		}
		tableData = append(tableData, TableRow{
			Metric: text.FgHiBlue.Sprint(jpd.Name),
			Value:  status,
		})
	}

	footer := ""
	if actualCount > displayLimit {
		footer = text.FgYellow.Sprintf("\n...and %d more JPDs. Refer JSON output format for complete list.", actualCount-displayLimit)
	}

	err := coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("JFrog Platform Deployments (JPDs)"), footer, "No JPDs Found", false)
	if err != nil {
		log.Error("Failed to print JPDs table:", err)
		return
	}
	log.Output()
}

func PrintReleaseBundlesDashboard(rbResponse *ReleaseBundleResponse) {
	loopRange := len(rbResponse.ReleaseBundles)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualCount := len(rbResponse.ReleaseBundles)

	tableData := []TableRow{{"Name", "Project Key"}}
	for i := 0; i < loopRange; i++ {
		rb := rbResponse.ReleaseBundles[i]
		tableData = append(tableData, TableRow{
			Metric: text.FgGreen.Sprint(rb.ReleaseBundleName),
			Value:  text.FgWhite.Sprint(rb.ProjectKey),
		})
	}

	footer := ""
	if actualCount > displayLimit {
		footer = text.FgYellow.Sprintf("\n...and %d more release bundles. Refer JSON output format for complete list.", actualCount-displayLimit)
	}

	err := coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("Release Bundles"), footer, "No Release Bundles Found", false)
	if err != nil {
		log.Error("Failed to print Release Bundles table:", err)
		return
	}
	log.Output()
}

func PrintErrorsDashboard(genericError jpd.GenericError) {
	errRows := createErrorRows(genericError)

	err := coreutils.PrintTableWithBorderless(errRows, text.FgCyan.Sprint(genericError.Product), "", genericError.Error(), false)
	if err != nil {
		log.Error("Failed to print error logs", err)
		return
	}
	log.Output()
}

func createErrorRows(genericError jpd.GenericError) []TableRow {
	errorRows := []TableRow{
		{Metric: text.FgCyan.Sprint("Product"), Value: text.FgRed.Sprint(genericError.Product)},
		{Metric: text.FgCyan.Sprint("Error"), Value: text.FgRed.Sprint(genericError.Error())},
	}
	return errorRows
}

func (rw *GenericResultsWriter) PrintSimple() error {
	if rw.data == nil {
		return nil
	}

	switch v := rw.data.(type) {
	case *ArtifactoryInfo:
		PrintArtifactoryStats(v)
	case []services.Project:
		PrintProjectsStats(v)
	case *[]JPD:
		PrintJPDsStats(v)
	case *ReleaseBundleResponse:
		PrintReleaseBundlesSimple(v)
	case *jpd.GenericError:
		PrintGenericError(v)
	}
	return nil
}

func PrintReleaseBundlesSimple(rbResponse *ReleaseBundleResponse) {
	log.Output("--- Release Bundles ---")
	if len(rbResponse.ReleaseBundles) == 0 {
		log.Output("No Release Bundles Available")
		log.Output()
		return
	}
	loopRange := len(rbResponse.ReleaseBundles)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualReleaseBundlesCount := len(rbResponse.ReleaseBundles)
	for i := 0; i < loopRange; i++ {
		rb := rbResponse.ReleaseBundles[i]
		log.Output(rb)
	}
	if actualReleaseBundlesCount > displayLimit {
		text.FgYellow.Sprintf("\n...and %d more release bundles. Refer JSON output format for complete list.", actualReleaseBundlesCount-displayLimit)
	}
	log.Output()
}

func PrintArtifactoryStats(stats *ArtifactoryInfo) {
	log.Output("--- Artifactory Statistics ---")
	log.Output("Total Projects: ", stats.ProjectsCount)
	log.Output("Total No of Binaries: ", stats.StorageInfo.BinariesCount)
	log.Output("Total Binaries Size: ", stats.StorageInfo.BinariesSize)
	log.Output("Total No of Artifacts: ", stats.StorageInfo.ArtifactsCount)
	log.Output("Total Artifacts Size: ", stats.StorageInfo.ArtifactsSize)
	log.Output("Storage Type: ", stats.StorageInfo.StorageType)
	log.Output()

	repoTypeCounts := make(map[string]int)
	for _, repo := range stats.RepositoriesDetails {
		if repo.Type != "TOTAL" && repo.Type != "NA" {
			repoTypeCounts[repo.Type]++
		}
	}
	log.Output("--- Repositories Details ---")
	for repoType, count := range repoTypeCounts {
		log.Output(repoType, ": ", count)
	}
	log.Output()
}

func PrintProjectsStats(projects []services.Project) {
	log.Output("--- Available Projects ---")
	if len(projects) == 0 {
		log.Output("No Projects Available")
		log.Output()
		return
	}
	loopRange := len(projects)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualProjectsCount := len(projects)
	for i := 0; i < loopRange; i++ {
		project := projects[i]
		log.Output(project)
	}
	if actualProjectsCount > displayLimit {
		text.FgYellow.Sprintf("\n...and %d more projects, Try JSON output format for complete list.", actualProjectsCount-displayLimit)
	}
	log.Output()
}

func PrintJPDsStats(jpdList *[]JPD) {
	log.Output("--- Available JPDs ---")
	if len(*jpdList) == 0 {
		log.Output("No JPDs Info Available")
		log.Output()
		return
	}
	loopRange := len(*jpdList)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualProjectsCount := len(*jpdList)
	for i := 0; i < loopRange; i++ {
		jpd := (*jpdList)[i]
		log.Output(jpd)
	}
	if actualProjectsCount > displayLimit {
		text.FgYellow.Sprintf("\n...and %d more JPDs, Try JSON output format for complete list.", actualProjectsCount-displayLimit)
	}
}

func PrintGenericError(err *jpd.GenericError) {
	if err != nil {
		log.Output("--- Unable to get error logs ---")
		return
	}
	log.Output("---", err.Product, "---")
	log.Output("Error: ", err.Err)
	log.Output()
}

func (sa *StatsArtifactory) GetArtifactoryStats() (interface{}, error) {
	var artifactoryStats ArtifactoryInfo
	storageInfo, err := sa.ServicesManager.GetStorageInfo()
	if err != nil {
		wrappedError := fmt.Errorf("failed to build ARTIFACTORY API endpoint: %w", err)
		return nil, jpd.NewGenericError("ARTIFACTORY", wrappedError)
	}
	artifactoryStats.StorageInfo = *storageInfo
	repositoriesDetails, err := sa.ServicesManager.GetAllRepositories()
	if err != nil {
		wrappedError := fmt.Errorf("failed to call ARTIFACTORY API: %w", err)
		return nil, jpd.NewGenericError("ARTIFACTORY", wrappedError)
	}
	artifactoryStats.RepositoriesDetails = *repositoriesDetails
	return &artifactoryStats, nil
}

func (sa *StatsArtifactory) GetProjectsStats() (interface{}, error) {
	projects, err := sa.AccessManager.GetAllProjects()
	if err != nil {
		wrappedError := fmt.Errorf("failed to call PROJECTS API: %w", err)
		return nil, jpd.NewGenericError("PROJECTS", wrappedError)
	}
	return projects, nil
}

func (sa *StatsArtifactory) GetJPDsStats() (interface{}, error) {
	body, err := sa.JPDServicesManager.GetJPDsStats(sa.ServerUrl)
	if err != nil {
		wrappedError := fmt.Errorf("failed to call PROJECTS API: %w", err)
		return nil, jpd.NewGenericError("JPDs", wrappedError)
	}
	var jpdList []JPD
	if err := json.Unmarshal(body, &jpdList); err != nil {
		wrappedError := fmt.Errorf("error parsing JPDs JSON: %w", err)
		return nil, jpd.NewGenericError("JPDs", wrappedError)
	}
	return &jpdList, nil
}

func (sa *StatsArtifactory) GetReleaseBundlesStats() (interface{}, error) {
	body, err := sa.LifecycleServiceManager.GetReleaseBundlesStats(sa.ServerUrl)
	if err != nil {
		wrappedError := fmt.Errorf("failed to call RELEASE-BUNDLES API endpoint: %w", err)
		return nil, jpd.NewGenericError("RELEASE-BUNDLES", wrappedError)
	}
	var releaseBundles ReleaseBundleResponse
	if err := json.Unmarshal(body, &releaseBundles); err != nil {
		wrappedError := fmt.Errorf("error parsing RELEASE-BUNDLES JSON: %w", err)
		return nil, jpd.NewGenericError("RELEASE-BUNDLES", wrappedError)
	}
	return &releaseBundles, nil
}

func updateProjectInArtifactoryInfo(allResultsMap *map[string]interface{}) {
	m := *allResultsMap

	pjResult, pjOk := m["pj"]
	if !pjOk || pjResult == nil {
		return
	}

	rtResult, rtOk := m["rt"]
	if !rtOk || rtResult == nil {
		return
	}

	projects, ok := pjResult.([]services.Project)
	if !ok {
		return
	}

	artifactoryInfo, ok := rtResult.(*ArtifactoryInfo)
	if !ok {
		return
	}

	artifactoryInfo.ProjectsCount = len(projects)
	m["rt"] = artifactoryInfo
}
