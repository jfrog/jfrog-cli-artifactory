package mvn

import (
	"context"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
	cmaven "github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents/maven"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"
)

func (mu *MvnUtils) runXrayComponentHealing(ctx context.Context, workingDir string, serverDetails *config.ServerDetails) (restore func() error, healed bool, err error) {
	if healcomponents.IsComponentResolutionDisabled() {
		return healcomponents.SkipHealing("Xray component healing disabled", nil)
	}
	command := cmaven.DeriveResolutionCommand(mu.goals)
	if command == "" || mu.vConfig == nil || !mu.vConfig.IsSet("resolver") {
		return healcomponents.SkipHealing("Xray component healing skipped", nil)
	}
	resolverRepo, resolverErr := mu.resolverRepoForHealing()
	if resolverErr != nil {
		return healcomponents.SkipHealing("Xray component healing skipped: could not determine resolver repo: ", resolverErr)
	}
	if resolverRepo == "" {
		return healcomponents.SkipHealing("Xray component healing skipped: resolver repo is empty", nil)
	}
	if serverDetails == nil {
		serverDetails, err = buildUtils.GetServerDetails(mu.vConfig)
		if err != nil {
			return healcomponents.SkipHealing("Xray component healing skipped: could not determine server details: ", err)
		}
		if serverDetails == nil {
			return healcomponents.SkipHealing("Xray component healing skipped: could not determine server details", nil)
		}
	}
	var projectKey string
	if mu.buildConf != nil {
		projectKey = mu.buildConf.GetProject()
	}
	xrayManager, xrayErr := xray.CreateXrayServiceManager(serverDetails, xray.WithScopedProjectKey(projectKey))
	if xrayErr != nil {
		return healcomponents.SkipHealing("Xray component healing skipped: could not create Xray service manager: ", xrayErr)
	}
	return healcomponents.RunIfEnabled(ctx, xrayManager, resolverRepo, cmaven.NewBuildToolWithGoals(mu.goals), command, workingDir, nil)
}

func (mu *MvnUtils) resolverRepoForHealing() (string, error) {
	repoConfig, err := project.GetRepoConfigByPrefix(mu.configPath, project.ProjectConfigResolverPrefix, mu.vConfig)
	if err != nil {
		return "", err
	}
	return repoConfig.TargetRepo(), nil
}
