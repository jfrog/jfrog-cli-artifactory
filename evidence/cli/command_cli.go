package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/create"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/get"
	"github.com/jfrog/jfrog-cli-artifactory/evidence/cli/docs/verify"
	sonarhelper "github.com/jfrog/jfrog-cli-artifactory/evidence/sonar"
	evidenceUtils "github.com/jfrog/jfrog-cli-artifactory/evidence/utils"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	"github.com/jfrog/jfrog-cli-core/v2/common/commands"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	coreUtils "github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func GetCommands() []components.Command {
	return []components.Command{
		{
			Name:        "create-evidence",
			Aliases:     []string{"create"},
			Flags:       GetCommandFlags(CreateEvidence),
			Description: create.GetDescription(),
			Arguments:   create.GetArguments(),
			Action:      createEvidence,
		},
		{
			Name:        "get-evidence",
			Aliases:     []string{"get"},
			Flags:       GetCommandFlags(GetEvidence),
			Description: get.GetDescription(),
			Arguments:   get.GetArguments(),
			Action:      getEvidence,
		},
		{
			Name:        "verify-evidence",
			Aliases:     []string{"verify"},
			Flags:       GetCommandFlags(VerifyEvidence),
			Description: verify.GetDescription(),
			Arguments:   verify.GetArguments(),
			Action:      verifyEvidence,
		},
	}
}

var execFunc = commands.Exec
var ErrUnsupportedSubject = errors.New("unsupported subject")

func createEvidence(ctx *components.Context) error {
	if err := validateCreateEvidenceCommonContext(ctx); err != nil {
		return err
	}
	evidenceType, err := getAndValidateSubject(ctx)
	if err != nil {
		return err
	}
	serverDetails, err := evidenceDetailsByFlags(ctx)
	if err != nil {
		return err
	}

	if slices.Contains(evidenceType, typeFlag) || (slices.Contains(evidenceType, buildName) && slices.Contains(evidenceType, typeFlag)) {
		return NewEvidenceGitHubCommand(ctx, execFunc).CreateEvidence(ctx, serverDetails)
	}

	evidenceCommands := map[string]func(*components.Context, execCommandFunc) EvidenceCommands{
		subjectRepoPath: NewEvidenceCustomCommand,
		releaseBundle:   NewEvidenceReleaseBundleCommand,
		buildName:       NewEvidenceBuildCommand,
		packageName:     NewEvidencePackageCommand,
	}

	if commandFunc, exists := evidenceCommands[evidenceType[0]]; exists {
		return commandFunc(ctx, execFunc).CreateEvidence(ctx, serverDetails)
	}

	return ErrUnsupportedSubject
}

func getEvidence(ctx *components.Context) error {
	if err := validateGetEvidenceCommonContext(ctx); err != nil {
		return err
	}

	evidenceType, err := getAndValidateSubject(ctx)
	if err != nil {
		return err
	}

	serverDetails, err := evidenceDetailsByFlags(ctx)
	if err != nil {
		return err
	}

	evidenceCommands := map[string]func(*components.Context, execCommandFunc) EvidenceCommands{
		subjectRepoPath: NewEvidenceCustomCommand,
		releaseBundle:   NewEvidenceReleaseBundleCommand,
	}

	if commandFunc, exists := evidenceCommands[evidenceType[0]]; exists {
		return commandFunc(ctx, execFunc).GetEvidence(ctx, serverDetails)
	}

	return ErrUnsupportedSubject
}

func validateGetEvidenceCommonContext(ctx *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(ctx, ctx.Arguments); show || err != nil {
		return err
	}

	if len(ctx.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(ctx)
	}

	return nil
}

func verifyEvidence(ctx *components.Context) error {
	// validate common context
	serverDetails, err := evidenceDetailsByFlags(ctx)
	if err != nil {
		return err
	}
	subjectType, err := getAndValidateSubject(ctx)
	if err != nil {
		return err
	}
	err = validateKeys(ctx)
	if err != nil {
		return err
	}
	evidenceCommands := map[string]func(*components.Context, execCommandFunc) EvidenceCommands{
		subjectRepoPath: NewEvidenceCustomCommand,
		releaseBundle:   NewEvidenceReleaseBundleCommand,
		buildName:       NewEvidenceBuildCommand,
		packageName:     NewEvidencePackageCommand,
	}
	if commandFunc, exists := evidenceCommands[subjectType[0]]; exists {
		err = commandFunc(ctx, execFunc).VerifyEvidence(ctx, serverDetails)
		if err != nil {
			if err.Error() != "" {
				return fmt.Errorf("evidence verification failed: %w", err)
			}
			return err
		}
		return nil
	}
	return errors.New("unsupported subject")
}

