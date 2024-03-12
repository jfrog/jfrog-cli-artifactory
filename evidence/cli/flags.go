package cli

import (
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	// Evidence commands keys
	CreateEvidence = "create-evidence"
	VerifyEvidence = "verify"
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
	EvdSubjects      = "subjects"
	EvdKey           = "key"
	EvdKeyId         = "key-name"
	EvdName          = "name"
	EvdOverride      = "override"
)

// Security Flag keys mapped to their corresponding components.Flag definition.
var flagsMap = map[string]components.Flag{
	// Common commands flags
	ServerId:    components.NewStringFlag(ServerId, "Server ID configured using the config command."),
	url:         components.NewStringFlag(url, "JFrog Platform URL."),
	user:        components.NewStringFlag(user, "JFrog username."),
	password:    components.NewStringFlag(password, "JFrog password."),
	accessToken: components.NewStringFlag(accessToken, "JFrog access token."),

	EvdPredicate:     components.NewStringFlag(EvdPredicate, "[Mandatory] Path for a file containing the predicate. The file should contain a valid JSON predicate."),
	EvdPredicateType: components.NewStringFlag(EvdPredicateType, "[Mandatory] The type of the predicate."),
	EvdSubjects:      components.NewStringFlag(EvdSubjects, "[Mandatory] Path for a file containing the subject."),
	EvdKey:           components.NewStringFlag(EvdKey, "[Mandatory] Path for a key pair (pk, puk)."),
	EvdKeyId:         components.NewStringFlag(EvdKeyId, "[Optional] KeyId"),
	EvdName:          components.NewStringFlag(EvdName, "[Optional] The name of the evidence to be created."),
	EvdOverride:      components.NewBoolFlag(EvdOverride, "[Default: false] Set to true to override evidence, if exists, in Artifactory."),
}

var commandFlags = map[string][]string{
	CreateEvidence: {
		url, user, password, accessToken, ServerId, EvdPredicate, EvdPredicateType, EvdSubjects, EvdKey, EvdKeyId, EvdName, EvdOverride,
	},
	VerifyEvidence: {
		EvdKey, EvdName,
	},
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
