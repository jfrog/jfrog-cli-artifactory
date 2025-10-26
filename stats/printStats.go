package stats

import (
	"encoding/json"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/access/services"
	"github.com/jfrog/jfrog-client-go/jpd"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"reflect"
	"strings"
)

type GenericResultsWriter struct {
	data         interface{}
	format       string
	displayLimit int
}

func NewGenericResultsWriter(data interface{}, format string, displayLimit int) *GenericResultsWriter {
	return &GenericResultsWriter{
		data:         data,
		format:       format,
		displayLimit: displayLimit,
	}
}

func (rw *GenericResultsWriter) Print() error {
	if rw.data == nil {
		return nil
	}
	switch rw.format {
	case "json", "simplejson":
		return rw.PrintJson()
	case "table":
		return rw.PrintDashboard()
	default:
		return rw.PrintConsole()
	}
}

func (rw *GenericResultsWriter) PrintJson() error {
	jsonBytes, err := json.MarshalIndent(rw.data, "", "  ")
	if err != nil {
		return err
	}
	result := string(jsonBytes)

	if len(result) <= 2 {
		msg := "No data available"
		switch v := rw.data.(type) {
		case *ArtifactoryStats:
			msg = "Artifacts: No Artifacts Available"
		case []services.Project:
			msg = "Projects: No Project Available"
		case *[]JPD:
			msg = "JPDs: No JPD Available"
		case *ReleaseBundleResponse:
			msg = "Release Bundles: No Release Bundle Info Available"
		case *jpd.GenericError:
			msg = fmt.Sprintf("Errors: %s", v.Error())
		}
		jsonBytes, err = json.MarshalIndent(msg, "", "  ")
		if err != nil {
			return err
		}
		result = string(jsonBytes)
	}
	log.Output(result)
	return nil
}

type TableRow struct {
	Metric string `col-name:"Metric"`
	Value  string `col-name:"Value"`
}

func (rw *GenericResultsWriter) PrintDashboard() error {
	switch v := rw.data.(type) {
	case *ArtifactoryStatsSummary:
		PrintArtifactoryDashboard(v)
	case []services.Project:
		PrintProjectsDashboard(v, rw.displayLimit)
	case *[]JPD:
		PrintJPDsDashboard(*v, rw.displayLimit)
	case *ReleaseBundleResponse:
		PrintReleaseBundlesDashboard(v, rw.displayLimit)
	case *jpd.GenericError:
		PrintErrorsDashboard(v)
	}
	return nil
}

func PrintArtifactoryDashboard(stats *ArtifactoryStatsSummary) {
	summarySlice := []TableRow{
		{Metric: text.FgHiBlue.Sprint("Total Projects"), Value: text.FgGreen.Sprint(stats.ProjectsCount)},
		{Metric: text.FgHiBlue.Sprint("Total Binaries"), Value: text.FgGreen.Sprint(stats.TotalBinariesCount)},
		{Metric: text.FgHiBlue.Sprint("Total Binaries Size"), Value: text.FgGreen.Sprint(stats.TotalBinariesSize)},
		{Metric: text.FgHiBlue.Sprint("Total Artifacts"), Value: text.FgGreen.Sprint(stats.TotalArtifactsCount)},
		{Metric: text.FgHiBlue.Sprint("Total Artifacts Size"), Value: text.FgGreen.Sprint(stats.TotalArtifactsSize)},
		{Metric: text.FgHiBlue.Sprint("Storage Type"), Value: text.FgGreen.Sprint(stats.StorageType)},
	}

	err := coreutils.PrintTableWithBorderless(summarySlice, text.FgCyan.Sprint("Artifacts Summary"), "", "No data found", false)
	if err != nil {
		log.Error("Failed to print Artifactory Summary table:", err)
		return
	}
	log.Output()

	repoTypeCounts := make(map[string]int)
	for _, repo := range stats.RepositoriesDetails {
		if repo.Type != "TOTAL" && repo.Type != "NA" {
			repoTypeCounts[repo.Type]++
		}
	}
	repoTypeTableData := []TableRow{
		{Metric: text.FgCyan.Sprint("Repository Type"), Value: text.FgCyan.Sprint("Count")},
	}
	for repoType, count := range repoTypeCounts {
		repoTypeTableData = append(repoTypeTableData, TableRow{Metric: text.FgHiBlue.Sprint(repoType), Value: text.FgGreen.Sprint(count)})
	}

	err = coreutils.PrintTableWithBorderless(repoTypeTableData, "", "", "No data found", false)
	if err != nil {
		log.Error("Failed to print Repository Types table:", err)
		return
	}
	log.Output()
}

func PrintProjectsDashboard(projects []services.Project, displayLimit int) {
	loopRange := len(projects)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}

	actualProjectsCount := len(projects)
	tableData := []TableRow{{"Project Key", "Display Name"}} // Headers

	for i := 0; i < loopRange; i++ {
		project := (projects)[i]
		tableData = append(tableData, TableRow{Metric: text.FgHiBlue.Sprint(project.ProjectKey), Value: text.FgGreen.Sprint(project.DisplayName)})
	}
	if len(tableData) == 1 {
		tableData = []TableRow{}
	}

	footer := ""
	if actualProjectsCount > displayLimit {
		footer = text.FgYellow.Sprintf("\n...and %d more projects. Refer JSON output format for complete list.", actualProjectsCount-displayLimit)
	}
	err := coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("Projects Summary"), footer, "No Projects Found", false)
	if err != nil {
		log.Error("Failed to print Projects table:", err)
		return
	}
	log.Output()
}

