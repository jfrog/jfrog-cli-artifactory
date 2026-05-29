package common

import (
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
)

func ListSkills(serverDetails *config.ServerDetails, repoKey string, limit int, sortBy string) ([]services.SkillListItem, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	var allItems []services.SkillListItem
	cursor := ""
	pageSize := 100
	for {
		items, nextCursor, err := sm.ListSkills(repoKey, pageSize, cursor, sortBy)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
		if limit > 0 && len(allItems) >= limit {
			return allItems[:limit], nil
		}
		if nextCursor == "" || len(items) < pageSize {
			break
		}
		cursor = nextCursor
	}
	return allItems, nil
}

func ListVersions(serverDetails *config.ServerDetails, repoKey, slug string) ([]services.SkillVersion, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.ListSkillVersions(repoKey, slug)
}

func SearchSkills(serverDetails *config.ServerDetails, repoKey, query string, limit int) ([]services.SkillSearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkills(repoKey, query, limit)
}

func VersionExists(serverDetails *config.ServerDetails, repoKey, slug, version string) (bool, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return false, err
	}
	return sm.SkillVersionExists(repoKey, slug, version)
}

func SearchSkillsByProperty(serverDetails *config.ServerDetails, query string) ([]services.SkillPropertySearchResult, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return nil, err
	}
	return sm.SearchSkillsByProperty(query)
}

// GetSkillDescription fetches the skill.description property for a given artifact path.
func GetSkillDescription(serverDetails *config.ServerDetails, repoPath string) (string, error) {
	sm, err := utils.CreateServiceManager(serverDetails, 3, 0, false)
	if err != nil {
		return "", err
	}
	props, err := sm.GetItemProps(repoPath)
	if err != nil {
		return "", err
	}
	if descs, ok := props.Properties["skill.description"]; ok && len(descs) > 0 {
		return descs[0], nil
	}
	return "", nil
}
