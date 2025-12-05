package flexpack

import "time"

const (
	gradlePropertiesTimeout       = 1 * time.Minute
	artifactSearchClockSkewBuffer = 1 * time.Minute
	gradleEnvPrefixLen            = 19

	// File Names
	buildGradleFileName       = "build.gradle"
	buildGradleKtsFileName    = "build.gradle.kts"
	settingsGradleFileName    = "settings.gradle"
	settingsGradleKtsFileName = "settings.gradle.kts"
	initGradleFileName        = "init.gradle"
	initGradleKtsFileName     = "init.gradle.kts"
	gradlePropertiesFileName  = "gradle.properties"

	// Directories
	initDDirName   = "init.d"
	dotGradleDir   = ".gradle"
	projectDirProp = "projectDir"
	rootDirProp    = "rootDir"

	// Environment Variables
	envGradleUserHome = "GRADLE_USER_HOME"
	envGradleOpts     = "GRADLE_OPTS"
	envJavaOpts       = "JAVA_OPTS"
	envProjectPrefix  = "ORG_GRADLE_PROJECT_"

	// Keywords
	gradleTaskPublish             = "publish"
	gradleTaskPublishToMavenLocal = "publishToMavenLocal"
	keywordSnapshot               = "snapshot"
	keywordRelease                = "release"
	keywordRepo                   = "repo"
	keywordUrl                    = "url"
	keywordDeploy                 = "deploy"
	keywordMaven                  = "maven"
	keywordGradle                 = "gradle"
	keywordIvy                    = "ivy"
	keywordApi                    = "api"

	// Script Blocks/Keywords
	blockRepositories     = "repositories"
	blockPublishing       = "publishing"
	blockUploadArchives   = "uploadArchives"
	blockDepResManagement = "dependencyResolutionManagement"
	blockExt              = "ext"
	keywordArtifactory    = "artifactory"
)

