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
		distUrl, user, password, accessToken, serverId, rbDryRun, DistRules, quiet,
		site, city, countryCodes, sync, maxWaitMinutes, InsecureTls, deleteFromDist, deleteQuiet,
	},
}

var flagsMap = map[string]components.Flag{
	distUrl:            components.NewStringFlag(distUrl, "Optional: JFrog Distribution URL. (example: https://acme.jfrog.io/distribution)"),
	user:               components.NewStringFlag(user, "Optional: JFrog username."),
	password:           components.NewStringFlag(password, "Optional: JFrog password."),
	accessToken:        components.NewStringFlag(accessToken, "Optional: JFrog access token."),
	serverId:           components.NewStringFlag(serverId, "Optional: Server ID configured using the 'jf config' command."),
	specFlag:           components.NewStringFlag(specFlag, "Optional: Path to a File Spec."),
	specVars:           components.NewStringFlag(specVars, "Optional: List of semicolon-separated(;) variables in the form of \"key1=value1;key2=value2;...\" to be replaced in the File Spec."),
	targetProps:        components.NewStringFlag(targetProps, "Optional: List of semicolon-separated(;) properties, in the form of \"key1=value1;key2=value2;...\" to be added to the artifacts after distribution of the release bundle."),
	rbDryRun:           components.NewBoolFlag(rbDryRun, "Default: false: Set to true to disable communication with JFrog Distribution."),
	sign:               components.NewBoolFlag(sign, "Default: false: If set to true, automatically signs the release bundle version."),
	desc:               components.NewStringFlag(desc, "Optional: Description of the release bundle."),
	exclusions:         components.NewStringFlag(exclusions, "Optional: List of semicolon-separated(;) exclusions. Exclusions can include the * and the ? wildcards."),
	releaseNotesPath:   components.NewStringFlag(releaseNotesPath, "Optional: Path to a file describes the release notes for the release bundle version."),
	releaseNotesSyntax: components.NewStringFlag(releaseNotesSyntax, "Default: plain_text: The syntax for the release notes. Can be one of 'markdown', 'asciidoc', or 'plain_text."),
	rbPassphrase:       components.NewStringFlag(rbPassphrase, "Optional: The passphrase for the signing key."),
	rbRepo:             components.NewStringFlag(rbRepo, "Optional: A repository name at source Artifactory to store release bundle artifacts in. If not provided, Artifactory will use the default one."),
	InsecureTls:        components.NewBoolFlag(InsecureTls, "Default: false: Set to true to skip TLS certificates verification."),
	distTarget:         components.NewStringFlag(distTarget, "Optional: The target path for distributed artifacts on the edge node."),
	rbDetailedSummary:  components.NewBoolFlag(rbDetailedSummary, "Default: false: Set to true to get a command summary with details about the release bundle artifact."),
	DistRules:          components.NewStringFlag(DistRules, "Optional: Path to distribution rules."),
	site:               components.NewStringFlag(site, "Default: '*': Wildcard filter for site name."),
	city:               components.NewStringFlag(city, "Default: '*': Wildcard filter for site city name."),
	countryCodes:       components.NewStringFlag(countryCodes, "Default: '*': List of semicolon-separated(;) wildcard filters for site country codes."),
	sync:               components.NewBoolFlag(sync, "Default: false: Set to true to enable sync distribution (the command execution will end when the distribution process ends)."),
	maxWaitMinutes:     components.NewStringFlag(maxWaitMinutes, "Default: 60: Max minutes to wait for sync distribution."),
	deleteFromDist:     components.NewBoolFlag(deleteFromDist, "Default: false: Set to true to delete release bundle version in JFrog Distribution itself after deletion is complete."),
	deleteQuiet:        components.NewBoolFlag(quiet, "Default: false: Set to true to skip the delete confirmation message."),
	CreateRepo:         components.NewBoolFlag(CreateRepo, "Default: false: Set to true to create the repository on the edge if it does not exist."),
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
