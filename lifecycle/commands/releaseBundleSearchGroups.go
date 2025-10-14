package commands

import (
	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
	_, err = lcServicesManager.ReleaseBundlesSearchGroup()
	if err != nil {
		return err
	}
	return nil
}
