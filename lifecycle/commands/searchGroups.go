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

type SearchGroupCommand struct {
	serverDetails *config.ServerDetails
	offset        int
	limit         int
	filterBy      string
	orderBy       string
	orderAsc      bool
	format        string
}

func NewSearchGroupCommand() *SearchGroupCommand {
	return &SearchGroupCommand{}
}

func (sgc *SearchGroupCommand) SetServerDetails(serverDetails *config.ServerDetails) *SearchGroupCommand {
	sgc.serverDetails = serverDetails
	return sgc
}

func (sgc *SearchGroupCommand) SetOffset(offset int) *SearchGroupCommand {
	sgc.offset = offset
	return sgc
}

func (sgc *SearchGroupCommand) SetLimit(limit int) *SearchGroupCommand {
	sgc.limit = limit
	return sgc
}

func (sgc *SearchGroupCommand) SetFilterBy(filterBy string) *SearchGroupCommand {
	sgc.filterBy = filterBy
	return sgc
}

func (sgc *SearchGroupCommand) SetOrderBy(orderBy string) *SearchGroupCommand {
	sgc.orderBy = orderBy
	return sgc
}

func (sgc *SearchGroupCommand) SetOrderAsc(orderAsc bool) *SearchGroupCommand {
	sgc.orderAsc = orderAsc
	return sgc
}

func (sgc *SearchGroupCommand) SetOutputFormat(format string) *SearchGroupCommand {
	sgc.format = format
	return sgc
}

func (sgc *SearchGroupCommand) CommandName() string {
	return "release-bundle-search"
}

func (sgc *SearchGroupCommand) ServerDetails() (*config.ServerDetails, error) {
	return sgc.serverDetails, nil
}

func (sgc *SearchGroupCommand) Run() error {
	lcServicesManager, err := rtUtils.CreateLifecycleServiceManager(sgc.serverDetails, false)
	if err != nil {
		return err
	}
	queryParameters := services.GetSearchOptionalQueryParams{
		Offset:   sgc.offset,
		Limit:    sgc.limit,
		FilterBy: sgc.filterBy,
		OrderBy:  sgc.orderBy,
		OrderAsc: sgc.orderAsc,
	}
	searchGroupResponse, err := lcServicesManager.ReleaseBundlesSearchGroup(queryParameters)
	if err != nil {
		return err
	}
	if sgc.format == "json" {
		content, err := json.Marshal(searchGroupResponse)
		if err != nil {
			return err
		}
		log.Output(clientutils.IndentJson(content))
		return nil
	} else {
		displayLimit := 10
		loopRange := len(searchGroupResponse.ReleaseBundleSearchGroup)
		if loopRange > displayLimit {
			loopRange = displayLimit
		}
		actualCount := len(searchGroupResponse.ReleaseBundleSearchGroup)
		tableData := []stats.TableRow{{Metric: "Name", Value: "Lastest Version"}}
		for i := 0; i < loopRange; i++ {
			rbDetails := searchGroupResponse.ReleaseBundleSearchGroup[i]
			tableData = append(tableData, stats.TableRow{
				Metric: text.FgHiBlue.Sprint(rbDetails.ReleaseBundleName),
				Value:  text.FgHiGreen.Sprint(rbDetails.ReleaseBundleVersionLatest),
			})
		}
		footer := ""
		if actualCount > displayLimit {
			footer = text.FgYellow.Sprintf("\n...and %d more release bundle names. Refer JSON output format for complete list.", actualCount-displayLimit)
		}
		err = coreutils.PrintTableWithBorderless(tableData, text.FgCyan.Sprint("Release Bundles Details"), footer, "No Release Bundle Found", false)
		if err != nil {
			return errors.New("failed to print ReleaseBundlesSearchGroup table")
		}
	}
	return nil
}