func validateCreateEvidenceCommonContext(ctx *components.Context) error {
	if show, err := pluginsCommon.ShowCmdHelpIfNeeded(ctx, ctx.Arguments); show || err != nil {
		return err
	}

	if len(ctx.Arguments) > 1 {
		return pluginsCommon.WrongNumberOfArgumentsHandler(ctx)
	}

	if ctx.IsFlagSet(sigstoreBundle) && assertValueProvided(ctx, sigstoreBundle) == nil {
		if err := validateSigstoreBundleArgsConflicts(ctx); err != nil {
			return err
		}
		return nil
	}

	if ctx.IsFlagSet(integration) && assertValueProvided(ctx, integration) == nil {
		if err := evidenceUtils.ValidateIntegration(ctx.GetStringFlagValue(integration)); err != nil {
			return err
		}
	}

	if (!ctx.IsFlagSet(predicate) || assertValueProvided(ctx, predicate) != nil) && !ctx.IsFlagSet(typeFlag) {
		if !evidenceUtils.IsSonarIntegration(ctx.GetStringFlagValue(integration)) {
			return errorutils.CheckErrorf("'predicate' is a mandatory field for creating evidence: --%s", predicate)
		}
	}

	if (!ctx.IsFlagSet(predicateType) || assertValueProvided(ctx, predicateType) != nil) && !ctx.IsFlagSet(typeFlag) {
		if !evidenceUtils.IsSonarIntegration(ctx.GetStringFlagValue(integration)) {
			return errorutils.CheckErrorf("'predicate-type' is a mandatory field for creating evidence: --%s", predicateType)
		}
	}

	// Validate SonarQube requirements when sonar integration is set
	if evidenceUtils.IsSonarIntegration(ctx.GetStringFlagValue(integration)) {
		if err := validateSonarQubeRequirements(); err != nil {
			return err
		}
		// Conflicting flags with sonar evidence type
		if ctx.IsFlagSet(predicate) && ctx.GetStringFlagValue(predicate) != "" {
			return errorutils.CheckErrorf("--%s cannot be used together with --%s %s", predicate, integration, evidenceUtils.SonarIntegration)
		}
		if ctx.IsFlagSet(predicateType) && ctx.GetStringFlagValue(predicateType) != "" {
			return errorutils.CheckErrorf("--%s cannot be used together with --%s %s", predicateType, integration, evidenceUtils.SonarIntegration)
		}
	}

	if err := ensureKeyExists(ctx, key); err != nil {
		return err
	}

	if !ctx.IsFlagSet(keyAlias) {
		setKeyAliasIfProvided(ctx, keyAlias)
	}
	return nil
}

func validateSigstoreBundleArgsConflicts(ctx *components.Context) error {
	var conflictingParams []string

	if ctx.IsFlagSet(key) && ctx.GetStringFlagValue(key) != "" {
		conflictingParams = append(conflictingParams, "--"+key)
	}
	if ctx.IsFlagSet(keyAlias) && ctx.GetStringFlagValue(keyAlias) != "" {
		conflictingParams = append(conflictingParams, "--"+keyAlias)
	}
	if ctx.IsFlagSet(predicate) && ctx.GetStringFlagValue(predicate) != "" {
		conflictingParams = append(conflictingParams, "--"+predicate)
	}
	if ctx.IsFlagSet(predicateType) && ctx.GetStringFlagValue(predicateType) != "" {
		conflictingParams = append(conflictingParams, "--"+predicateType)
	}

	if len(conflictingParams) > 0 {
		return errorutils.CheckErrorf("The following parameters cannot be used with --%s: %s. These values are extracted from the bundle itself:", sigstoreBundle, strings.Join(conflictingParams, ", "))
	}

	return nil
}

func ensureKeyExists(ctx *components.Context, key string) error {
	if ctx.IsFlagSet(key) && assertValueProvided(ctx, key) == nil {
		return nil
	}

	signingKeyValue, _ := evidenceUtils.GetEnvVariable(coreUtils.SigningKey)
	if signingKeyValue == "" {
		return errorutils.CheckErrorf("JFROG_CLI_SIGNING_KEY env variable or --%s flag must be provided when creating evidence", key)
	}
	ctx.AddStringFlag(key, signingKeyValue)
	return nil
}

func setKeyAliasIfProvided(ctx *components.Context, keyAlias string) {
	evdKeyAliasValue, _ := evidenceUtils.GetEnvVariable(coreUtils.KeyAlias)
	if evdKeyAliasValue != "" {
		ctx.AddStringFlag(keyAlias, evdKeyAliasValue)
	}
}

