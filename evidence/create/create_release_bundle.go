package create

import (
	"fmt"

	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils/commandsummary"

	"github.com/jfrog/jfrog-cli-artifactory/evidence"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/model"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	artifactoryUtils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	lifecycleServices "github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type createEvidenceReleaseBundle struct {
	createEvidenceBase
	project              string
	releaseBundle        string
	releaseBundleVersion string
}

func NewCreateEvidenceReleaseBundle(serverDetails *config.ServerDetails, predicateFilePath, predicateType, markdownFilePath, key, keyId, project, releaseBundle,
	releaseBundleVersion string, useSonarPredicate bool) evidence.Command {
	return &createEvidenceReleaseBundle{
		createEvidenceBase: createEvidenceBase{
			serverDetails:     serverDetails,
			predicateFilePath: predicateFilePath,
			predicateType:     predicateType,
			markdownFilePath:  markdownFilePath,
			key:               key,
			keyId:             keyId,
			stage:             getReleaseBundleStage(serverDetails, releaseBundle, releaseBundleVersion, project),
			useSonarPredicate: useSonarPredicate,
		},
		project:              project,
		releaseBundle:        releaseBundle,
		releaseBundleVersion: releaseBundleVersion,
	}
}

func (c *createEvidenceReleaseBundle) CommandName() string {
	return "create-release-bundle-evidence"
}

func (c *createEvidenceReleaseBundle) ServerDetails() (*config.ServerDetails, error) {
	return c.serverDetails, nil
}

func (c *createEvidenceReleaseBundle) Run() error {
	artifactoryClient, err := c.createArtifactoryClient()
	if err != nil {
		log.Error("failed to create Artifactory client", err)
		return err
	}
	subject, sha256, err := c.buildReleaseBundleSubjectPath(artifactoryClient)
	if err != nil {
		return err
	}
	envelope, err := c.createEnvelope(subject, sha256)
	if err != nil {
		return err
	}
	response, err := c.uploadEvidence(envelope, subject)
	if err != nil {
		return err
	}
	c.recordSummary(response, subject, sha256)

	return nil
}

func (c *createEvidenceReleaseBundle) buildReleaseBundleSubjectPath(artifactoryClient artifactory.ArtifactoryServicesManager) (string, string, error) {
	repoKey := utils.BuildReleaseBundleRepoKey(c.project)
	manifestPath := buildManifestPath(repoKey, c.releaseBundle, c.releaseBundleVersion)

	manifestChecksum, err := c.getFileChecksum(manifestPath, artifactoryClient)
	if err != nil {
		return "", "", err
	}

	return manifestPath, manifestChecksum, nil
}

func (c *createEvidenceReleaseBundle) recordSummary(response *model.CreateResponse, subject string, sha256 string) {
	displayName := fmt.Sprintf("%s %s", c.releaseBundle, c.releaseBundleVersion)
	commandSummary := commandsummary.EvidenceSummaryData{
		Subject:              subject,
		SubjectSha256:        sha256,
		PredicateType:        c.predicateType,
		PredicateSlug:        response.PredicateSlug,
		Verified:             response.Verified,
		DisplayName:          displayName,
		SubjectType:          commandsummary.SubjectTypeReleaseBundle,
		ReleaseBundleName:    c.releaseBundle,
		ReleaseBundleVersion: c.releaseBundleVersion,
		RepoKey:              utils.BuildReleaseBundleRepoKey(c.project),
	}
	err := c.recordEvidenceSummary(commandSummary)
	if err != nil {
		log.Warn("Failed to record evidence summary:", err.Error())
	}
}
func buildManifestPath(repoKey, name, version string) string {
	return fmt.Sprintf("%s/%s/%s/release-bundle.json.evd", repoKey, name, version)
}

func getReleaseBundleStage(serverDetails *config.ServerDetails, releaseBundle, releaseBundleVersion, project string) string {
	log.Debug("fetching release bundle %s:%s stage", releaseBundle, releaseBundleVersion)
	lifecycleServiceManager, err := artifactoryUtils.CreateLifecycleServiceManager(serverDetails, false)
	if err != nil {
		log.Warn("Failed to create lifecycle service manager:", err)
		return ""
	}

	rbDetails, queryParams := initReleaseBundlePromotionDetails(releaseBundle, releaseBundleVersion, project)

	promotionDetails, err := lifecycleServiceManager.GetReleaseBundleVersionPromotions(rbDetails, queryParams)
	if err != nil {
		log.Warn("Failed to get release bundle promotions:", err)
		return ""
	}

	return getReleaseBundleCurrentStage(promotionDetails)
}

func initReleaseBundlePromotionDetails(releaseBundle, releaseBundleVersion, project string) (lifecycleServices.ReleaseBundleDetails, lifecycleServices.GetPromotionsOptionalQueryParams) {
	rbDetails := lifecycleServices.ReleaseBundleDetails{
		ReleaseBundleName:    releaseBundle,
		ReleaseBundleVersion: releaseBundleVersion,
	}
	queryParams := lifecycleServices.GetPromotionsOptionalQueryParams{
		ProjectKey: project,
	}

	return rbDetails, queryParams
}

func getReleaseBundleCurrentStage(promotionDetails lifecycleServices.RbPromotionsResponse) string {
	for _, promotion := range promotionDetails.Promotions {
		if promotion.Status != "COMPLETED" { // If promotion is not completed, than its not the current stage
			continue
		}
		return promotion.Environment
	}

	return ""
}
