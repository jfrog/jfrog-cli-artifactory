package flagkit

import (
	"strconv"

	"github.com/jfrog/jfrog-cli-artifactory/cliutils/cmddefs"
	commonCliUtils "github.com/jfrog/jfrog-cli-core/v2/common/cliutils"
	pluginsCommon "github.com/jfrog/jfrog-cli-core/v2/plugins/common"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

const (
	DownloadMinSplitKb    = 5120
	DownloadSplitCount    = 3
	DownloadMaxSplitCount = 15

	// Upload
	UploadMinSplitMb    = 200
	UploadSplitCount    = 5
	UploadChunkSizeMb   = 20
	UploadMaxSplitCount = 100

	// Common
	Retries                = 3
	RetryWaitMilliSecs     = 0
	ArtifactoryTokenExpiry = 3600
	ChunkSize              = "chunk-size"

	// Artifactory's Commands Keys
	Upload                 = "upload"
	Download               = "download"
	Move                   = "move"
	Copy                   = "copy"
	Delete                 = "delete"
	Properties             = "properties"
	Search                 = "search"
	BuildPublish           = "build-publish"
	BuildAppend            = "build-append"
	BuildScanLegacy        = "build-scan-legacy"
	BuildPromote           = "build-promote"
	BuildDiscard           = "build-discard"
	BuildAddDependencies   = "build-add-dependencies"
	BuildAddGit            = "build-add-git"
	BuildCollectEnv        = "build-collect-env"
	GitLfsClean            = "git-lfs-clean"
	Mvn                    = "mvn"
	MvnConfig              = "mvn-config"
	CocoapodsConfig        = "cocoapods-config"
	SwiftConfig            = "swift-config"
	Gradle                 = "gradle"
	GradleConfig           = "gradle-config"
	DockerPromote          = "docker-promote"
	Docker                 = "docker"
	DockerPush             = "docker-push"
	DockerPull             = "docker-pull"
	ContainerPull          = "container-pull"
	ContainerPush          = "container-push"
	BuildDockerCreate      = "build-docker-create"
	OcStartBuild           = "oc-start-build"
	NpmConfig              = "npm-config"
	NpmInstallCi           = "npm-install-ci"
	NpmPublish             = "npm-publish"
	PnpmConfig             = "pnpm-config"
	YarnConfig             = "yarn-config"
	Yarn                   = "yarn"
	NugetConfig            = "nuget-config"
	Nuget                  = "nuget"
	Dotnet                 = "dotnet"
	DotnetConfig           = "dotnet-config"
	Go                     = "go"
	GoConfig               = "go-config"
	GoPublish              = "go-publish"
	PipInstall             = "pip-install"
	PipConfig              = "pip-config"
	TerraformConfig        = "terraform-config"
	Terraform              = "terraform"
	Twine                  = "twine"
	PipenvConfig           = "pipenv-config"
	PipenvInstall          = "pipenv-install"
	PoetryConfig           = "poetry-config"
	Poetry                 = "poetry"
	Ping                   = "ping"
	RtCurl                 = "rt-curl"
	TemplateConsumer       = "template-consumer"
	RepoDelete             = "repo-delete"
	ReplicationDelete      = "replication-delete"
	PermissionTargetDelete = "permission-target-delete"
	// #nosec G101 -- False positive - no hardcoded credentials.
	ArtifactoryAccessTokenCreate = "artifactory-access-token-create"
	UserCreate                   = "user-create"
	UsersCreate                  = "users-create"
	UsersDelete                  = "users-delete"
	GroupCreate                  = "group-create"
	GroupAddUsers                = "group-add-users"
	GroupDelete                  = "group-delete"
	passphrase                   = "passphrase"

	// Config commands keys
	AddConfig    = "config-add"
	EditConfig   = "config-edit"
	DeleteConfig = "delete-config"

	// *** Artifactory Commands' flags ***
	// Base flags
	url         = "url"
	platformUrl = "platform-url"
	user        = "user"
	password    = "password"
	accessToken = "access-token"
	serverId    = "server-id"

	passwordStdin    = "password-stdin"
	accessTokenStdin = "access-token-stdin"

	// Ssh flags
	sshKeyPath    = "ssh-key-path"
	sshPassphrase = "ssh-passphrase"

	// Client certification flags
	ClientCertPath    = "client-cert-path"
	ClientCertKeyPath = "client-cert-key-path"
	InsecureTls       = "insecure-tls"

	// Sort & limit flags
	sortBy    = "sort-by"
	sortOrder = "sort-order"
	limit     = "limit"
	offset    = "offset"

	// Spec flags
	specFlag = "spec"
	specVars = "spec-vars"

	// Build info flags
	BuildName   = "build-name"
	BuildNumber = "build-number"
	module      = "module"

	// Generic commands flags
	exclusions              = "exclusions"
	Recursive               = "recursive"
	flat                    = "flat"
	build                   = "build"
	excludeArtifacts        = "exclude-artifacts"
	includeDeps             = "include-deps"
	regexpFlag              = "regexp"
	retries                 = "retries"
	retryWaitTime           = "retry-wait-time"
	dryRun                  = "dry-run"
	explode                 = "explode"
	bypassArchiveInspection = "bypass-archive-inspection"
	includeDirs             = "include-dirs"
	props                   = "props"
	targetProps             = "target-props"
	excludeProps            = "exclude-props"
	failNoOp                = "fail-no-op"
	threads                 = "threads"
	syncDeletes             = "sync-deletes"
	quiet                   = "quiet"
	bundle                  = "bundle"
	publicGpgKey            = "gpg-key"
	archiveEntries          = "archive-entries"
	detailedSummary         = "detailed-summary"
	archive                 = "archive"
	syncDeletesQuiet        = syncDeletes + "-" + quiet
	antFlag                 = "ant"
	fromRt                  = "from-rt"
	transitive              = "transitive"
	Status                  = "status"
	MinSplit                = "min-split"
	SplitCount              = "split-count"
	chunkSize               = "chunk-size"

	// Config flags
	interactive   = "interactive"
	EncPassword   = "enc-password"
	BasicAuthOnly = "basic-auth-only"
	Overwrite     = "overwrite"

	// Unique upload flags
	uploadPrefix      = "upload-"
	uploadExclusions  = uploadPrefix + exclusions
	uploadRecursive   = uploadPrefix + Recursive
	uploadFlat        = uploadPrefix + flat
	uploadRegexp      = uploadPrefix + regexpFlag
	uploadExplode     = uploadPrefix + explode
	uploadTargetProps = uploadPrefix + targetProps
	uploadSyncDeletes = uploadPrefix + syncDeletes
	uploadArchive     = uploadPrefix + archive
	uploadMinSplit    = uploadPrefix + MinSplit
	uploadSplitCount  = uploadPrefix + SplitCount
	deb               = "deb"
	symlinks          = "symlinks"
	uploadAnt         = uploadPrefix + antFlag

	// Unique download flags
	downloadPrefix       = "download-"
	downloadRecursive    = downloadPrefix + Recursive
	downloadFlat         = downloadPrefix + flat
	downloadExplode      = downloadPrefix + explode
	downloadProps        = downloadPrefix + props
	downloadExcludeProps = downloadPrefix + excludeProps
	downloadSyncDeletes  = downloadPrefix + syncDeletes
	downloadMinSplit     = downloadPrefix + MinSplit
	downloadSplitCount   = downloadPrefix + SplitCount
	validateSymlinks     = "validate-symlinks"
	skipChecksum         = "skip-checksum"

	// Unique move flags
	movePrefix       = "move-"
	moveRecursive    = movePrefix + Recursive
	moveFlat         = movePrefix + flat
	moveProps        = movePrefix + props
	moveExcludeProps = movePrefix + excludeProps

	// Unique copy flags
	copyPrefix       = "copy-"
	copyRecursive    = copyPrefix + Recursive
	copyFlat         = copyPrefix + flat
	copyProps        = copyPrefix + props
	copyExcludeProps = copyPrefix + excludeProps

	// Unique delete flags
	deletePrefix       = "delete-"
	deleteRecursive    = deletePrefix + Recursive
	deleteProps        = deletePrefix + props
	deleteExcludeProps = deletePrefix + excludeProps
	deleteQuiet        = deletePrefix + quiet

	// Unique search flags
	searchInclude      = "include"
	searchPrefix       = "search-"
	searchRecursive    = searchPrefix + Recursive
	searchProps        = searchPrefix + props
	searchExcludeProps = searchPrefix + excludeProps
	count              = "count"
	searchTransitive   = searchPrefix + transitive

	// Unique properties flags
	propertiesPrefix  = "props-"
	propsRecursive    = propertiesPrefix + Recursive
	propsProps        = propertiesPrefix + props
	propsExcludeProps = propertiesPrefix + excludeProps

	// Unique go publish flags
	goPublishExclusions = GoPublish + exclusions

	// Unique build-publish flags
	buildPublishPrefix = "bp-"
	bpDryRun           = buildPublishPrefix + dryRun
	bpDetailedSummary  = buildPublishPrefix + detailedSummary
	envInclude         = "env-include"
	envExclude         = "env-exclude"
	buildUrl           = "build-url"
	Project            = "project"
	bpOverwrite        = "bpOverwrite"

	// Unique build-add-dependencies flags
	badPrefix    = "bad-"
	badDryRun    = badPrefix + dryRun
	badRecursive = badPrefix + Recursive
	badRegexp    = badPrefix + regexpFlag
	badFromRt    = badPrefix + fromRt
	badModule    = badPrefix + module

	// Unique build-add-git flags
	configFlag = "config"

	// Unique build-scan flags
	fail = "fail"

	// Unique build-promote flags
	buildPromotePrefix  = "bpr-"
	bprDryRun           = buildPromotePrefix + dryRun
	bprProps            = buildPromotePrefix + props
	comment             = "comment"
	sourceRepo          = "source-repo"
	includeDependencies = "include-dependencies"
	copyFlag            = "copy"
	failFast            = "fail-fast"

	Async = "async"

	// Unique build-discard flags
	buildDiscardPrefix = "bdi-"
	bdiAsync           = buildDiscardPrefix + Async
	maxDays            = "max-days"
	maxBuilds          = "max-builds"
	excludeBuilds      = "exclude-builds"
	deleteArtifacts    = "delete-artifacts"

	repo = "repo"

	// Unique git-lfs-clean flags
	glcPrefix = "glc-"
	glcDryRun = glcPrefix + dryRun
	glcQuiet  = glcPrefix + quiet
	glcRepo   = glcPrefix + repo
	refs      = "refs"

	// Build tool config flags
	global          = "global"
	serverIdResolve = "server-id-resolve"
	serverIdDeploy  = "server-id-deploy"
	repoResolve     = "repo-resolve"
	repoDeploy      = "repo-deploy"

	// Unique maven-config flags
	repoResolveReleases  = "repo-resolve-releases"
	repoResolveSnapshots = "repo-resolve-snapshots"
	repoDeployReleases   = "repo-deploy-releases"
	repoDeploySnapshots  = "repo-deploy-snapshots"
	includePatterns      = "include-patterns"
	excludePatterns      = "exclude-patterns"

	// Unique gradle-config flags
	usesPlugin          = "uses-plugin"
	UseWrapper          = "use-wrapper"
	deployMavenDesc     = "deploy-maven-desc"
	deployIvyDesc       = "deploy-ivy-desc"
	ivyDescPattern      = "ivy-desc-pattern"
	ivyArtifactsPattern = "ivy-artifacts-pattern"

	// Build tool flags
	deploymentThreads = "deployment-threads"
	skipLogin         = "skip-login"
	validateSha       = "validate-sha"

	// Unique docker promote flags
	dockerPromotePrefix = "docker-promote-"
	targetDockerImage   = "target-docker-image"
	sourceTag           = "source-tag"
	targetTag           = "target-tag"
	dockerPromoteCopy   = dockerPromotePrefix + Copy

	// Unique build docker create
	imageFile = "image-file"

	// Unique oc start-build flags
	ocStartBuildPrefix = "oc-start-build-"
	ocStartBuildRepo   = ocStartBuildPrefix + repo

	// Unique npm flags
	npmPrefix          = "npm-"
	npmDetailedSummary = npmPrefix + detailedSummary
	runNative          = "run-native"

	// Unique nuget/dotnet config flags
	nugetV2                  = "nuget-v2"
	allowInsecureConnections = "allow-insecure-connections"

	// Unique go flags
	noFallback = "no-fallback"

	// Unique Terraform flags
	namespace = "namespace"
	provider  = "provider"
	Tag       = "tag"

	// Template user flags
	vars = "vars"

	// User Management flags
	csv            = "csv"
	usersCreateCsv = "users-create-csv"
	usersDeleteCsv = "users-delete-csv"
	UsersGroups    = "users-groups"
	Replace        = "replace"
	Admin          = "admin"

	// Mutual *-access-token-create flags
	Groups      = "groups"
	GrantAdmin  = "grant-admin"
	Expiry      = "expiry"
	Refreshable = "refreshable"
	Audience    = "audience"

	// Unique artifactory-access-token-create flags
	artifactoryAccessTokenCreatePrefix = "rt-atc-"
	rtAtcGroups                        = artifactoryAccessTokenCreatePrefix + Groups
	rtAtcGrantAdmin                    = artifactoryAccessTokenCreatePrefix + GrantAdmin
	rtAtcExpiry                        = artifactoryAccessTokenCreatePrefix + Expiry
	rtAtcRefreshable                   = artifactoryAccessTokenCreatePrefix + Refreshable
	rtAtcAudience                      = artifactoryAccessTokenCreatePrefix + Audience

	// Unique Xray Flags for upload/publish commands
	xrayScan = "scan"

	// *** Distribution Commands' flags ***
	// Base flags
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

	// Unique offline-update flags
	target = "target"

	// Unique scan flags
	xrOutput            = "format"
	BypassArchiveLimits = "bypass-archive-limits"

	// Audit commands
	watches       = "watches"
	repoPath      = "repo-path"
	licenses      = "licenses"
	vuln          = "vuln"
	ExtendedTable = "extended-table"
	MinSeverity   = "min-severity"
	FixableOnly   = "fixable-only"

	// *** Config Commands' flags ***
	configPrefix      = "config-"
	configPlatformUrl = configPrefix + url
	configRtUrl       = "artifactory-url"
	configDistUrl     = "distribution-url"
	configXrUrl       = "xray-url"
	configMcUrl       = "mission-control-url"
	configPlUrl       = "pipelines-url"
	configAccessToken = configPrefix + accessToken
	configUser        = configPrefix + user
	configPassword    = configPrefix + password
	configInsecureTls = configPrefix + InsecureTls

	// Generic commands flags
	name            = "name"
	IncludeRepos    = "include-repos"
	ExcludeRepos    = "exclude-repos"
	IncludeProjects = "include-projects"
	ExcludeProjects = "exclude-projects"

	// Unique lifecycle flags
	Sync                     = "sync"
	lifecyclePrefix          = "lc-"
	lcSync                   = lifecyclePrefix + Sync
	lcProject                = lifecyclePrefix + Project
	Builds                   = "builds"
	lcBuilds                 = lifecyclePrefix + Builds
	ReleaseBundles           = "release-bundles"
	lcReleaseBundles         = lifecyclePrefix + ReleaseBundles
	SigningKey               = "signing-key"
	lcSigningKey             = lifecyclePrefix + SigningKey
	PathMappingPattern       = "mapping-pattern"
	lcPathMappingPattern     = lifecyclePrefix + PathMappingPattern
	PathMappingTarget        = "mapping-target"
	lcPathMappingTarget      = lifecyclePrefix + PathMappingTarget
	lcDryRun                 = lifecyclePrefix + dryRun
	lcIncludeRepos           = lifecyclePrefix + IncludeRepos
	lcExcludeRepos           = lifecyclePrefix + ExcludeRepos
	PromotionType            = "promotion-type"
	lcTag                    = lifecyclePrefix + Tag
	lcProperties             = lifecyclePrefix + Properties
	DeleteProperty           = "del-prop"
	lcDeleteProperties       = lifecyclePrefix + DeleteProperty
	SourceTypeReleaseBundles = "source-type-release-bundles"
	SourceTypeBuilds         = "source-type-builds"
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
	cmddefs.ReleaseBundleCreate: {
		platformUrl, user, password, accessToken, serverId, lcSigningKey, lcSync, lcProject, lcBuilds, lcReleaseBundles,
		specFlag, specVars, BuildName, BuildNumber, SourceTypeReleaseBundles, SourceTypeBuilds,
	},
	cmddefs.ReleaseBundlePromote: {
		platformUrl, user, password, accessToken, serverId, lcSigningKey, lcSync, lcProject, lcIncludeRepos,
		lcExcludeRepos, PromotionType,
	},
	cmddefs.ReleaseBundleDistribute: {
		platformUrl, user, password, accessToken, serverId, lcProject, DistRules, site, city, countryCodes,
		lcDryRun, CreateRepo, lcPathMappingPattern, lcPathMappingTarget, lcSync, maxWaitMinutes,
	},
	cmddefs.ReleaseBundleDeleteLocal: {
		platformUrl, user, password, accessToken, serverId, deleteQuiet, lcSync, lcProject,
	},
	cmddefs.ReleaseBundleDeleteRemote: {
		platformUrl, user, password, accessToken, serverId, deleteQuiet, lcDryRun, DistRules, site, city, countryCodes,
		lcSync, maxWaitMinutes, lcProject,
	},
	cmddefs.ReleaseBundleExport: {
		platformUrl, user, password, accessToken, serverId, lcPathMappingTarget, lcPathMappingPattern, Project,
		downloadMinSplit, downloadSplitCount,
	},
	cmddefs.ReleaseBundleImport: {
		user, password, accessToken, serverId, platformUrl,
	},
	cmddefs.ReleaseBundleAnnotate: {
		platformUrl, user, password, accessToken, serverId, lcProject, lcTag, lcProperties, lcDeleteProperties, propsRecursive,
	},
	AddConfig: {
		interactive, EncPassword, configPlatformUrl, configRtUrl, configDistUrl, configXrUrl, configMcUrl, configPlUrl, configUser, configPassword, configAccessToken, sshKeyPath, sshPassphrase, ClientCertPath,
		ClientCertKeyPath, BasicAuthOnly, configInsecureTls, Overwrite, passwordStdin, accessTokenStdin,
	},
	EditConfig: {
		interactive, EncPassword, configPlatformUrl, configRtUrl, configDistUrl, configXrUrl, configMcUrl, configPlUrl, configUser, configPassword, configAccessToken, sshKeyPath, sshPassphrase, ClientCertPath,
		ClientCertKeyPath, BasicAuthOnly, configInsecureTls, passwordStdin, accessTokenStdin,
	},
	DeleteConfig: {
		deleteQuiet,
	},
	Upload: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath, uploadTargetProps,
		ClientCertKeyPath, specFlag, specVars, BuildName, BuildNumber, module, uploadExclusions, deb,
		uploadRecursive, uploadFlat, uploadRegexp, retries, retryWaitTime, dryRun, uploadExplode, symlinks, includeDirs,
		failNoOp, threads, uploadSyncDeletes, syncDeletesQuiet, InsecureTls, detailedSummary, Project,
		uploadAnt, uploadArchive, uploadMinSplit, uploadSplitCount, chunkSize,
	},
	Download: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, BuildName, BuildNumber, module, exclusions, sortBy,
		sortOrder, limit, offset, downloadRecursive, downloadFlat, build, includeDeps, excludeArtifacts, downloadMinSplit, downloadSplitCount,
		retries, retryWaitTime, dryRun, downloadExplode, bypassArchiveInspection, validateSymlinks, bundle, publicGpgKey, includeDirs,
		downloadProps, downloadExcludeProps, failNoOp, threads, archiveEntries, downloadSyncDeletes, syncDeletesQuiet, InsecureTls, detailedSummary, Project,
		skipChecksum,
	},
	Move: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, exclusions, sortBy, sortOrder, limit, offset, moveRecursive,
		moveFlat, dryRun, build, includeDeps, excludeArtifacts, moveProps, moveExcludeProps, failNoOp, threads, archiveEntries,
		InsecureTls, retries, retryWaitTime, Project,
	},
	Copy: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, exclusions, sortBy, sortOrder, limit, offset, copyRecursive,
		copyFlat, dryRun, build, includeDeps, excludeArtifacts, bundle, copyProps, copyExcludeProps, failNoOp, threads,
		archiveEntries, InsecureTls, retries, retryWaitTime, Project,
	},
	Delete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, exclusions, sortBy, sortOrder, limit, offset,
		deleteRecursive, dryRun, build, includeDeps, excludeArtifacts, deleteQuiet, deleteProps, deleteExcludeProps, failNoOp, threads, archiveEntries,
		InsecureTls, retries, retryWaitTime, Project,
	},
	Search: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, exclusions, sortBy, sortOrder, limit, offset,
		searchRecursive, build, includeDeps, excludeArtifacts, count, bundle, includeDirs, searchProps, searchExcludeProps, failNoOp, archiveEntries,
		InsecureTls, searchTransitive, retries, retryWaitTime, Project, searchInclude,
	},
	Properties: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, specFlag, specVars, exclusions, sortBy, sortOrder, limit, offset,
		propsRecursive, build, includeDeps, excludeArtifacts, bundle, includeDirs, failNoOp, threads, archiveEntries, propsProps, propsExcludeProps,
		InsecureTls, retries, retryWaitTime, Project,
	},
	BuildPublish: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, buildUrl, bpDryRun,
		envInclude, envExclude, InsecureTls, Project, bpDetailedSummary, bpOverwrite,
	},
	BuildAppend: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, buildUrl, bpDryRun,
		envInclude, envExclude, InsecureTls, Project,
	},
	BuildAddDependencies: {
		specFlag, specVars, uploadExclusions, badRecursive, badRegexp, badDryRun, Project, badFromRt, serverId, badModule,
	},
	BuildAddGit: {
		configFlag, serverId, Project,
	},
	BuildCollectEnv: {
		Project,
	},
	BuildDockerCreate: {
		BuildName, BuildNumber, module, url, user, password, accessToken, sshPassphrase, sshKeyPath,
		serverId, imageFile, Project,
	},
	OcStartBuild: {
		BuildName, BuildNumber, module, Project, serverId, ocStartBuildRepo,
	},
	BuildScanLegacy: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, fail, InsecureTls,
		Project,
	},
	BuildPromote: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, Status, comment,
		sourceRepo, includeDependencies, copyFlag, failFast, bprDryRun, bprProps, InsecureTls, Project,
	},
	BuildDiscard: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, maxDays, maxBuilds,
		excludeBuilds, deleteArtifacts, bdiAsync, InsecureTls, Project,
	},
	GitLfsClean: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, refs, glcRepo, glcDryRun,
		glcQuiet, InsecureTls, retries, retryWaitTime,
	},
	CocoapodsConfig: {
		global, serverIdResolve, repoResolve,
	},
	SwiftConfig: {
		global, serverIdResolve, repoResolve,
	},
	MvnConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolveReleases, repoResolveSnapshots, repoDeployReleases, repoDeploySnapshots, includePatterns, excludePatterns, UseWrapper,
	},
	GradleConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolve, repoDeploy, usesPlugin, UseWrapper, deployMavenDesc,
		deployIvyDesc, ivyDescPattern, ivyArtifactsPattern,
	},
	Mvn: {
		BuildName, BuildNumber, deploymentThreads, InsecureTls, Project, detailedSummary, xrayScan, xrOutput,
	},
	Gradle: {
		BuildName, BuildNumber, deploymentThreads, Project, detailedSummary, xrayScan, xrOutput,
	},
	Docker: {
		BuildName, BuildNumber, module, Project,
		serverId, skipLogin, threads, detailedSummary, watches, repoPath, licenses, xrOutput, fail, ExtendedTable, BypassArchiveLimits, MinSeverity, FixableOnly, vuln,
	},
	DockerPush: {
		BuildName, BuildNumber, module, Project,
		serverId, skipLogin, threads, detailedSummary,
	},
	DockerPull: {
		BuildName, BuildNumber, module, Project,
		serverId, skipLogin,
	},
	DockerPromote: {
		targetDockerImage, sourceTag, targetTag, dockerPromoteCopy, url, user, password, accessToken, sshPassphrase, sshKeyPath,
		serverId,
	},
	ContainerPush: {
		BuildName, BuildNumber, module, url, user, password, accessToken, sshPassphrase, sshKeyPath,
		serverId, skipLogin, threads, Project, detailedSummary, validateSha,
	},
	ContainerPull: {
		BuildName, BuildNumber, module, url, user, password, accessToken, sshPassphrase, sshKeyPath,
		serverId, skipLogin, Project,
	},
	NpmConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolve, repoDeploy,
	},
	NpmInstallCi: {
		BuildName, BuildNumber, module, Project, runNative,
	},
	NpmPublish: {
		BuildName, BuildNumber, module, Project, npmDetailedSummary, xrayScan, xrOutput, runNative,
	},
	PnpmConfig: {
		global, serverIdResolve, repoResolve,
	},
	YarnConfig: {
		global, serverIdResolve, repoResolve,
	},
	Yarn: {
		BuildName, BuildNumber, module, Project,
	},
	NugetConfig: {
		global, serverIdResolve, repoResolve, nugetV2,
	},
	Nuget: {
		BuildName, BuildNumber, module, Project, allowInsecureConnections,
	},
	DotnetConfig: {
		global, serverIdResolve, repoResolve, nugetV2,
	},
	Dotnet: {
		BuildName, BuildNumber, module, Project,
	},
	GoConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolve, repoDeploy,
	},
	GoPublish: {
		url, user, password, accessToken, BuildName, BuildNumber, module, Project, detailedSummary, goPublishExclusions,
	},
	Go: {
		BuildName, BuildNumber, module, Project, noFallback,
	},
	TerraformConfig: {
		global, serverIdDeploy, repoDeploy,
	},
	Terraform: {
		namespace, provider, Tag, exclusions,
		BuildName, BuildNumber, module, Project,
	},
	Twine: {
		BuildName, BuildNumber, module, Project,
	},
	Ping: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, InsecureTls,
	},
	RtCurl: {
		serverId,
	},
	PipConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolve, repoDeploy,
	},
	PipInstall: {
		BuildName, BuildNumber, module, Project,
	},
	PipenvConfig: {
		global, serverIdResolve, serverIdDeploy, repoResolve, repoDeploy,
	},
	PipenvInstall: {
		BuildName, BuildNumber, module, Project,
	},
	PoetryConfig: {
		global, serverIdResolve, repoResolve,
	},
	Poetry: {
		BuildName, BuildNumber, module, Project,
	},
	TemplateConsumer: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, vars,
	},
	RepoDelete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, deleteQuiet,
	},
	ReplicationDelete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, deleteQuiet,
	},
	PermissionTargetDelete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, deleteQuiet,
	},
	ArtifactoryAccessTokenCreate: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, ClientCertPath,
		ClientCertKeyPath, rtAtcGroups, rtAtcGrantAdmin, rtAtcExpiry, rtAtcRefreshable, rtAtcAudience,
	},
	UserCreate: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId,
		UsersGroups, Replace, Admin,
	},
	UsersCreate: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId,
		usersCreateCsv, UsersGroups, Replace,
	},
	UsersDelete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId,
		usersDeleteCsv, deleteQuiet,
	},
	GroupCreate: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId,
		Replace,
	},
	GroupAddUsers: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId,
	},
	GroupDelete: {
		url, user, password, accessToken, sshPassphrase, sshKeyPath, serverId, deleteQuiet,
	},
}

