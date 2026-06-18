package mvn

import (
	"context"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents"
	cmaven "github.com/jfrog/jfrog-cli-artifactory/artifactory/healcomponents/maven"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/xray"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func (mu *MvnUtils) runXrayComponentHealing(ctx context.Context, workingDir string, serverDetails *config.ServerDetails) (restore func() error, healed bool, err error) {
	noop := func() error { return nil }
	if healcomponents.IsComponentResolutionDisabled() {
		return noop, false, nil
	}
	command := cmaven.DeriveResolutionCommand(mu.goals)
	if command == "" || mu.vConfig == nil || !mu.vConfig.IsSet("resolver") {
		return noop, false, nil
	}
	resolverRepo, err := mu.resolverRepoForHealing()
	if err != nil || resolverRepo == "" {
		log.Debug("Xray component healing skipped: could not determine resolver repo")
		return noop, false, nil
	}
	if serverDetails == nil {
		serverDetails, err = buildUtils.GetServerDetails(mu.vConfig)
		if err != nil || serverDetails == nil {
			log.Debug("Xray component healing skipped: could not determine server details")
			return noop, false, nil
		}
	}
	var projectKey string
	if mu.buildConf != nil {
		projectKey = mu.buildConf.GetProject()
	}
	xrayManager, err := xray.CreateXrayServiceManager(serverDetails, xray.WithScopedProjectKey(projectKey))
	if err != nil {
		log.Debug("Xray component healing skipped: could not create Xray service manager: " + err.Error())
		return noop, false, nil
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