func getAndValidateSubject(ctx *components.Context) ([]string, error) {
	var foundSubjects []string
	for _, key := range subjectTypes {
		if ctx.GetStringFlagValue(key) != "" {
			foundSubjects = append(foundSubjects, key)
		}
	}

	if len(foundSubjects) == 0 {
		if ctx.IsFlagSet(sigstoreBundle) && assertValueProvided(ctx, sigstoreBundle) == nil {
			return []string{subjectRepoPath}, nil // Return subjectRepoPath as the type for routing
		}
		// If we have no subject - we will try to create EVD on build
		if !attemptSetBuildNameAndNumber(ctx) {
			return nil, errorutils.CheckErrorf("subject must be one of the fields: [%s]", strings.Join(subjectTypes, ", "))
		}
		foundSubjects = append(foundSubjects, buildName)
	}

	if err := validateFoundSubjects(ctx, foundSubjects); err != nil {
		return nil, err
	}

	return foundSubjects, nil
}

func attemptSetBuildNameAndNumber(ctx *components.Context) bool {
	buildNameAdded := setBuildValue(ctx, buildName, coreUtils.BuildName)
	buildNumberAdded := setBuildValue(ctx, buildNumber, coreUtils.BuildNumber)

	return buildNameAdded && buildNumberAdded
}

func setBuildValue(ctx *components.Context, flag, envVar string) bool {
	// Check if the flag is provided. If so, we use it.
	if ctx.IsFlagSet(flag) {
		return true
	}
	// If the flag is not set, then check the environment variable
	if currentValue := os.Getenv(envVar); currentValue != "" {
		ctx.AddStringFlag(flag, currentValue)
		return true
	}
	return false
}

func validateKeys(ctx *components.Context) error {
	signingKeyValue, _ := evidenceUtils.GetEnvVariable(coreUtils.SigningKey)
	providedKeys := ctx.GetStringsArrFlagValue(publicKeys)
	if len(providedKeys) > 0 {
		joinedKeys := strings.Join(append(providedKeys, signingKeyValue), ";")
		ctx.SetStringFlagValue(publicKeys, joinedKeys)
	} else {
		ctx.AddStringFlag(publicKeys, signingKeyValue)
	}
	return nil
}

func validateFoundSubjects(ctx *components.Context, foundSubjects []string) error {
	if slices.Contains(foundSubjects, typeFlag) && slices.Contains(foundSubjects, buildName) {
		return nil
	}

	if slices.Contains(foundSubjects, typeFlag) && attemptSetBuildNameAndNumber(ctx) {
		return nil
	}

	if len(foundSubjects) > 1 {
		return errorutils.CheckErrorf("multiple subjects found: [%s]", strings.Join(foundSubjects, ", "))
	}
	return nil
}

func evidenceDetailsByFlags(ctx *components.Context) (*config.ServerDetails, error) {
	serverDetails, err := pluginsCommon.CreateServerDetailsWithConfigOffer(ctx, true, commonCliUtils.Platform)
	if err != nil {
		return nil, err
	}
	if serverDetails.Url == "" {
		return nil, errors.New("platform URL is mandatory for evidence commands")
	}
	platformToEvidenceUrls(serverDetails)

	if serverDetails.GetUser() != "" && serverDetails.GetPassword() != "" {
		return nil, errors.New("evidence service does not support basic authentication")
	}

	return serverDetails, nil
}

func platformToEvidenceUrls(rtDetails *config.ServerDetails) {
	rtDetails.ArtifactoryUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "artifactory/"
	rtDetails.EvidenceUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "evidence/"
	rtDetails.MetadataUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "metadata/"
	rtDetails.OnemodelUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "onemodel/"
	rtDetails.LifecycleUrl = utils.AddTrailingSlashIfNeeded(rtDetails.Url) + "lifecycle/"
}

func assertValueProvided(c *components.Context, fieldName string) error {
	if c.GetStringFlagValue(fieldName) == "" {
		return errorutils.CheckErrorf("the argument --%s can not be empty", fieldName)
	}
	return nil
}

func validateSonarQubeRequirements() error {
	// Check if SonarQube token is present
	if os.Getenv("SONAR_TOKEN") == "" && os.Getenv("SONARQUBE_TOKEN") == "" {
		return errorutils.CheckErrorf("SonarQube token is required when using --%s %s. Please set SONAR_TOKEN or SONARQUBE_TOKEN environment variable", integration, evidenceUtils.SonarIntegration)
	}

	// Check if report-task.txt exists using the detector or config
	reportPath := sonarhelper.GetReportTaskPath()
	if reportPath == "" {
		return errorutils.CheckErrorf("SonarQube report-task.txt file not found. Please ensure SonarQube analysis has been completed or configure a custom path in evidence config")
	}
	log.Info("Found SonarQube task report:", reportPath)

	return nil
}
