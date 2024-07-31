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
	password    = "password"
	accessToken = "access-token"
	project     = "project"

	// RLM flags keys
	releaseBundle = "release-bundle"

	// Unique evidence flags
	evidencePrefix = "evd-"
	predicate      = "predicate"
	predicateType  = "predicate-type"
	repoPath       = "repo-path"
	key            = "key"
	keyId          = "key-name"
)

// Flag keys mapped to their corresponding components.Flag definition.
var flagsMap = map[string]components.Flag{
	// Common commands flags
	ServerId:    components.NewStringFlag(ServerId, "Server ID configured using the config command.", func(f *components.StringFlag) { f.Mandatory = false }),
	url:         components.NewStringFlag(url, "JFrog Platform URL.", func(f *components.StringFlag) { f.Mandatory = false }),
	user:        components.NewStringFlag(user, "JFrog username.", func(f *components.StringFlag) { f.Mandatory = false }),
	password:    components.NewStringFlag(password, "JFrog password.", func(f *components.StringFlag) { f.Mandatory = false }),
	accessToken: components.NewStringFlag(accessToken, "JFrog access token.", func(f *components.StringFlag) { f.Mandatory = false }),
	project:     components.NewStringFlag(project, "Project key associated with the created evidence.", func(f *components.StringFlag) { f.Mandatory = false }),

	releaseBundle: components.NewStringFlag(releaseBundle, "Release Bundle name and version. Format: <name>:<version>", func(f *components.StringFlag) { f.Mandatory = false }),

	predicate:     components.NewStringFlag(predicate, "Path to the predicate, arbitrary JSON.", func(f *components.StringFlag) { f.Mandatory = true }),
	predicateType: components.NewStringFlag(predicateType, "Type of the predicate.", func(f *components.StringFlag) { f.Mandatory = true }),
	repoPath:      components.NewStringFlag(repoPath, "Full path to some artifact' location.", func(f *components.StringFlag) { f.Mandatory = false }),
	key:           components.NewStringFlag(key, "Path to a private key that will sign the DSSE. Supported keys: 'ecdsa','rsa' and 'ed25519'.", func(f *components.StringFlag) { f.Mandatory = true }),
	keyId:         components.NewStringFlag(keyId, "KeyId", func(f *components.StringFlag) { f.Mandatory = false }),
}

var commandFlags = map[string][]string{
	CreateEvidence: {
		url, user, password, accessToken, ServerId, project, releaseBundle, predicate, predicateType, repoPath, key, keyId,
	},
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
