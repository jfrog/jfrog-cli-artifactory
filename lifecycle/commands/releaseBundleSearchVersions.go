package commands

import (
	"encoding/json"
	"errors"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/jfrog/jfrog-cli-artifactory/stats"
	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	clientutils "github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type SearchVersionsCommand struct {
	serverDetails     *config.ServerDetails
	releaseBundleName string
	offset            int
	limit             int
	filterBy          string
	orderBy           string
	includes          string
	orderAsc          bool
	format            string
}

func NewSearchVersionsCommand() *SearchVersionsCommand {
	return &SearchVersionsCommand{}
}

func (svc *SearchVersionsCommand) SetServerDetails(serverDetails *config.ServerDetails) *SearchVersionsCommand {
	svc.serverDetails = serverDetails
	return svc
}

func (svc *SearchVersionsCommand) SetOffset(offset int) *SearchVersionsCommand {
	svc.offset = offset
	return svc
}

func (svc *SearchVersionsCommand) SetLimit(limit int) *SearchVersionsCommand {
	svc.limit = limit
	return svc
}

func (svc *SearchVersionsCommand) SetFilterBy(filterBy string) *SearchVersionsCommand {
	svc.filterBy = filterBy
	return svc
}

func (svc *SearchVersionsCommand) SetOrderBy(orderBy string) *SearchVersionsCommand {
	svc.orderBy = orderBy
	return svc
}

func (svc *SearchVersionsCommand) SetOrderAsc(orderAsc bool) *SearchVersionsCommand {
	svc.orderAsc = orderAsc
	return svc
}

func (svc *SearchVersionsCommand) SetIncludes(includes string) *SearchVersionsCommand {
	svc.includes = includes
	return svc
}

func (svc *SearchVersionsCommand) SetReleaseBundleName(releaseBundleName string) *SearchVersionsCommand {
	svc.releaseBundleName = releaseBundleName
	return svc
}

func (svc *SearchVersionsCommand) GetReleaseBundleName() string {
	return svc.releaseBundleName
}

func (svc *SearchVersionsCommand) SetOutputFormat(format string) *SearchVersionsCommand {
	svc.format = format
	return svc
}

func (svc *SearchVersionsCommand) CommandName() string {
	return "release-bundle-search"
}

func (svc *SearchVersionsCommand) ServerDetails() (*config.ServerDetails, error) {
	return svc.serverDetails, nil
}

func (svc *SearchVersionsCommand) Run() error {
	lcServicesManager, err := rtUtils.CreateLifecycleServiceManager(svc.serverDetails, false)
	if err != nil {
		return err
	}
	queryParameters := services.GetSearchOptionalQueryParams{
		Offset:   svc.offset,
		Limit:    svc.limit,
		FilterBy: svc.filterBy,
		OrderBy:  svc.orderBy,
		OrderAsc: svc.orderAsc,
	}
	searchVersionResponse, err := lcServicesManager.ReleaseBundlesSearchVersions(svc.releaseBundleName, queryParameters)
	if err != nil {
		return err
	}
	if svc.format == "json" {
		content, err := json.Marshal(searchVersionResponse)
		if err != nil {
			return err
		}
		log.Output(clientutils.IndentJson(content))
		return nil
	} else {
		return printReleaseBundleSearchVersionsTable(searchVersionResponse)
	}
}

func printReleaseBundleSearchVersionsTable(searchVersionResponse services.ReleaseBundleVersionsResponse) error {
	displayLimit := 10
	actualCount := len(searchVersionResponse.ReleaseBundles)
	loopRange := len(searchVersionResponse.ReleaseBundles)
	if loopRange > displayLimit {
		loopRange = displayLimit
	}

	tableData := []stats.TableRow{{Metric: "Name", Value: "Version"}}
	for i := 0; i < loopRange; i++ {
		rbDetails := searchVersionResponse.ReleaseBundles[i]
		tableData = append(tableData, stats.TableRow{
			Metric: text.FgHiBlue.Sprint(rbDetails.ReleaseBundleName),
			Value:  text.FgHiGreen.Sprint(rbDetails.ReleaseBundleVersion),
		})
	}
	footer := ""
	if actualCount > loopRange {
		footer = text.FgYellow.Sprintf("\n...and %d more release bundle versions. Refer JSON output format for complete list.", actualCount-loopRange)
	}
	err := coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("Release Bundles Versions"), footer, "No Release Bundle Version Found", false)
	if err != nil {
		return errors.New("failed to print ReleaseBundlesSearchVersions table")
	}
	return nil
}
