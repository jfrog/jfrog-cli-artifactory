package cli

import (
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	// Evidence commands keys
	CreateEvidence = "create-evidence"
	GetEvidence    = "get-evidence"
	VerifyEvidence = "verify-evidence"
)

const (
	// Base flags keys
	ServerId    = "server-id"
	url         = "url"
	user        = "user"
	accessToken = "access-token"
	project     = "project"
	format      = "format"
	output      = "output"

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
	predicate          = "predicate"
	predicateType      = "predicate-type"
	includePredicate   = "include-predicate"
	markdown           = "markdown"
	subjectRepoPath    = "subject-repo-path"
	subjectSha256      = "subject-sha256"
	key                = "key"
	keyAlias           = "key-alias"
	providerId         = "provider-id"
	publicKeys         = "public-keys"
	useArtifactoryKeys = "use-artifactory-keys"
	useSonarPredicate  = "use-sonar-predicate"
	sigstoreBundle     = "sigstore-bundle"
	artifactsLimit     = "artifacts-limit"
)

// Flag keys mapped to their corresponding components.Flag definition.
var flagsMap = map[string]components.Flag{
	// Common commands flags
	ServerId:    components.NewStringFlag(ServerId, "Server ID configured using the config command.", func(f *components.StringFlag) { f.Mandatory = false }),
	url:         components.NewStringFlag(url, "JFrog Platform URL.", func(f *components.StringFlag) { f.Mandatory = false }),
	user:        components.NewStringFlag(user, "JFrog username.", func(f *components.StringFlag) { f.Mandatory = false }),
	accessToken: components.NewStringFlag(accessToken, "JFrog access token.", func(f *components.StringFlag) { f.Mandatory = false }),
	project:     components.NewStringFlag(project, "Project key associated with the created evidence.", func(f *components.StringFlag) { f.Mandatory = false }),
	format:      components.NewStringFlag(format, "Output format. Supported formats: 'json'. For 'jf evd get' command you can additionally choose 'jsonl' format", func(f *components.StringFlag) { f.Mandatory = false }),
	output:      components.NewStringFlag(output, "Output file path, should be in the format of 'path/to/file.json'. If not provided, output will be printed to the console.", func(f *components.StringFlag) { f.Mandatory = false }),

	releaseBundle:        components.NewStringFlag(releaseBundle, "Release Bundle name.", func(f *components.StringFlag) { f.Mandatory = false }),
	releaseBundleVersion: components.NewStringFlag(releaseBundleVersion, "Release Bundle version.", func(f *components.StringFlag) { f.Mandatory = false }),
	buildName:            components.NewStringFlag(buildName, "Build name.", func(f *components.StringFlag) { f.Mandatory = false }),
	buildNumber:          components.NewStringFlag(buildNumber, "Build number.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageName:          components.NewStringFlag(packageName, "Package name.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageVersion:       components.NewStringFlag(packageVersion, "Package version.", func(f *components.StringFlag) { f.Mandatory = false }),
	packageRepoName:      components.NewStringFlag(packageRepoName, "Package repository Name.", func(f *components.StringFlag) { f.Mandatory = false }),
	typeFlag:             components.NewStringFlag(typeFlag, "Type can contain 'gh-commiter' value.", func(f *components.StringFlag) { f.Mandatory = false }),

	predicate:        components.NewStringFlag(predicate, "Path to the predicate, arbitrary JSON. Mandatory unless --"+sigstoreBundle+" is used", func(f *components.StringFlag) { f.Mandatory = false }),
	predicateType:    components.NewStringFlag(predicateType, "Type of the predicate. Mandatory unless --"+sigstoreBundle+" is used", func(f *components.StringFlag) { f.Mandatory = false }),
	includePredicate: components.NewBoolFlag(includePredicate, "Include the predicate data in the get evidence output.", components.WithBoolDefaultValueFalse()),
	markdown:         components.NewStringFlag(markdown, "Markdown of the predicate.", func(f *components.StringFlag) { f.Mandatory = false }),
	subjectRepoPath:  components.NewStringFlag(subjectRepoPath, "Full path to some subject location.", func(f *components.StringFlag) { f.Mandatory = false }),
	subjectSha256:    components.NewStringFlag(subjectSha256, "Subject checksum sha256.", func(f *components.StringFlag) { f.Mandatory = false }),
	key:              components.NewStringFlag(key, "Path to a private key that will sign the DSSE. Supported keys: 'ecdsa','rsa' and 'ed25519'.", func(f *components.StringFlag) { f.Mandatory = false }),
	keyAlias:         components.NewStringFlag(keyAlias, "Key alias", func(f *components.StringFlag) { f.Mandatory = false }),

	providerId:         components.NewStringFlag(providerId, "Provider ID for the evidence.", func(f *components.StringFlag) { f.Mandatory = false }),
	publicKeys:         components.NewStringFlag(publicKeys, "Array of paths to public keys for signatures verification with \";\" separator. Supported keys: 'ecdsa','rsa' and 'ed25519'.", func(f *components.StringFlag) { f.Mandatory = false }),
	sigstoreBundle:     components.NewStringFlag(sigstoreBundle, "Path to a Sigstore bundle file with a pre-signed DSSE envelope. Incompatible with --"+key+", --"+keyAlias+", --"+predicate+", --"+predicateType+" and --"+subjectSha256+".", func(f *components.StringFlag) { f.Mandatory = false }),
	useArtifactoryKeys: components.NewBoolFlag(useArtifactoryKeys, "Use Artifactory keys for verification. When enabled, the verify command retrieves keys from Artifactory.", components.WithBoolDefaultValueFalse()),
	artifactsLimit:     components.NewStringFlag(artifactsLimit, "The number of artifacts in a release bundle to be included in the evidences file. The default value is 1000 artifacts", func(f *components.StringFlag) { f.Mandatory = false }),
	useSonarPredicate:  components.NewBoolFlag(useSonarPredicate, "Use SonarQube predicate generation. When enabled, automatically generates predicate from SonarQube analysis data. Required SONAR_TOKEN or SONARQUBE_TOKEN environment variable", components.WithBoolDefaultValueFalse()),
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
		providerId,
		useSonarPredicate,
		sigstoreBundle,
	},
	VerifyEvidence: {
		url,
		user,
		accessToken,
		ServerId,
		publicKeys,
		format,
		project,
		releaseBundle,
		releaseBundleVersion,
		subjectRepoPath,
		buildName,
		buildNumber,
		packageName,
		packageVersion,
		packageRepoName,
		useArtifactoryKeys,
	},
	GetEvidence: {
		url,
		user,
		accessToken,
		ServerId,
		format,
		output,
		project,
		releaseBundle,
		releaseBundleVersion,
		subjectRepoPath,
		includePredicate,
		artifactsLimit,
	},
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
