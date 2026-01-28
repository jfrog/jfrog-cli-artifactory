package commands

import (
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/common/spec"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/lifecycle/services"
	"github.com/stretchr/testify/assert"
)

func TestValidateCreationSources(t *testing.T) {
	testCases := []struct {
		testName                string
		detectedCreationSources []services.SourceType
		errExpected             bool
		errMsg                  string
	}{
		{"missing creation sources", []services.SourceType{}, true, missingCreationSourcesErrMsg},
		{"single creation source", []services.SourceType{services.Aql, services.Artifacts, services.Builds},
			true, multipleCreationSourcesErrMsg + " 'aql, artifacts and builds'"},
		{"single aql err", []services.SourceType{services.Aql, services.Aql}, true, singleAqlErrMsg},
		{"valid aql", []services.SourceType{services.Aql}, false, ""},
		{"valid artifacts", []services.SourceType{services.Artifacts, services.Artifacts}, false, ""},
		{"valid builds", []services.SourceType{services.Builds, services.Builds}, false, ""},
		{"valid release bundles", []services.SourceType{services.ReleaseBundles, services.ReleaseBundles}, false, ""},
		{"invalid source type", []services.SourceType{services.Packages}, true, "creation source 'package' is not supported in current version"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			err := validateCreationSources(testCase.detectedCreationSources, false)
			if testCase.errExpected {
				assert.EqualError(t, err, testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCreationForMultipleSources(t *testing.T) {
	testCases := []struct {
		testName                string
		detectedCreationSources []services.SourceType
		errExpected             bool
		errMsg                  string
	}{
		{"missing creation sources", []services.SourceType{}, true, missingCreationSourcesErrMsg},
		{"multiple creation source", []services.SourceType{services.Aql, services.Artifacts, services.Builds, services.ReleaseBundles},
			false, ""},
		{"multiple creation source with duplicated source types", []services.SourceType{services.Aql, services.Artifacts, services.Builds,
			services.ReleaseBundles, services.ReleaseBundles, services.Packages, services.Packages}, false, ""},
		{"single aql err", []services.SourceType{services.Aql, services.Aql}, true, singleAqlErrMsg},
		{"valid aql", []services.SourceType{services.Aql}, false, ""},
		{"valid artifacts", []services.SourceType{services.Artifacts, services.Artifacts}, false, ""},
		{"valid builds", []services.SourceType{services.Builds, services.Builds}, false, ""},
		{"valid release bundles", []services.SourceType{services.ReleaseBundles, services.ReleaseBundles}, false, ""},
		{"valid packages", []services.SourceType{services.Packages, services.Packages}, false, ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			err := validateCreationSources(testCase.detectedCreationSources, true)
			if testCase.errExpected {
				assert.EqualError(t, err, testCase.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFile(t *testing.T) {
	testCases := []struct {
		testName           string
		file               spec.File
		errExpected        bool
		expectedSourceType services.SourceType
	}{
		{"valid aql", spec.File{Aql: utils.Aql{ItemsFind: "abc"}}, false, services.Aql},
		{"valid build", spec.File{Build: "name/number", IncludeDeps: "true", Project: "project"}, false, services.Builds},
		{"valid bundle", spec.File{Bundle: "name/number", Project: "project"}, false, services.ReleaseBundles},
		{"valid artifacts",
			spec.File{
				Pattern:      "repo/path/file",
				Exclusions:   []string{"exclude"},
				Props:        "prop",
				ExcludeProps: "exclude prop",
				Recursive:    "false"}, false, services.Artifacts},
		{"invalid fields", spec.File{PathMapping: utils.PathMapping{Input: "input"}, Target: "target"}, true, ""},
		{"multiple creation sources in a file",
			spec.File{Aql: utils.Aql{ItemsFind: "abc"}, Build: "name/number", Bundle: "name/number", Pattern: "repo/path/file"},
			true, ""},
		{"invalid aql", spec.File{Aql: utils.Aql{ItemsFind: "abc"}, Props: "prop"}, true, ""},
		{"invalid builds", spec.File{Build: "name/number", Recursive: "false"}, true, ""},
		{"invalid bundles", spec.File{Bundle: "name/number", IncludeDeps: "true"}, true, ""},
		{"invalid artifacts", spec.File{Pattern: "repo/path/file", Project: "proj"}, true, ""},
		{"invalid source", spec.File{Package: "abc", Version: "ver"}, true, ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			sourceType, err := validateFile(testCase.file, false)
			if testCase.errExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedSourceType, sourceType)
			}
		})
	}
}

func TestValidateFileForPackageAndMultipleSourceSupportedVer(t *testing.T) {
	testCases := []struct {
		testName           string
		file               spec.File
		errExpected        bool
		expectedSourceType services.SourceType
	}{
		{"valid aql", spec.File{Aql: utils.Aql{ItemsFind: "abc"}}, false, services.Aql},
		{"valid build", spec.File{Build: "name/number", IncludeDeps: "true", Project: "project"}, false, services.Builds},
		{"valid package", spec.File{Package: "abc", Version: "1.0.0", Type: "type", RepoKey: "repo"}, false, services.Packages},
		{"valid bundle", spec.File{Bundle: "name/number", Project: "project"}, false, services.ReleaseBundles},
		{"valid artifacts",
			spec.File{
				Pattern:      "repo/path/file",
				Exclusions:   []string{"exclude"},
				Props:        "prop",
				ExcludeProps: "exclude prop",
				Recursive:    "false"}, false, services.Artifacts},
		{"invalid fields", spec.File{PathMapping: utils.PathMapping{Input: "input"}, Target: "target"}, true, ""},
		{"multiple creation sources in a file",
			spec.File{Aql: utils.Aql{ItemsFind: "abc"}, Build: "name/number", Bundle: "name/number", Pattern: "repo/path/file"},
			false, "aql"},
		{"invalid aql", spec.File{Aql: utils.Aql{ItemsFind: "abc"}, Props: "prop"}, true, ""},
		{"invalid builds", spec.File{Build: "name/number", Recursive: "false"}, true, ""},
		{"invalid bundles", spec.File{Bundle: "name/number", IncludeDeps: "true"}, true, ""},
		{"invalid artifacts", spec.File{Pattern: "repo/path/file", Project: "proj"}, true, ""},
		{"invalid package", spec.File{Package: "abc", Recursive: "false"}, true, ""},
	}
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			sourceType, err := validateFile(testCase.file, true)
			if testCase.errExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedSourceType, sourceType)
			}
		})
	}
}

func TestUpdateReleaseBundleRepoKeyWithProject(t *testing.T) {
	testCases := []struct {
		testName     string
		sources      []services.RbSource
		expectedRepo string
		shouldUpdate bool
	}{
		{"empty sources", []services.RbSource{}, "", false},
		{"first source is not release_bundles", []services.RbSource{{SourceType: "builds", Builds: []services.BuildSource{{BuildName: "test-build", BuildNumber: "1"}}}}, "", false},
		{"release_bundles source with ProjectKey", []services.RbSource{{SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle1", ReleaseBundleVersion: "1.0.0", ProjectKey: "myproject", RepositoryKey: ""}}}}, "myproject-release-bundles-v2", true},
		{"release_bundles source without ProjectKey", []services.RbSource{{SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle1", ReleaseBundleVersion: "1.0.0", ProjectKey: "", RepositoryKey: "existing-repo"}}}}, "existing-repo", false},
		{"multiple release bundles with ProjectKey", []services.RbSource{{SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle1", ReleaseBundleVersion: "1.0.0", ProjectKey: "project1", RepositoryKey: ""}, {ReleaseBundleName: "bundle2", ReleaseBundleVersion: "2.0.0", ProjectKey: "project2", RepositoryKey: ""}}}}, "project1-release-bundles-v2", true},
		{"multiple sources with first being release_bundles", []services.RbSource{{SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle1", ReleaseBundleVersion: "1.0.0", ProjectKey: "project1", RepositoryKey: ""}}}, {SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle2", ReleaseBundleVersion: "2.0.0", ProjectKey: "project2", RepositoryKey: ""}}}}, "project1-release-bundles-v2", true},
		{"mixed ProjectKey values in same source", []services.RbSource{{SourceType: "release_bundles", ReleaseBundles: []services.ReleaseBundleSource{{ReleaseBundleName: "bundle1", ReleaseBundleVersion: "1.0.0", ProjectKey: "project1", RepositoryKey: ""}, {ReleaseBundleName: "bundle2", ReleaseBundleVersion: "2.0.0", ProjectKey: "", RepositoryKey: "existing-repo"}, {ReleaseBundleName: "bundle3", ReleaseBundleVersion: "3.0.0", ProjectKey: "project3", RepositoryKey: ""}}}}, "project1-release-bundles-v2", true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			// Create a copy of sources to avoid modifying the original test data
			sourcesCopy := make([]services.RbSource, len(testCase.sources))
			for i := range testCase.sources {
				sourcesCopy[i] = testCase.sources[i]
				if len(sourcesCopy[i].ReleaseBundles) > 0 {
					sourcesCopy[i].ReleaseBundles = make([]services.ReleaseBundleSource, len(testCase.sources[i].ReleaseBundles))
					copy(sourcesCopy[i].ReleaseBundles, testCase.sources[i].ReleaseBundles)
				}
			}
			updateReleaseBundleRepoKeyWithProject(sourcesCopy)
			if !testCase.shouldUpdate {
				// Verify no changes were made
				for i := range testCase.sources {
					if len(testCase.sources[i].ReleaseBundles) > 0 {
						for j := range testCase.sources[i].ReleaseBundles {
							assert.Equal(t, testCase.sources[i].ReleaseBundles[j].RepositoryKey, sourcesCopy[i].ReleaseBundles[j].RepositoryKey)
						}
					}
				}
			} else {
				// Verify RepositoryKey was updated for bundles with ProjectKey
				for i := range sourcesCopy {
					if sourcesCopy[i].SourceType == "release_bundles" {
						for j := range sourcesCopy[i].ReleaseBundles {
							rb := sourcesCopy[i].ReleaseBundles[j]
							if rb.ProjectKey != "" {
								expectedRepo := rb.ProjectKey + "-release-bundles-v2"
								assert.Equal(t, expectedRepo, rb.RepositoryKey, "RepositoryKey should be updated for bundle with ProjectKey")
							} else {
								// If no ProjectKey, RepositoryKey should remain unchanged
								originalRb := testCase.sources[i].ReleaseBundles[j]
								assert.Equal(t, originalRb.RepositoryKey, rb.RepositoryKey, "RepositoryKey should remain unchanged when ProjectKey is empty")
							}
						}
					}
				}
			}
		})
	}
}
