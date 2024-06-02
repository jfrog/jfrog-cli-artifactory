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

	// Unique evidence flags
	evidencePrefix   = "evd-"
	EvdPredicate     = "predicate"
	EvdPredicateType = "predicate-type"
	EvdSubject       = "subject"
	EvdKey           = "key"
	EvdKeyId         = "key-name"
)

// Flag keys mapped to their corresponding components.Flag definition.
var flagsMap = map[string]components.Flag{
	// Common commands flags
	ServerId:    components.NewStringFlag(ServerId, "Server ID configured using the config command.", func(f *components.StringFlag) { f.Mandatory = false }),
	url:         components.NewStringFlag(url, "JFrog Platform URL.", func(f *components.StringFlag) { f.Mandatory = false }),
	user:        components.NewStringFlag(user, "JFrog username.", func(f *components.StringFlag) { f.Mandatory = false }),
	password:    components.NewStringFlag(password, "JFrog password.", func(f *components.StringFlag) { f.Mandatory = false }),
	accessToken: components.NewStringFlag(accessToken, "JFrog access token.", func(f *components.StringFlag) { f.Mandatory = false }),

	EvdPredicate:     components.NewStringFlag(EvdPredicate, "Path for a file containing the predicate. The file should contain a valid JSON predicate.", func(f *components.StringFlag) { f.Mandatory = true }),
	EvdPredicateType: components.NewStringFlag(EvdPredicateType, "The type of the predicate.", func(f *components.StringFlag) { f.Mandatory = true }),
	EvdSubject:       components.NewStringFlag(EvdSubject, "Path for a file containing the subject.", func(f *components.StringFlag) { f.Mandatory = true }),
	EvdKey:           components.NewStringFlag(EvdKey, "Private key.", func(f *components.StringFlag) { f.Mandatory = true }),
	EvdKeyId:         components.NewStringFlag(EvdKeyId, "KeyId", func(f *components.StringFlag) { f.Mandatory = false }),
}

var commandFlags = map[string][]string{
	CreateEvidence: {
		url, user, password, accessToken, ServerId, EvdPredicate, EvdPredicateType, EvdSubject, EvdKey, EvdKeyId,
	},
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