func PrintJPDsDashboard(jpdList []JPD, displayLimit int) {
	loopRange := len(jpdList)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualCount := len(jpdList)

	tableData := []TableRow{{"Name", "Status"}} // Headers
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
	if len(tableData) == 1 {
		tableData = []TableRow{}
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

func PrintReleaseBundlesDashboard(rbResponse *ReleaseBundleResponse, displayLimit int) {
	loopRange := len(rbResponse.ReleaseBundles)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualCount := len(rbResponse.ReleaseBundles)

	tableData := []TableRow{{"Name", "Release Bundle"}}
	for i := 0; i < loopRange; i++ {
		rb := rbResponse.ReleaseBundles[i]
		tableData = append(tableData, TableRow{
			Metric: text.FgGreen.Sprint(rb.ReleaseBundleName),
			Value:  text.FgWhite.Sprint(rb.ProjectKey),
		})
	}
	if len(tableData) == 1 {
		tableData = []TableRow{}
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

func PrintErrorsDashboard(genericError *jpd.GenericError) {
	errRows := createErrorRows(*genericError)

	err := coreutils.PrintTableWithBorderless(errRows, text.FgCyan.Sprint(genericError.Product), "", genericError.Error(), false)
	if err != nil {
		log.Error("Failed to print error logs", err)
		return
	}
	log.Output()
}

func createErrorRows(genericError jpd.GenericError) []TableRow {
	errorRows := []TableRow{
		{Metric: text.FgCyan.Sprint("Error"), Value: text.FgRed.Sprint(genericError.Error())},
	}
	return errorRows
}

func (rw *GenericResultsWriter) PrintConsole() error {
	switch v := rw.data.(type) {
	case *ArtifactoryStatsSummary:
		PrintArtifactoryStats(v)
	case []services.Project:
		PrintProjectsStats(v, rw.displayLimit)
	case *[]JPD:
		PrintJPDsStats(v, rw.displayLimit)
	case *ReleaseBundleResponse:
		PrintReleaseBundlesStats(v, rw.displayLimit)
	case *jpd.GenericError:
		PrintGenericError(v)
	}
	return nil
}

func PrintArtifactoryStats(stats *ArtifactoryStatsSummary) {
	log.Output("--- Artifactory Statistics ---")
	resultString := FormatWithDisplayTags(stats)
	log.Output(resultString)

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

func PrintProjectsStats(projects []services.Project, displayLimit int) {
	log.Output("--- Available Projects ---")
	if len(projects) == 0 {
		log.Output("No Projects Available\n")
		return
	}
	loopRange := len(projects)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualProjectsCount := len(projects)
	for i := 0; i < loopRange; i++ {
		jpdData := FormatWithDisplayTags(projects[i])
		log.Output(jpdData)
	}
	if actualProjectsCount > displayLimit {
		log.Output(text.FgYellow.Sprintf("\n...and %d more projects, Try JSON output format for complete list.", actualProjectsCount-displayLimit))
	}
}

func PrintJPDsStats(jpdList *[]JPD, displayLimit int) {
	log.Output("--- Available JPDs ---")
	if len(*jpdList) == 0 {
		log.Output("No JPDs Info Available\n")
		return
	}
	loopRange := len(*jpdList)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}
	actualJPDsCount := len(*jpdList)
	for i := 0; i < loopRange; i++ {
		jpdData := FormatWithDisplayTags((*jpdList)[i])
		log.Output(jpdData)
	}
	if actualJPDsCount > displayLimit {
		log.Output(text.FgYellow.Sprintf("\n...and %d more JPDs, Try JSON output format for complete list.", actualJPDsCount-displayLimit))
	}
}

func PrintReleaseBundlesStats(rbResponse *ReleaseBundleResponse, displayLimit int) {
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
		resultString := FormatWithDisplayTags(rbResponse.ReleaseBundles[i])
		log.Output(resultString)
	}
	if actualReleaseBundlesCount > displayLimit {
		log.Output(text.FgYellow.Sprintf("\n...and %d more release bundles. Refer JSON output format for complete list.", actualReleaseBundlesCount-displayLimit))
	}
}

func PrintGenericError(err *jpd.GenericError) {
	log.Output("---", err.Product, "---")
	resultString := FormatWithDisplayTags(err)
	log.Output(resultString)
}

func FormatWithDisplayTags(v interface{}) string {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typeOfVal := val.Type()
	var builder strings.Builder
	for i := 0; i < val.NumField(); i++ {
		field := typeOfVal.Field(i)
		displayTag := field.Tag.Get("display")
		if displayTag == "" {
			continue
		}
		fieldValue := val.Field(i)
		builder.WriteString(fmt.Sprintf("%s: %v\n", displayTag, fieldValue.Interface()))
	}
	return builder.String()
}
