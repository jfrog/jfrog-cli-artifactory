package cli

import (
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	ReleaseBundleV1Create     = "release-bundle-v1-create"
	ReleaseBundleV1Update     = "release-bundle-v1-update"
	ReleaseBundleV1Sign       = "release-bundle-v1-sign"
	ReleaseBundleV1Distribute = "release-bundle-v1-distribute"
	ReleaseBundleV1Delete     = "release-bundle-v1-delete"

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
)

var commandFlags = map[string][]string{
	ReleaseBundleV1Create: {
		distUrl, user, password, accessToken, serverId, specFlag, specVars, targetProps,
		rbDryRun, sign, desc, exclusions, releaseNotesPath, releaseNotesSyntax, rbPassphrase, rbRepo, InsecureTls, distTarget, rbDetailedSummary,
	},
	ReleaseBundleV1Update: {
		distUrl, user, password, accessToken, serverId, specFlag, specVars, targetProps,
		rbDryRun, sign, desc, exclusions, releaseNotesPath, releaseNotesSyntax, rbPassphrase, rbRepo, InsecureTls, distTarget, rbDetailedSummary,
	},
	ReleaseBundleV1Sign: {
		distUrl, user, password, accessToken, serverId, rbPassphrase, rbRepo,
		InsecureTls, rbDetailedSummary,
	},
	ReleaseBundleV1Distribute: {
		distUrl, user, password, accessToken, serverId, rbDryRun, DistRules,
		site, city, countryCodes, sync, maxWaitMinutes, InsecureTls, CreateRepo,
	},
	ReleaseBundleV1Delete: {
		distUrl, user, password, accessToken, serverId, rbDryRun, DistRules,
		site, city, countryCodes, sync, maxWaitMinutes, InsecureTls, deleteFromDist, deleteQuiet,
	},
}

var flagsMap = map[string]components.Flag{
	distUrl:            components.NewStringFlag(url, "JFrog Distribution URL. (example: https://acme.jfrog.io/distribution)", SetMandatoryFalse()),
	user:               components.NewStringFlag(user, "JFrog username.", SetMandatoryFalse()),
	password:           components.NewStringFlag(password, "JFrog password.", SetMandatoryFalse()),
	accessToken:        components.NewStringFlag(accessToken, "JFrog access token.", SetMandatoryFalse()),
	serverId:           components.NewStringFlag(serverId, "Server ID configured using the 'jf config' command.", SetMandatoryFalse()),
	specFlag:           components.NewStringFlag(specFlag, "Path to a File Spec.", SetMandatoryFalse()),
	specVars:           components.NewStringFlag(specVars, "List of semicolon-separated(;) variables in the form of \"key1=value1;key2=value2;...\" to be replaced in the File Spec.", SetMandatoryFalse()),
	targetProps:        components.NewStringFlag(targetProps, "List of semicolon-separated(;) properties, in the form of \"key1=value1;key2=value2;...\" to be added to the artifacts after distribution of the release bundle.", SetMandatoryFalse()),
	rbDryRun:           components.NewBoolFlag(dryRun, "Set to true to disable communication with JFrog Distribution.", WithBoolDefaultValueFalse()),
	sign:               components.NewBoolFlag(sign, "If set to true, automatically signs the release bundle version.", WithBoolDefaultValueFalse()),
	desc:               components.NewStringFlag(desc, "Description of the release bundle.", SetMandatoryFalse()),
	exclusions:         components.NewStringFlag(exclusions, "List of semicolon-separated(;) exclusions. Exclusions can include the * and the ? wildcards.", SetMandatoryFalse()),
	releaseNotesPath:   components.NewStringFlag(releaseNotesPath, "Path to a file describes the release notes for the release bundle version.", SetMandatoryFalse()),
	releaseNotesSyntax: components.NewStringFlag(releaseNotesSyntax, "The syntax for the release notes. Can be one of 'markdown', 'asciidoc', or 'plain_text.", SetMandatoryFalse()),
	rbPassphrase:       components.NewStringFlag(passphrase, "The passphrase for the signing key.", SetMandatoryFalse()),
	rbRepo:             components.NewStringFlag(repo, "A repository name at source Artifactory to store release bundle artifacts in. If not provided, Artifactory will use the default one.", SetMandatoryFalse()),
	InsecureTls:        components.NewBoolFlag(InsecureTls, "Set to true to skip TLS certificates verification.", WithBoolDefaultValueFalse()),
	distTarget:         components.NewStringFlag(target, "The target path for distributed artifacts on the edge node.", SetMandatoryFalse()),
	rbDetailedSummary:  components.NewBoolFlag(detailedSummary, "Set to true to get a command summary with details about the release bundle artifact.", WithBoolDefaultValueFalse()),
	DistRules:          components.NewStringFlag(DistRules, "Path to distribution rules.", SetMandatoryFalse()),
	site:               components.NewStringFlag(site, "Wildcard filter for site name.", SetMandatoryFalse()),
	city:               components.NewStringFlag(city, "Wildcard filter for site city name.", SetMandatoryFalse()),
	countryCodes:       components.NewStringFlag(countryCodes, "List of semicolon-separated(;) wildcard filters for site country codes.", SetMandatoryFalse()),
	sync:               components.NewBoolFlag(sync, "Set to true to enable sync distribution (the command execution will end when the distribution process ends).", WithBoolDefaultValueFalse()),
	maxWaitMinutes:     components.NewStringFlag(maxWaitMinutes, "Max minutes to wait for sync distribution.", WithStrDefaultValue("60")),
	deleteFromDist:     components.NewBoolFlag(deleteFromDist, "Set to true to delete release bundle version in JFrog Distribution itself after deletion is complete.", WithBoolDefaultValueFalse()),
	deleteQuiet:        components.NewBoolFlag(quiet, "Set to true to skip the delete confirmation message.", WithBoolDefaultValueFalse()),
	CreateRepo:         components.NewBoolFlag(CreateRepo, "Set to true to create the repository on the edge if it does not exist.", WithBoolDefaultValueFalse()),
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}

func SetMandatoryFalse() components.StringFlagOption {
	return func(f *components.StringFlag) {
		f.Mandatory = false
	}
}

func WithBoolDefaultValueFalse() components.BoolFlagOption {
	return func(f *components.BoolFlag) {
		f.DefaultValue = false
	}
}

func WithStrDefaultValue(defaultValue string) components.StringFlagOption {
	return func(f *components.StringFlag) {
		f.DefaultValue = defaultValue
	}
}
