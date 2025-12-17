package flexpack

const (
	gradleEnvPrefixLen = 19

	// File Names
	gradlePropertiesFileName = "gradle.properties"

	// Directories
	initDDirName   = "init.d"
	projectDirProp = "projectDir"
	rootDirProp    = "rootDir"

	// Environment Variables
	envGradleOpts    = "GRADLE_OPTS"
	envJavaOpts      = "JAVA_OPTS"
	envProjectPrefix = "ORG_GRADLE_PROJECT_"

	// Keywords
	gradleTaskPublish = "publish"
	keywordSnapshot   = "snapshot"
	keywordRelease    = "release"
	keywordRepo       = "repo"
	keywordUrl        = "url"
	keywordDeploy     = "deploy"
	keywordMaven      = "maven"
	keywordGradle     = "gradle"
	keywordIvy        = "ivy"
	keywordApi        = "api"

	// Script Blocks/Keywords
	blockRepositories     = "repositories"
	blockPublishing       = "publishing"
	blockUploadArchives   = "uploadArchives"
	blockDepResManagement = "dependencyResolutionManagement"
	blockExt              = "ext"
)