var flagsMap = map[string]components.Flag{

	// Common commands flags
	url:               components.NewStringFlag(url, "JFrog Platform URL. (example: https://acme.jfrog.io)", components.SetMandatoryFalse()),
	user:              components.NewStringFlag(user, "JFrog username.", components.SetMandatoryFalse()),
	password:          components.NewStringFlag(password, "JFrog password.", components.SetMandatoryFalse()),
	accessToken:       components.NewStringFlag(accessToken, "JFrog access token.", components.SetMandatoryFalse()),
	sshPassphrase:     components.NewStringFlag(sshPassphrase, "SSH key passphrase.", components.SetMandatoryFalse()),
	sshKeyPath:        components.NewStringFlag(sshKeyPath, "SSH key file path.", components.SetMandatoryFalse()),
	serverId:          components.NewStringFlag(serverId, "Server ID configured using the 'jf config' command.", components.SetMandatoryFalse()),
	ClientCertPath:    components.NewStringFlag(ClientCertPath, "Client certificate file in PEM format.", components.SetMandatoryFalse()),
	ClientCertKeyPath: components.NewStringFlag(ClientCertKeyPath, "Private key file for the client certificate in PEM format.", components.SetMandatoryFalse()),
	specFlag:          components.NewStringFlag(specFlag, "Path to a File Spec.", components.SetMandatoryFalse()),
	specVars:          components.NewStringFlag(specVars, "[Optional] List of semicolon-separated(;) variables in the form of \"key1=value1;key2=value2;...\" (wrapped by quotes) to be replaced in the File Spec. In the File Spec, the variables should be used as follows: ${key1}.", components.SetMandatoryFalse()),
	BuildName:         components.NewStringFlag(BuildName, "[Optional] Providing this option will collect and record build info for this build name. Build number option is mandatory when this option is provided.", components.SetMandatoryFalse()),
	BuildNumber:       components.NewStringFlag(BuildNumber, "[Optional] Providing this option will collect and record build info for this build number. Build name option is mandatory when this option is provided.", components.SetMandatoryFalse()),
	module:            components.NewStringFlag(module, "[Optional] Optional module name for the build-info. Build name and number options are mandatory when this option is provided.", components.SetMandatoryFalse()),
	retries:           components.NewStringFlag(retries, "[Default: "+strconv.Itoa(Retries)+"] Number of HTTP retries.", components.SetMandatoryFalse()),
	retryWaitTime:     components.NewStringFlag(retryWaitTime, "[Default: 0] Number of seconds or milliseconds to wait between retries. The numeric value should either end with s for seconds or ms for milliseconds (for example: 10s or 100ms).", components.SetMandatoryFalse()),
	dryRun:            components.NewBoolFlag(dryRun, "[Default: false] Set to true to disable communication with Artifactory.", components.WithBoolDefaultValueFalse()),
	InsecureTls:       components.NewBoolFlag(InsecureTls, "[Default: false] Set to true to skip TLS certificates verification.", components.WithBoolDefaultValueFalse()),
	detailedSummary:   components.NewBoolFlag(detailedSummary, "[Default: false] Set to true to include a list of the affected files in the command summary.", components.WithBoolDefaultValueFalse()),
	Project:           components.NewStringFlag(Project, "[Optional] JFrog Artifactory project key.", components.SetMandatoryFalse()),
	failNoOp:          components.NewBoolFlag(failNoOp, "[Default: false] Set to true if you'd like the command to return exit code 2 in case of no files are affected.", components.WithBoolDefaultValueFalse()),
	threads:           components.NewStringFlag(threads, "[Default: "+strconv.Itoa(commonCliUtils.Threads)+"] Number of working threads.", components.SetMandatoryFalse()),
	syncDeletesQuiet:  components.NewBoolFlag(quiet, "[Default: $CI] Set to true to skip the sync-deletes confirmation message.", components.WithBoolDefaultValueFalse()),
	sortBy:            components.NewStringFlag(sortBy, "[Optional] List of semicolon-separated(;) fields to sort by. The fields must be part of the 'items' AQL domain. For more information, see %sjfrog-artifactory-documentation/artifactory-query-language.", components.SetMandatoryFalse()),
	bundle:            components.NewStringFlag(bundle, "[Optional] If specified, only artifacts of the specified bundle are matched. The value format is bundle-name/bundle-version.", components.SetMandatoryFalse()),
	imageFile:         components.NewStringFlag(imageFile, "[Mandatory] Path to a file which includes one line in the following format: <IMAGE-TAG>@sha256:<MANIFEST-SHA256>.", components.SetMandatoryTrue()),
	ocStartBuildRepo:  components.NewStringFlag(repo, "[Mandatory] The name of the repository to which the image was pushed.", components.SetMandatoryTrue()),
	runNative:         components.NewBoolFlag(runNative, "[Default: false] Set to true if you'd like to use the native client configurations. Note: This flag would invoke native client behind the scenes, has performance implications and does not support deployment view and detailed summary.", components.WithBoolDefaultValueFalse()),

	// Config specific commands flags
	interactive:       components.NewBoolFlag(interactive, "[Default: true, unless $CI is true] Set to false if you do not want the config command to be interactive. If true, the --url option becomes optional.", components.WithBoolDefaultValueFalse()),
	EncPassword:       components.NewBoolFlag(EncPassword, "[Default: true] If set to false then the configured password will not be encrypted using Artifactory's encryption API.", components.WithBoolDefaultValueFalse()),
	configPlatformUrl: components.NewStringFlag(url, "[Optional] JFrog platform URL. (example: https://acme.jfrog.io)", components.SetMandatoryFalse()),
	configRtUrl:       components.NewStringFlag(configRtUrl, "[Optional] JFrog Artifactory URL. (example: https://acme.jfrog.io/artifactory)", components.SetMandatoryFalse()),
	configDistUrl:     components.NewStringFlag(configDistUrl, "[Optional] JFrog Distribution URL. (example: https://acme.jfrog.io/distribution)", components.SetMandatoryFalse()),
	configXrUrl:       components.NewStringFlag(configXrUrl, "[Optional] JFrog Xray URL. (example: https://acme.jfrog.io/xray)", components.SetMandatoryFalse()),
	configMcUrl:       components.NewStringFlag(configMcUrl, "[Optional] JFrog Mission Control URL. (example: https://acme.jfrog.io/mc)", components.SetMandatoryFalse()),
	configPlUrl:       components.NewStringFlag(configPlUrl, "[Optional] JFrog Pipelines URL. (example: https://acme.jfrog.io/pipelines)", components.SetMandatoryFalse()),
	configUser:        components.NewStringFlag(user, "[Optional] JFrog Platform username.", components.SetMandatoryFalse()),
	configPassword:    components.NewStringFlag(password, "[Optional] JFrog Platform password or API key.", components.SetMandatoryFalse()),
	configAccessToken: components.NewStringFlag(accessToken, "[Optional] JFrog Platform access token.", components.SetMandatoryFalse()),
	BasicAuthOnly:     components.NewBoolFlag(BasicAuthOnly, "[Default: false] Set to true to disable replacing username and password/API key with an automatically created access token that's refreshed hourly. Username and password/API key will still be used with commands which use external tools or the JFrog Distribution service. Can only be passed along with username and password/API key options.", components.WithBoolDefaultValueFalse()),
	configInsecureTls: components.NewBoolFlag(InsecureTls, "[Default: false] Set to true to skip TLS certificates verification, while encrypting the Artifactory password during the config process.", components.WithBoolDefaultValueFalse()),
	Overwrite:         components.NewBoolFlag(Overwrite, "[Default: false] Overwrites the instance configuration if an instance with the same ID already exists.", components.WithBoolDefaultValueFalse()),
	passwordStdin:     components.NewBoolFlag(passwordStdin, "[Default: false] Set to true if you'd like to provide the password via stdin.", components.WithBoolDefaultValueFalse()),
	accessTokenStdin:  components.NewBoolFlag(accessTokenStdin, "[Default: false] Set to true if you'd like to provide the access token via stdin.", components.WithBoolDefaultValueFalse()),

	// Download specific commands flags
	exclusions:              components.NewStringFlag(exclusions, "[Optional] List of semicolon-separated(;) exclusions. Exclusions can include the * and the ? wildcards.", components.SetMandatoryFalse()),
	sortOrder:               components.NewStringFlag(sortOrder, "[Default: asc] The order by which fields in the 'sort-by' option should be sorted. Accepts 'asc' or 'desc'.", components.SetMandatoryFalse()),
	limit:                   components.NewStringFlag(limit, "[Optional] The maximum number of items to fetch. Usually used with the 'sort-by' option.", components.SetMandatoryFalse()),
	offset:                  components.NewStringFlag(offset, "[Optional] The offset from which to fetch items (i.e. how many items should be skipped). Usually used with the 'sort-by' option.", components.SetMandatoryFalse()),
	downloadRecursive:       components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to include the download of artifacts inside sub-folders in Artifactory.", components.WithBoolDefaultValueFalse()),
	downloadFlat:            components.NewBoolFlag(flat, "[Default: false] Set to true if you do not wish to have the Artifactory repository path structure created locally for your downloaded files.", components.WithBoolDefaultValueFalse()),
	build:                   components.NewStringFlag(build, "[Optional] If specified, only artifacts of the specified build are matched. The property format is build-name/build-number. If you do not specify the build number, the artifacts are filtered by the latest build number. If the build is assigned to a specific project please provide the project key using the --project flag.", components.SetMandatoryFalse()),
	includeDeps:             components.NewStringFlag(includeDeps, "[Default: false] If specified, also dependencies of the specified build are matched. Used together with the --build flag.", components.SetMandatoryFalse()),
	excludeArtifacts:        components.NewStringFlag(excludeArtifacts, "[Default: false] If specified, build artifacts are not matched. Used together with the --build flag.", components.SetMandatoryFalse()),
	downloadMinSplit:        components.NewStringFlag(MinSplit, "[Default: "+strconv.Itoa(DownloadMinSplitKb)+"] Minimum file size in KB to split into ranges when downloading. Set to -1 for no splits.", components.SetMandatoryFalse()),
	downloadSplitCount:      components.NewStringFlag(SplitCount, "[Default: "+strconv.Itoa(DownloadSplitCount)+"] Number of parts to split a file when downloading. Set to 0 for no splits.", components.SetMandatoryFalse()),
	downloadExplode:         components.NewBoolFlag(explode, "[Default: false] Set to true to extract an archive after it is downloaded from Artifactory.", components.WithBoolDefaultValueFalse()),
	bypassArchiveInspection: components.NewBoolFlag(bypassArchiveInspection, "[Default: false] Set to true to bypass the archive security inspection before it is unarchived. Used with the 'explode' option.", components.WithBoolDefaultValueFalse()),
	validateSymlinks:        components.NewBoolFlag(validateSymlinks, "[Default: false] Set to true to perform a checksum validation when downloading symbolic links.", components.WithBoolDefaultValueFalse()),
	publicGpgKey:            components.NewStringFlag(publicGpgKey, "[Optional] Path to the public GPG key file located on the file system, used to validate downloaded release bundles.", components.SetMandatoryFalse()),
	includeDirs:             components.NewBoolFlag(includeDirs, "[Default: false] Set to true if you'd like to also apply the source path pattern for directories and not just for files.", components.WithBoolDefaultValueFalse()),
	downloadProps:           components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties will be downloaded.", components.SetMandatoryFalse()),
	downloadExcludeProps:    components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties will be downloaded.", components.SetMandatoryFalse()),
	archiveEntries:          components.NewStringFlag(archiveEntries, "[Optional] This option is no longer supported since version 7.90.5 of Artifactory. If specified, only archive artifacts containing entries matching this pattern are matched. You can use wildcards to specify multiple artifacts.", components.SetMandatoryFalse()),
	downloadSyncDeletes:     components.NewStringFlag(syncDeletes, "[Optional] Specific path in the local file system, under which to sync dependencies after the download. After the download, this path will include only the dependencies downloaded during this download operation. The other files under this path will be deleted.", components.SetMandatoryFalse()),
	skipChecksum:            components.NewBoolFlag(skipChecksum, "[Default: false] Set to true to skip checksum verification when downloading.", components.WithBoolDefaultValueFalse()),

	// Upload specific commands flags
	uploadTargetProps: components.NewStringFlag(targetProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Those properties will be attached to the uploaded artifacts.", components.SetMandatoryFalse()),
	uploadExclusions:  components.NewStringFlag(exclusions, "[Optional] List of semicolon-separated(;) exclude patterns. Exclude patterns may contain the * and the ? wildcards or a regex pattern, according to the value of the 'regexp' option.", components.SetMandatoryFalse()),
	deb:               components.NewStringFlag(deb, "[Optional] Used for Debian packages in the form of distribution/component/architecture. If the value for distribution, component or architecture includes a slash, the slash should be escaped with a back-slash.", components.SetMandatoryFalse()),
	uploadRecursive:   components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to collect artifacts in sub-folders to be uploaded to Artifactory.", components.WithBoolDefaultValueFalse()),
	uploadFlat:        components.NewBoolFlag(flat, "[Default: false] If set to false, files are uploaded according to their file system hierarchy.", components.WithBoolDefaultValueFalse()),
	uploadRegexp:      components.NewBoolFlag(regexpFlag, "[Default: false] Set to true to use a regular expression instead of wildcards expression to collect files to upload.", components.WithBoolDefaultValueFalse()),
	uploadExplode:     components.NewBoolFlag(explode, "[Default: false] Set to true to extract an archive after it is deployed to Artifactory.", components.WithBoolDefaultValueFalse()),
	symlinks:          components.NewBoolFlag(symlinks, "[Default: false] Set to true to preserve symbolic links structure in Artifactory.", components.WithBoolDefaultValueFalse()),
	uploadSyncDeletes: components.NewStringFlag(syncDeletes, "[Optional] Specific path in Artifactory, under which to sync artifacts after the upload. After the upload, this path will include only the artifacts uploaded during this upload operation. The other files under this path will be deleted.", components.SetMandatoryFalse()),
	uploadAnt:         components.NewBoolFlag(antFlag, "[Default: false] Set to true to use an ant pattern instead of wildcards expression to collect files to upload.", components.WithBoolDefaultValueFalse()),
	uploadArchive:     components.NewStringFlag(archive, "[Optional] Set to \"zip\" to pack and deploy the files to Artifactory inside a ZIP archive. Currently, the only packaging format supported is zip.", components.SetMandatoryFalse()),
	uploadMinSplit:    components.NewStringFlag(MinSplit, "[Default: "+strconv.Itoa(UploadMinSplitMb)+"] The minimum file size in MiB required to attempt a multi-part upload. This option, as well as the functionality of multi-part upload, requires Artifactory with S3 or GCP storage.", components.SetMandatoryFalse()),
	uploadSplitCount:  components.NewStringFlag(SplitCount, "[Default: "+strconv.Itoa(UploadSplitCount)+"] The maximum number of parts that can be concurrently uploaded per file during a multi-part upload. Set to 0 to disable multi-part upload. This option, as well as the functionality of multi-part upload, requires Artifactory with S3 or GCP storage.", components.SetMandatoryFalse()),
	chunkSize:         components.NewStringFlag(chunkSize, "[Default: "+strconv.Itoa(UploadChunkSizeMb)+"] The upload chunk size in MiB that can be concurrently uploaded during a multi-part upload. This option, as well as the functionality of multi-part upload, requires Artifactory with S3 or GCP storage.", components.SetMandatoryFalse()),

	// Move specific commands flags
	moveRecursive:    components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to move artifacts inside sub-folders in Artifactory.", components.WithBoolDefaultValueFalse()),
	moveFlat:         components.NewBoolFlag(flat, "[Default: false] If set to false, files are moved according to their file system hierarchy.", components.WithBoolDefaultValueFalse()),
	moveProps:        components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties will be moved.", components.SetMandatoryFalse()),
	moveExcludeProps: components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties will be moved.", components.SetMandatoryFalse()),

	// Copy specific commands flags
	copyRecursive:    components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to copy artifacts inside sub-folders in Artifactory.", components.WithBoolDefaultValueFalse()),
	copyFlat:         components.NewBoolFlag(flat, "[Default: false] If set to false, files are copied according to their file system hierarchy.", components.WithBoolDefaultValueFalse()),
	copyProps:        components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties will be copied.", components.SetMandatoryFalse()),
	copyExcludeProps: components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties will be copied.", components.SetMandatoryFalse()),

	// Delete specific commands flags
	deleteRecursive:    components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to delete artifacts inside sub-folders in Artifactory.", components.WithBoolDefaultValueFalse()),
	deleteQuiet:        components.NewBoolFlag(quiet, "[Default: $CI] Set to true to skip the delete confirmation message.", components.WithBoolDefaultValueFalse()),
	deleteProps:        components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties will be deleted.", components.SetMandatoryFalse()),
	deleteExcludeProps: components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties will be deleted.", components.SetMandatoryFalse()),

	// Search specific commands flags
	searchRecursive:    components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to search artifacts inside sub-folders in Artifactory.", components.WithBoolDefaultValueFalse()),
	count:              components.NewBoolFlag(count, "[Optional] Set to true to display only the total of files or folders found.", components.WithBoolDefaultValueFalse()),
	searchProps:        components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties will be returned.", components.SetMandatoryFalse()),
	searchExcludeProps: components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties will be returned.", components.SetMandatoryFalse()),
	searchTransitive:   components.NewBoolFlag(transitive, "[Default: false] Set to true to look for artifacts also in remote repositories. The search will run on the first five remote repositories within the virtual repository. Available on Artifactory version 7.17.0 or higher.", components.WithBoolDefaultValueFalse()),
	searchInclude:      components.NewStringFlag(searchInclude, "[Optional] List of semicolon-separated(;) fields in the form of \"value1;value2;...\". Only the path and the fields that are specified will be returned. The fields must be part of the 'items' AQL domain. For the full supported items list, check %sjfrog-artifactory-documentation/artifactory-query-language.", components.SetMandatoryFalse()),

	// Properties specific commands flags
	propsRecursive:    components.NewBoolFlag(Recursive, "[Default: true] When false, artifacts inside sub-folders in Artifactory will not be affected.", components.WithBoolDefaultValueFalse()),
	propsProps:        components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts with these properties are affected.", components.SetMandatoryFalse()),
	propsExcludeProps: components.NewStringFlag(excludeProps, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\". Only artifacts without the specified properties are affected.", components.SetMandatoryFalse()),

	// Build Publish and Append specific commands flags
	buildUrl:          components.NewStringFlag(buildUrl, "[Optional] Can be used for setting the CI server build URL in the build-info.", components.SetMandatoryFalse()),
	bpDryRun:          components.NewBoolFlag(dryRun, "[Default: false] Set to true to get a preview of the recorded build info, without publishing it to Artifactory.", components.WithBoolDefaultValueFalse()),
	envInclude:        components.NewStringFlag(envInclude, "[Default: *] List of patterns in the form of \"value1;value2;...\" Only environment variables match those patterns will be included.", components.SetMandatoryFalse()),
	envExclude:        components.NewStringFlag(envExclude, "[Default: *password*;*psw*;*secret*;*key*;*token*;*auth*] List of case insensitive patterns in the form of \"value1;value2;...\". Environment variables match those patterns will be excluded.", components.SetMandatoryFalse()),
	bpDetailedSummary: components.NewBoolFlag(detailedSummary, "[Default: false] Set to true to get a command summary with details about the build info artifact.", components.WithBoolDefaultValueFalse()),
	bpOverwrite:       components.NewBoolFlag(Overwrite, "[Default: false] Overwrites all existing occurrences of build infos with the provided name and number. Build artifacts will not be deleted.", components.WithBoolDefaultValueFalse()),

	// Build Add Dependencies specific commands flags
	badRecursive: components.NewBoolFlag(Recursive, "[Default: true] Set to false if you do not wish to collect artifacts in sub-folders to be added to the build info.", components.WithBoolDefaultValueFalse()),
	badRegexp:    components.NewBoolFlag(regexpFlag, "[Default: false] Set to true to use a regular expression instead of wildcards expression to collect files to be added to the build info."),
	badDryRun:    components.NewBoolFlag(dryRun, "[Default: false] Set to true to only get a summary of the dependencies that will be added to the build info.", components.WithBoolDefaultValueFalse()),
	badFromRt:    components.NewBoolFlag(fromRt, "[Default: false] Set true to search the files in Artifactory, rather than on the local file system. The --regexp option is not supported when --from-rt is set to true.", components.WithBoolDefaultValueFalse()),
	badModule:    components.NewStringFlag(module, "[Optional] Optional module name in the build-info for adding the dependency.", components.SetMandatoryFalse()),

	// Build Add Git specific commands flags
	configFlag: components.NewStringFlag(configFlag, "[Optional] Path to a configuration file.", components.SetMandatoryFalse()),

	// BuildScanLegacy specific commands flags
	fail: components.NewBoolFlag(fail, "[Default: false] Set to true if you'd like the command to return exit code 2 in case of no files are affected.", components.WithBoolDefaultValueFalse()),

	// BuildPromote specific commands flags
	Status:              components.NewStringFlag(Status, "[Optional] Build promotion status.", components.SetMandatoryFalse()),
	comment:             components.NewStringFlag(comment, "[Optional] Build promotion comment.", components.SetMandatoryFalse()),
	sourceRepo:          components.NewStringFlag(sourceRepo, "[Optional] Build promotion source repository.", components.SetMandatoryFalse()),
	includeDependencies: components.NewBoolFlag(includeDependencies, "[Default: false] If true, the build dependencies are also promoted.", components.WithBoolDefaultValueFalse()),
	copyFlag:            components.NewBoolFlag(copyFlag, "[Default: false] If true, the build artifacts and dependencies are copied to the target repository, otherwise they are moved.", components.WithBoolDefaultValueFalse()),
	failFast:            components.NewBoolFlag(failFast, "[Default: true] If true, fail and abort the operation upon receiving an error.", components.WithBoolDefaultValueFalse()),
	bprDryRun:           components.NewBoolFlag(dryRun, "[Default: false] If true, promotion is only simulated. The build is not promoted.", components.WithBoolDefaultValueFalse()),
	bprProps:            components.NewStringFlag(props, "[Optional] List of semicolon-separated(;) properties in the form of \"key1=value1;key2=value2;...\" to be attached to the build artifacts.", components.SetMandatoryFalse()),

	// BuildDiscard specific commands flags
	maxDays:         components.NewStringFlag(maxDays, "[Optional] The maximum number of days to keep builds in Artifactory.", components.SetMandatoryFalse()),
	maxBuilds:       components.NewStringFlag(maxBuilds, "[Optional] The maximum number of builds to store in Artifactory.", components.SetMandatoryFalse()),
	excludeBuilds:   components.NewStringFlag(excludeBuilds, "[Optional] List of comma-separated(,) build numbers in the form of \"value1,value2,...\", that should not be removed from Artifactory.", components.SetMandatoryFalse()),
	deleteArtifacts: components.NewBoolFlag(deleteArtifacts, "[Default: false] If set to true, automatically removes build artifacts stored in Artifactory.", components.WithBoolDefaultValueFalse()),
	bdiAsync:        components.NewBoolFlag(Async, "[Default: false] If set to true, build discard will run asynchronously and will not wait for response.", components.WithBoolDefaultValueFalse()),

	// GitLfsClean specific commands flags
	refs:      components.NewStringFlag(refs, "[Default: refs/remotes/*] List of comma-separated(,) Git references in the form of \"ref1,ref2,...\" which should be preserved.", components.SetMandatoryFalse()),
	glcRepo:   components.NewStringFlag(repo, "[Optional] Local Git LFS repository which should be cleaned. If omitted, this is detected from the Git repository.", components.SetMandatoryFalse()),
	glcDryRun: components.NewBoolFlag(dryRun, "[Default: false] If true, cleanup is only simulated. No files are actually deleted.", components.WithBoolDefaultValueFalse()),
	glcQuiet:  components.NewBoolFlag(quiet, "[Default: $CI] Set to true to skip the delete confirmation message.", components.WithBoolDefaultValueFalse()),

	// Config commands flags
	global:          components.NewBoolFlag(global, "[Default: false] Set to true if you'd like the configuration to be global (for all projects). Specific projects can override the global configuration.", components.WithBoolDefaultValueFalse()),
	serverIdResolve: components.NewStringFlag(serverIdResolve, "[Optional] Artifactory server ID for resolution. The server should be configured using the 'jfrog c add' command.", components.SetMandatoryFalse()),
	repoResolve:     components.NewStringFlag(repoResolve, "[Optional] Repository for dependencies resolution.", components.SetMandatoryFalse()),

	// MvnConfig specific commands flags
	serverIdDeploy:       components.NewStringFlag(serverIdDeploy, "[Optional] Artifactory server ID for deployment. The server should be configured using the 'jfrog c add' command.", components.SetMandatoryFalse()),
	repoResolveReleases:  components.NewStringFlag(repoResolveReleases, "[Optional] Resolution repository for release dependencies.", components.SetMandatoryFalse()),
	repoResolveSnapshots: components.NewStringFlag(repoResolveSnapshots, "[Optional] Resolution repository for snapshot dependencies.", components.SetMandatoryFalse()),
	repoDeployReleases:   components.NewStringFlag(repoDeployReleases, "[Optional] Deployment repository for release artifacts.", components.SetMandatoryFalse()),
	repoDeploySnapshots:  components.NewStringFlag(repoDeploySnapshots, "[Optional] Deployment repository for snapshot artifacts.", components.SetMandatoryFalse()),
	includePatterns:      components.NewStringFlag(includePatterns, "[Optional] Filter deployed artifacts by setting a wildcard pattern that specifies which artifacts to include. You may provide multiple patterns separated by ', '.", components.SetMandatoryFalse()),
	excludePatterns:      components.NewStringFlag(excludePatterns, "[Optional] Filter deployed artifacts by setting a wildcard pattern that specifies which artifacts to exclude. You may provide multiple patterns separated by ', '.", components.SetMandatoryFalse()),
	UseWrapper:           components.NewBoolFlag(UseWrapper, "[Default: false] Set to true if you wish to use the wrapper.", components.WithBoolDefaultValueFalse()),

	// GradleConfig specific commands flags
	repoDeploy:          components.NewStringFlag(repoDeploy, "[Optional] Repository for artifacts deployment.", components.SetMandatoryFalse()),
	usesPlugin:          components.NewBoolFlag(usesPlugin, "[Default: false] Set to true if the Gradle Artifactory Plugin is already applied in the build script.", components.WithBoolDefaultValueFalse()),
	deployMavenDesc:     components.NewBoolFlag(deployMavenDesc, "[Default: true] Set to false if you do not wish to deploy Maven descriptors.", components.WithBoolDefaultValueFalse()),
	deployIvyDesc:       components.NewBoolFlag(deployIvyDesc, "[Default: true] Set to false if you do not wish to deploy Ivy descriptors.", components.WithBoolDefaultValueFalse()),
	ivyDescPattern:      components.NewStringFlag(ivyDescPattern, "[Default: '[organization]/[module]/ivy-[revision].xml' Set the deployed Ivy descriptor pattern.", components.SetMandatoryFalse()),
	ivyArtifactsPattern: components.NewStringFlag(ivyArtifactsPattern, "[Default: '[organization]/[module]/[revision]/[artifact]-[revision](-[classifier]).[ext]' Set the deployed Ivy artifacts pattern.", components.SetMandatoryFalse()),

	// Mvn and Gradle specific commands flags
	deploymentThreads: components.NewStringFlag(threads, "[Default: "+strconv.Itoa(commonCliUtils.Threads)+"] Number of threads for uploading build artifacts.", components.SetMandatoryFalse()),
	xrayScan:          components.NewBoolFlag(xrayScan, "[Default: false] Set if you'd like all files to be scanned by Xray on the local file system prior to the upload, and skip the upload if any of the files are found vulnerable.", components.WithBoolDefaultValueFalse()),
	xrOutput:          components.NewStringFlag(xrOutput, "[Default: table] Defines the output format of the command. Acceptable values are: table, json, simple-json and sarif. Note: the json format doesn't include information about scans that are included as part of the Advanced Security package.", components.SetMandatoryFalse()),

	// Docker specific commands flags
	skipLogin:           components.NewBoolFlag(skipLogin, "[Default: false] Set to true if you'd like the command to skip performing docker login.", components.WithBoolDefaultValueFalse()),
	validateSha:         components.NewBoolFlag(validateSha, "[Default: false] Set to true to enable SHA validation during Docker push.", components.WithBoolDefaultValueFalse()),
	watches:             components.NewStringFlag(watches, "[Optional] A comma-separated(,) list of Xray watches, to determine Xray's violations creation.", components.SetMandatoryFalse()),
	repoPath:            components.NewStringFlag(repoPath, "[Optional] Target repo path, to enable Xray to determine watches accordingly.", components.SetMandatoryFalse()),
	licenses:            components.NewBoolFlag(licenses, "[Default: false] Set to true if you'd like to receive licenses from Xray scanning.", components.WithBoolDefaultValueFalse()),
	ExtendedTable:       components.NewBoolFlag(ExtendedTable, "[Default: false] Set to true if you'd like the table to include extended fields such as 'CVSS' & 'Xray Issue Id'. Ignored if the 'format' provided is not 'table'.", components.WithBoolDefaultValueFalse()),
	BypassArchiveLimits: components.NewBoolFlag(BypassArchiveLimits, "[Default: false] Set to true to bypass the indexer-app archive limits.", components.WithBoolDefaultValueFalse()),
	MinSeverity:         components.NewStringFlag(MinSeverity, "[Optional] Set the minimum severity of issues to display. The following values are accepted: Low, Medium, High or Critical.", components.SetMandatoryFalse()),
	FixableOnly:         components.NewBoolFlag(FixableOnly, "[Optional] Set to true if you wish to display issues that have a fixed version only.", components.WithBoolDefaultValueFalse()),
	vuln:                components.NewBoolFlag(vuln, "[Default: false] Set to true if you'd like to receive an additional view of all vulnerabilities, regardless of the policy configured in Xray. Ignored if the provided 'format' is 'sarif'.", components.WithBoolDefaultValueFalse()),

	// DockerPromote specific commands flags
	targetDockerImage: components.NewStringFlag("target-docker-image", "[Optional] Docker target image name.", components.SetMandatoryFalse()),
	sourceTag:         components.NewStringFlag("source-tag", "[Optional] The tag name to promote.", components.SetMandatoryFalse()),
	targetTag:         components.NewStringFlag("target-tag", "[Optional] The target tag to assign the image after promotion.", components.SetMandatoryFalse()),
	dockerPromoteCopy: components.NewBoolFlag("copy", "[Default: false] If set true, the Docker image is copied to the target repository, otherwise it is moved.", components.WithBoolDefaultValueFalse()),

	allowInsecureConnections: components.NewBoolFlag(allowInsecureConnections, "[Default: false] Set to true if you wish to configure NuGet sources with unsecured connections. This is recommended for testing purposes only.", components.WithBoolDefaultValueFalse()),
	npmDetailedSummary:       components.NewBoolFlag(detailedSummary, "[Default: false] Set to true to include a list of the affected files in the command summary.", components.WithBoolDefaultValueFalse()),
	nugetV2:                  components.NewBoolFlag(nugetV2, "[Default: false] Set to true if you'd like to use the NuGet V2 protocol when restoring packages from Artifactory.", components.WithBoolDefaultValueFalse()),

	// GoPublish specific commands flags
	goPublishExclusions: components.NewStringFlag(exclusions, "[Optional] List of semicolon-separated(;) exclusions. Exclusions can include the * and the ? wildcards.", components.SetMandatoryFalse()),
	noFallback:          components.NewBoolFlag(noFallback, "[Default: false] Set to true to avoid downloading packages from the VCS, if they are missing in Artifactory.", components.WithBoolDefaultValueFalse()),

	// Terraform specific commands flags
	namespace:       components.NewStringFlag(namespace, "[Mandatory] Terraform namespace.", components.SetMandatoryTrue()),
	provider:        components.NewStringFlag(provider, "[Mandatory] Terraform provider.", components.SetMandatoryTrue()),
	Tag:             components.NewStringFlag(Tag, "[Mandatory] Terraform package tag.", components.SetMandatoryTrue()),
	IncludeProjects: components.NewStringFlag(IncludeProjects, "[Optional] List of semicolon-separated(;) JFrog Project keys to include in the transfer. You can use wildcards to specify patterns for the JFrog Project keys.", components.SetMandatoryFalse()),
	ExcludeProjects: components.NewStringFlag(ExcludeProjects, "[Optional] List of semicolon-separated(;) JFrog Projects to exclude from the transfer. You can use wildcards to specify patterns for the project keys.", components.SetMandatoryFalse()),

	// TemplateConsumer specific commands flags
	vars: components.NewStringFlag(vars, "[Optional] List of semicolon-separated(;) variables in the form of \"key1=value1;key2=value2;...\" (wrapped by quotes) to be replaced in the template. In the template, the variables should be used as follows: ${key1}.", components.SetMandatoryFalse()),

	// ArtifactoryAccessTokenCreate specific commands flags
	rtAtcGroups:      components.NewStringFlag(Groups, "[Default: *] A list of comma-separated(,) groups for the access token to be associated with. Specify * to indicate that this is a 'user-scoped token', i.e., the token provides the same access privileges that the current subject has, and is therefore evaluated dynamically. ", components.SetMandatoryFalse()),
	rtAtcGrantAdmin:  components.NewBoolFlag(GrantAdmin, "[Default: false] Set to true to provide admin privileges to the access token. This is only available for administrators.", components.WithBoolDefaultValueFalse()),
	rtAtcExpiry:      components.NewStringFlag(Expiry, "[Default: "+strconv.Itoa(ArtifactoryTokenExpiry)+"] The time in seconds for which the token will be valid. To specify a token that never expires, set to zero. Non-admin may only set a value that is equal to or less than the default 3600.", components.SetMandatoryFalse()),
	rtAtcRefreshable: components.NewBoolFlag(Refreshable, "[Default: false] Set to true if you'd like the token to be refreshable. A refresh token will also be returned in order to be used to generate a new token once it expires.", components.WithBoolDefaultValueFalse()),
	rtAtcAudience:    components.NewStringFlag(Audience, "[Optional] A space-separated list of the other Artifactory instances or services that should accept this token identified by their Artifactory Service IDs, as obtained by the 'jfrog rt curl api/system/service_id' command.", components.SetMandatoryFalse()),

	// UserCreate
	Admin:          components.NewBoolFlag(Admin, "[Default: false] Set to true if you'd like to create an admin user.", components.WithBoolDefaultValueFalse()),
	usersCreateCsv: components.NewStringFlag(csv, "[Mandatory] Path to a CSV file with the users' details. The first row of the file is reserved for the cells' headers. It must include \"username\",\"password\",\"email\"", components.SetMandatoryTrue()),
	UsersGroups:    components.NewStringFlag(UsersGroups, "[Optional] A list of comma-separated(,) groups for the new users to be associated with.` `", components.SetMandatoryFalse()),

	// UsersDelete
	usersDeleteCsv: components.NewStringFlag(csv, "[Optional] Path to a CSV file with the users' details. The first row of the file is reserved for the cells' headers. It must include \"username\"", components.SetMandatoryFalse()),

	// GroupCreate
	Replace: components.NewBoolFlag(Replace, "[Default: false] Set to true if you'd like existing groups to be replaced.", components.WithBoolDefaultValueFalse()),

	distUrl:              components.NewStringFlag(url, "JFrog Distribution URL. (example: https://acme.jfrog.io/distribution)", components.SetMandatoryFalse()),
	targetProps:          components.NewStringFlag(targetProps, "List of semicolon-separated(;) properties, in the form of \"key1=value1;key2=value2;...\" to be added to the artifacts after distribution of the release bundle.", components.SetMandatoryFalse()),
	rbDryRun:             components.NewBoolFlag(dryRun, "Set to true to disable communication with JFrog Distribution.", components.WithBoolDefaultValueFalse()),
	sign:                 components.NewBoolFlag(sign, "If set to true, automatically signs the release bundle version.", components.WithBoolDefaultValueFalse()),
	desc:                 components.NewStringFlag(desc, "Description of the release bundle.", components.SetMandatoryFalse()),
	releaseNotesPath:     components.NewStringFlag(releaseNotesPath, "Path to a file describes the release notes for the release bundle version.", components.SetMandatoryFalse()),
	releaseNotesSyntax:   components.NewStringFlag(releaseNotesSyntax, "The syntax for the release notes. Can be one of 'markdown', 'asciidoc', or 'plain_text.", components.SetMandatoryFalse()),
	rbPassphrase:         components.NewStringFlag(passphrase, "The passphrase for the signing key.", components.SetMandatoryFalse()),
	rbRepo:               components.NewStringFlag(repo, "A repository name at source Artifactory to store release bundle artifacts in. If not provided, Artifactory will use the default one.", components.SetMandatoryFalse()),
	distTarget:           components.NewStringFlag(target, "The target path for distributed artifacts on the edge node.", components.SetMandatoryFalse()),
	rbDetailedSummary:    components.NewBoolFlag(detailedSummary, "Set to true to get a command summary with details about the release bundle artifact.", components.WithBoolDefaultValueFalse()),
	DistRules:            components.NewStringFlag(DistRules, "Path to distribution rules.", components.SetMandatoryFalse()),
	site:                 components.NewStringFlag(site, "Wildcard filter for site name.", components.SetMandatoryFalse()),
	city:                 components.NewStringFlag(city, "Wildcard filter for site city name.", components.SetMandatoryFalse()),
	countryCodes:         components.NewStringFlag(countryCodes, "List of semicolon-separated(;) wildcard filters for site country codes.", components.SetMandatoryFalse()),
	sync:                 components.NewBoolFlag(sync, "Set to true to enable sync distribution (the command execution will end when the distribution process ends).", components.WithBoolDefaultValueFalse()),
	maxWaitMinutes:       components.NewStringFlag(maxWaitMinutes, "Max minutes to wait for sync distribution."),
	deleteFromDist:       components.NewBoolFlag(deleteFromDist, "Set to true to delete release bundle version in JFrog Distribution itself after deletion is complete.", components.WithBoolDefaultValueFalse()),
	CreateRepo:           components.NewBoolFlag(CreateRepo, "Set to true to create the repository on the edge if it does not exist.", components.WithBoolDefaultValueFalse()),
	lcSync:               components.NewBoolFlag(Sync, "Set to false to run asynchronously.", components.WithBoolDefaultValueTrue()),
	lcProject:            components.NewStringFlag(Project, "Project key associated with the Release Bundle version.", components.SetMandatoryFalse()),
	lcBuilds:             components.NewStringFlag(Builds, "Path to a JSON file containing information of the source builds from which to create a release bundle.", components.SetHiddenStrFlag(), components.SetMandatoryFalse()),
	lcReleaseBundles:     components.NewStringFlag(ReleaseBundles, "Path to a JSON file containing information of the source release bundles from which to create a release bundle.", components.SetHiddenStrFlag(), components.SetMandatoryFalse()),
	lcSigningKey:         components.NewStringFlag(SigningKey, "The GPG/RSA key-pair name given in Artifactory. If the key isn't provided, the command creates or uses the default key.", components.SetMandatoryFalse()),
	lcPathMappingPattern: components.NewStringFlag(PathMappingPattern, "Specify along with "+PathMappingTarget+" to distribute artifacts to a different path on the edge node. You can use wildcards to specify multiple artifacts.", components.SetMandatoryFalse()),
	lcPathMappingTarget: components.NewStringFlag(PathMappingTarget, "The target path for distributed artifacts on the edge node. If not specified, the artifacts will have the same path and name on the edge node, as on the source Artifactory server. "+
		"For flexibility in specifying the distribution path, you can include placeholders in the form of {1}, {2} which are replaced by corresponding tokens in the pattern path that are enclosed in parenthesis.` `", components.SetMandatoryFalse()),
	lcDryRun: components.NewBoolFlag(dryRun, "Set to true to only simulate the distribution of the release bundle.", components.WithBoolDefaultValueFalse()),
	lcIncludeRepos: components.NewStringFlag(IncludeRepos, "List of semicolon-separated(;) repositories to include in the promotion. If this property is left undefined, all repositories (except those specifically excluded) are included in the promotion. "+
		"If one or more repositories are specifically included, all other repositories are excluded.` `", components.SetMandatoryFalse()),
	lcExcludeRepos:           components.NewStringFlag(ExcludeRepos, "List of semicolon-separated(;) repositories to exclude from the promotion.` `", components.SetMandatoryFalse()),
	platformUrl:              components.NewStringFlag(url, "JFrog platform URL. (example: https://acme.jfrog.io)` `", components.SetMandatoryFalse()),
	PromotionType:            components.NewStringFlag(PromotionType, "The promotion type. Can be one of 'copy' or 'move'.", components.WithStrDefaultValue("copy")),
	lcTag:                    components.NewStringFlag(Tag, "Tag to put on Release Bundle version.", components.SetMandatoryFalse()),
	lcProperties:             components.NewStringFlag(Properties, "Properties to put on the of Manifest Release Bundle version.", components.SetMandatoryFalse()),
	lcDeleteProperties:       components.NewStringFlag(DeleteProperty, "Properties to be deleted on the of Manifest Release Bundle version.", components.SetMandatoryFalse()),
	SourceTypeReleaseBundles: components.NewStringFlag(SourceTypeReleaseBundles, "List of semicolon-seperated(;) release bundles in the form of 'name=releaseBundleName1, version=version1; name=releaseBundleName2, version=version2' to be included in the new bundle.", components.SetMandatoryFalse()),
	SourceTypeBuilds:         components.NewStringFlag(SourceTypeBuilds, "List of semicolon-separated(;) builds in the form of 'name=buildName1, id=runID1, include-deps=true; name=buildName2, id=runID2' to be included in the new bundle.", components.SetMandatoryFalse()),
}

func GetCommandFlags(cmdKey string) []components.Flag {
	return pluginsCommon.GetCommandFlags(cmdKey, commandFlags, flagsMap)
}
