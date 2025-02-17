package flagkit

import (
	"github.com/jfrog/jfrog-cli-artifactory/cliutils/cmddefs"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	distUrl = "dist-url"

	// Unique release-bundle-* v1 flags
	releaseBundleV1Prefix = "rbv1-"
	rbDryRun              = releaseBundleV1Prefix + dryRun
	rbRepo                = releaseBundleV1Prefix + repo
	rbPassphrase          = releaseBundleV1Prefix + passphrase
	distTarget            = releaseBundleV1Prefix + target
	rbDetailedSummary     = releaseBundleV1Prefix + detailedSummary
	sign                  = "sign"
	desc                  = "desc"
	releaseNotesPath      = "release-notes-path"
	releaseNotesSyntax    = "release-notes-syntax"
	deleteFromDist        = "delete-from-dist"

	// Common release-bundle-* v1&v2 flags
	DistRules      = "dist-rules"
	site           = "site"
	city           = "city"
	countryCodes   = "country-codes"
	sync           = "sync"
	maxWaitMinutes = "max-wait-minutes"
	CreateRepo     = "create-repo"

	user        = "user"
	password    = "password"
	accessToken = "access-token"
	serverId    = "server-id"

	// Client certification flags
	InsecureTls = "insecure-tls"

	// Spec flags
	specFlag = "spec"
	specVars = "spec-vars"

	// Generic commands flags
	exclusions      = "exclusions"
	dryRun          = "dry-run"
	targetProps     = "target-props"
	quiet           = "quiet"
	detailedSummary = "detailed-summary"
	deletePrefix    = "delete-"
	deleteQuiet     = deletePrefix + quiet
	repo            = "repo"
	target          = "target"
	name            = "name"
	passphrase      = "passphrase"
	url             = "url"
	Project         = "project"
	IncludeRepos    = "include-repos"
	ExcludeRepos    = "exclude-repos"

	// Unique lifecycle flags
	Sync                 = "sync"
	lifecyclePrefix      = "lc-"
	lcSync               = lifecyclePrefix + Sync
	lcProject            = lifecyclePrefix + Project
	Builds               = "builds"
	lcBuilds             = lifecyclePrefix + Builds
	ReleaseBundles       = "release-bundles"
	lcReleaseBundles     = lifecyclePrefix + ReleaseBundles
	SigningKey           = "signing-key"
	lcSigningKey         = lifecyclePrefix + SigningKey
	PathMappingPattern   = "mapping-pattern"
	lcPathMappingPattern = lifecyclePrefix + PathMappingPattern
	PathMappingTarget    = "mapping-target"
	lcPathMappingTarget  = lifecyclePrefix + PathMappingTarget
	lcDryRun             = lifecyclePrefix + dryRun
	lcIncludeRepos       = lifecyclePrefix + IncludeRepos
	lcExcludeRepos       = lifecyclePrefix + ExcludeRepos
	setupRepo            = repo

	// Build Info flags
	BuildName   = "build-name"
	BuildNumber = "build-number"
)

var commandFlags = map[string][]string{
	cmddefs.ReleaseBundleV1Create: {
		distUrl, user, password, accessToken, serverId, specFlag, specVars, targetProps,
		rbDryRun, sign, desc, exclusions, releaseNotesPath, releaseNotesSyntax, rbPassphrase, rbRepo, InsecureTls, distTarget, rbDetailedSummary,
	},
	cmddefs.ReleaseBundleV1Update: {
		distUrl, user, password, accessToken, serverId, specFlag, specVars, targetProps,
		rbDryRun, sign, desc, exclusions, releaseNotesPath, releaseNotesSyntax, rbPassphrase, rbRepo, InsecureTls, distTarget, rbDetailedSummary,
	},
	cmddefs.ReleaseBundleV1Sign: {
		distUrl, user, password, accessToken, serverId, rbPassphrase, rbRepo,
		InsecureTls, rbDetailedSummary,
	},
	cmddefs.ReleaseBundleV1Distribute: {
		distUrl, user, password, accessToken, serverId, rbDryRun, DistRules,
		site, city, countryCodes, sync, maxWaitMinutes, InsecureTls, CreateRepo,
	},
	cmddefs.ReleaseBundleV1Delete: {
		distUrl, user, password, accessToken, serverId, rbDryRun, DistRules,
		site, city, countryCodes, sync, maxWaitMinutes, InsecureTls, deleteFromDist, deleteQuiet,
	},
}

