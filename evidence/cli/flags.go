package cli

import (
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	// Evidence commands keys
	CreateEvidence = "create-evidence"
)

const (
	// Base flags keys
	ServerId    = "server-id"
	url         = "url"
	user        = "user"
	accessToken = "access-token"
	project     = "project"

	// RLM flags keys
	releaseBundle        = "release-bundle"
	releaseBundleVersion = "release-bundle-version"
	buildName            = "build-name"
	buildNumber          = "build-number"
	packageName          = "package-name"
	packageVersion       = "package-version"
	packageRepoName      = "package-repo-name"
	typeFlag             = "type"

	// Unique evidence flags
	predicate       = "predicate"
	predicateType   = "predicate-type"
	markdown        = "markdown"
	subjectRepoPath = "subject-repo-path"
	subjectSha256   = "subject-sha256"
	key             = "key"
	keyAlias        = "key-alias"
)

// Flag keys mapped to their corresponding components.Flag definition.
var flagsMap = map[string]components.Flag{
	// Common commands flags
	ServerId:    components.NewStringFlag(ServerId, "Server ID configured using the config command.", func(f *components.StringFlag) { f.Mandatory = false }),
	url:         components.NewStringFlag(url, "JFrog Platform URL.", func(f *components.StringFlag) { f.Mandatory = false }),
	user:        components.NewStringFlag(user, "JFrog username.", func(f *components.StringFlag) { f.Mandatory = false }),
	accessToken: components.NewStringFlag(accessToken, "JFrog access token.", func(f *components.StringFlag) { f.Mandatory = false }),
	project:     components.NewStringFlag(project, "Project key associated with the created evidence.", func(f *components.StringFlag) { f.Mandatory = false }),

	releaseBundle:        components.NewStringFlag(releaseBundle, "Release Bundle name.", func(f *components.StringFlag) { f.Mandatory = false }),
	releaseBundleVersion: components.NewStringFlag(releaseBundleVersion, "Release Bundle version.", func(f *components.StringFlag) { f.Mandatory = false }),
	buildName:            components.NewStringFlag(buildName, "Build name.", func(f *components.StringFlag) { f.Mandatory = false }),
	buildNumber:          components.NewStringFlag(buildNumber, "Build number.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageName:          components.NewStringFlag(packageName, "Package name.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageVersion:       components.NewStringFlag(packageVersion, "Package version.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageRepoName:      components.NewStringFlag(packageRepoName, "Package repository Name.", func(f *components.StringFlag) { f.Mandatory = false }),
	typeFlag:             components.NewStringFlag(typeFlag, "Type can contain 'gh-commiter' value.", func(f *components.StringFlag) { f.Mandatory = false }),

	predicate:       components.NewStringFlag(predicate, "Path to the predicate, arbitrary JSON.", func(f *components.StringFlag) { f.Mandatory = true }),
	predicateType:   components.NewStringFlag(predicateType, "Type of the predicate.", func(f *components.StringFlag) { f.Mandatory = true }),
	markdown:        components.NewStringFlag(markdown, "Markdown of the predicate.", func(f *components.StringFlag) { f.Mandatory = false }),
	subjectRepoPath: components.NewStringFlag(subjectRepoPath, "Full path to some subject' location.", func(f *components.StringFlag) { f.Mandatory = false }),
	subjectSha256:   components.NewStringFlag(subjectSha256, "Subject checksum sha256.", func(f *components.StringFlag) { f.Mandatory = false }),
	key:             components.NewStringFlag(key, "Path to a private key that will sign the DSSE. Supported keys: 'ecdsa','rsa' and 'ed25519'.", func(f *components.StringFlag) { f.Mandatory = false }),
	keyAlias:        components.NewStringFlag(keyAlias, "Key alias", func(f *components.StringFlag) { f.Mandatory = false }),
}

var commandFlags = map[string][]string{
	CreateEvidence: {
		url,
		user,
		accessToken,
		ServerId,
		project,
		releaseBundle,
		releaseBundleVersion,
		buildName,
		buildNumber,
		packageName,
		packageVersion,
		packageRepoName,
		typeFlag,
		predicate,
		predicateType,
		markdown,
		subjectRepoPath,
		subjectSha256,
		key,
		keyAlias,
	},
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
