package commands

import (
	rtUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
	_, err = lcServicesManager.ReleaseBundlesSearchVersions(svc.GetReleaseBundleName())
	if err != nil {
		return err
	}
	return nil
}