var flagsMap = map[string]components.Flag{
	distUrl:              components.NewStringFlag(url, "JFrog Distribution URL. (example: https://acme.jfrog.io/distribution)", components.SetMandatoryFalse()),
	user:                 components.NewStringFlag(user, "JFrog username.", components.SetMandatoryFalse()),
	password:             components.NewStringFlag(password, "JFrog password.", components.SetMandatoryFalse()),
	accessToken:          components.NewStringFlag(accessToken, "JFrog access token.", components.SetMandatoryFalse()),
	serverId:             components.NewStringFlag(serverId, "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	specFlag:             components.NewStringFlag(specFlag, "Path to a File Spec.", components.SetMandatoryFalse()),
	specVars:             components.NewStringFlag(specVars, "List of semicolon-separated(;) variables in the form of \"key1=value1;key2=value2;...\" to be replaced in the File Spec.", components.SetMandatoryFalse()),
	targetProps:          components.NewStringFlag(targetProps, "List of semicolon-separated(;) properties, in the form of \"key1=value1;key2=value2;...\" to be added to the artifacts after distribution of the release bundle.", components.SetMandatoryFalse()),
	rbDryRun:             components.NewBoolFlag(dryRun, "Set to true to disable communication with JFrog Distribution.", components.WithBoolDefaultValueFalse()),
	sign:                 components.NewBoolFlag(sign, "If set to true, automatically signs the release bundle version.", components.WithBoolDefaultValueFalse()),
	desc:                 components.NewStringFlag(desc, "Description of the release bundle.", components.SetMandatoryFalse()),
	exclusions:           components.NewStringFlag(exclusions, "List of semicolon-separated(;) exclusions. Exclusions can include the * and the ? wildcards.", components.SetMandatoryFalse()),
	releaseNotesPath:     components.NewStringFlag(releaseNotesPath, "Path to a file describes the release notes for the release bundle version.", components.SetMandatoryFalse()),
	releaseNotesSyntax:   components.NewStringFlag(releaseNotesSyntax, "The syntax for the release notes. Can be one of 'markdown', 'asciidoc', or 'plain_text.", components.SetMandatoryFalse()),
	rbPassphrase:         components.NewStringFlag(passphrase, "The passphrase for the signing key.", components.SetMandatoryFalse()),
	rbRepo:               components.NewStringFlag(repo, "A repository name at source Artifactory to store release bundle artifacts in. If not provided, Artifactory will use the default one.", components.SetMandatoryFalse()),
	InsecureTls:          components.NewBoolFlag(InsecureTls, "Set to true to skip TLS certificates verification.", components.WithBoolDefaultValueFalse()),
	distTarget:           components.NewStringFlag(target, "The target path for distributed artifacts on the edge node.", components.SetMandatoryFalse()),
	rbDetailedSummary:    components.NewBoolFlag(detailedSummary, "Set to true to get a command summary with details about the release bundle artifact.", components.WithBoolDefaultValueFalse()),
	DistRules:            components.NewStringFlag(DistRules, "Path to distribution rules.", components.SetMandatoryFalse()),
	site:                 components.NewStringFlag(site, "Wildcard filter for site name.", components.SetMandatoryFalse()),
	city:                 components.NewStringFlag(city, "Wildcard filter for site city name.", components.SetMandatoryFalse()),
	countryCodes:         components.NewStringFlag(countryCodes, "List of semicolon-separated(;) wildcard filters for site country codes.", components.SetMandatoryFalse()),
	sync:                 components.NewBoolFlag(sync, "Set to true to enable sync distribution (the command execution will end when the distribution process ends).", components.WithBoolDefaultValueFalse()),
	maxWaitMinutes:       components.NewStringFlag(maxWaitMinutes, "Max minutes to wait for sync distribution.", components.WithStrDefaultValue("60")),
	deleteFromDist:       components.NewBoolFlag(deleteFromDist, "Set to true to delete release bundle version in JFrog Distribution itself after deletion is complete.", components.WithBoolDefaultValueFalse()),
	deleteQuiet:          components.NewBoolFlag(quiet, "Set to true to skip the delete confirmation message.", components.WithBoolDefaultValueFalse()),
	CreateRepo:           components.NewBoolFlag(CreateRepo, "Set to true to create the repository on the edge if it does not exist.", components.WithBoolDefaultValueFalse()),
	lcSync:               components.NewBoolFlag(Sync, "Set to false to run asynchronously.", components.WithBoolDefaultValueTrue()),
	lcProject:            components.NewStringFlag(Project, "Project key associated with the Release Bundle version.", components.SetMandatoryFalse()),
	lcBuilds:             components.NewStringFlag(Builds, "Path to a JSON file containing information of the source builds from which to create a release bundle.", components.WithHiddenTrue(), components.SetMandatoryFalse()),
	lcReleaseBundles:     components.NewStringFlag(ReleaseBundles, "Path to a JSON file containing information of the source release bundles from which to create a release bundle.", components.WithHiddenTrue(), components.SetMandatoryFalse()),
	lcSigningKey:         components.NewStringFlag(SigningKey, "The GPG/RSA key-pair name given in Artifactory. If the key isn't provided, the command creates or uses the default key.", components.SetMandatoryFalse()),
	lcPathMappingPattern: components.NewStringFlag(PathMappingPattern, "Specify along with "+PathMappingTarget+" to distribute artifacts to a different path on the edge node. You can use wildcards to specify multiple artifacts.", components.SetMandatoryFalse()),
	lcPathMappingTarget: components.NewStringFlag(PathMappingTarget, "The target path for distributed artifacts on the edge node. If not specified, the artifacts will have the same path and name on the edge node, as on the source Artifactory server. "+
		"For flexibility in specifying the distribution path, you can include placeholders in the form of {1}, {2} which are replaced by corresponding tokens in the pattern path that are enclosed in parenthesis.` `", components.SetMandatoryFalse()),
	lcDryRun: components.NewBoolFlag(dryRun, "Set to true to only simulate the distribution of the release bundle.", components.WithBoolDefaultValueFalse()),
	lcIncludeRepos: components.NewStringFlag(IncludeRepos, "List of semicolon-separated(;) repositories to include in the promotion. If this property is left undefined, all repositories (except those specifically excluded) are included in the promotion. "+
		"If one or more repositories are specifically included, all other repositories are excluded.` `", components.SetMandatoryFalse()),
	lcExcludeRepos: components.NewStringFlag(ExcludeRepos, "List of semicolon-separated(;) repositories to exclude from the promotion.` `", components.SetMandatoryFalse()),
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
