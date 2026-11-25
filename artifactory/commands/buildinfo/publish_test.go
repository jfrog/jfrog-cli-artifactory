package buildinfo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	buildinfo "github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/build-info-go/utils/cienv"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils/civcs"
	"github.com/jfrog/jfrog-cli-core/v2/common/build"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"github.com/jfrog/jfrog-client-go/utils/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
	mock.Mock
}

func (m *mockServicesManager) SetProps(params services.PropsParams) (int, error) {
	args := m.Called(params)
	return args.Int(0), args.Error(1)
}

// searchFunc lets a test compute the SearchFiles result per call. A reader is consumed and its file
// deleted on Close, so every call must get a fresh reader. Pass it as the first value to Return().
type searchFunc func(services.SearchParams) (*content.ContentReader, error)

func (m *mockServicesManager) SearchFiles(params services.SearchParams) (*content.ContentReader, error) {
	args := m.Called(params)
	if fn, ok := args.Get(0).(searchFunc); ok {
		return fn(params)
	}
	reader, _ := args.Get(0).(*content.ContentReader)
	return reader, args.Error(1)
}

func createSearchReader(t *testing.T, jsonContent string) (*content.ContentReader, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-search-*.json")
	assert.NoError(t, err)
	_, err = tmpFile.WriteString(jsonContent)
	assert.NoError(t, err)
	assert.NoError(t, tmpFile.Close())
	filePath := tmpFile.Name()
	reader := content.NewContentReader(filePath, content.DefaultKey)
	return reader, func() {
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			assert.NoError(t, err)
		}
	}
}

func createTestSearchReader(t *testing.T) (*content.ContentReader, func()) {
	return createSearchReader(t, `{"results":[{"repo":"libs-release","path":"com/example","name":"file.jar","type":"file","size":0,"created":"","modified":""}]}`)
}

func createEmptySearchReader(t *testing.T) (*content.ContentReader, func()) {
	return createSearchReader(t, `{"results":[]}`)
}

func setUpCIGitHubEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CI", "true")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_WORKFLOW", "test")
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_REPOSITORY_OWNER", "jfrog")
	t.Setenv("GITHUB_REPOSITORY", "jfrog/jfrog-cli")
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GITHUB_SHA", "")
	t.Setenv("GITHUB_REF", "")
	t.Setenv("GITHUB_REF_NAME", "")
	t.Setenv("GITHUB_HEAD_REF", "")
}

// newReaderWithItems returns a search reader holding one item per given name.
// With no names the reader is empty.
func newReaderWithItems(t *testing.T, names ...string) *content.ContentReader {
	t.Helper()
	items := make([]string, len(names))
	for i, name := range names {
		items[i] = fmt.Sprintf(`{"repo":"libs-release","path":"com/example","name":%q,"type":"file","size":0,"created":"","modified":""}`, name)
	}
	reader, cleanup := createSearchReader(t, `{"results":[`+strings.Join(items, ",")+`]}`)
	t.Cleanup(cleanup)
	return reader
}

// exactSearchHits answers an exact-path SearchFiles call with one item when the pattern is one of
// existing, and with an empty result otherwise.
func exactSearchHits(t *testing.T, existing ...string) searchFunc {
	return func(params services.SearchParams) (*content.ContentReader, error) {
		for _, pattern := range existing {
			if params.Pattern == pattern {
				return newReaderWithItems(t, pattern), nil
			}
		}
		return newReaderWithItems(t), nil
	}
}

// matchExactSearch matches the SearchFiles calls made for exact artifact paths (not build-scoped).
func matchExactSearch() interface{} {
	return mock.MatchedBy(func(params services.SearchParams) bool {
		return params.Build == "" && params.Pattern != ""
	})
}

// captureLog routes the logger output at WARN level and above to a buffer for the test duration.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := new(bytes.Buffer)
	previous := log.Logger
	log.SetLogger(log.NewLogger(log.WARN, buf))
	t.Cleanup(func() { log.SetLogger(previous) })
	return buf
}

// warnLines returns the captured log lines that were logged at WARN level.
func warnLines(buf *bytes.Buffer) []string {
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "[Warn]") {
			lines = append(lines, line)
		}
	}
	return lines
}

// runSetVcsProps runs setVcsPropsOnArtifacts for the build info in a CI environment.
func runSetVcsProps(t *testing.T, sm artifactory.ArtifactoryServicesManager, bi *buildinfo.BuildInfo) {
	t.Helper()
	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(t.TempDir())
	bpc.buildConfiguration = build.NewBuildConfiguration(bi.Name, bi.Number, "", "")
	bpc.setVcsPropsOnArtifacts(sm, bi)
}

const (
	jarPath      = "libs-release/com/example/app/1.0/app-1.0.jar"
	pomPath      = "libs-release/com/example/app/1.0/app-1.0.pom"
	snapshotRepo = "libs-snapshot-local"
)

func releaseBuildInfo() *buildinfo.BuildInfo {
	return &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{{Artifacts: []buildinfo.Artifact{
			{Name: "app-1.0.jar", Path: "com/example/app/1.0/app-1.0.jar", OriginalDeploymentRepo: "libs-release"},
			{Name: "app-1.0.pom", Path: "com/example/app/1.0/app-1.0.pom", OriginalDeploymentRepo: "libs-release"},
		}}},
	}
}

// snapshotBuildInfo lists the -SNAPSHOT names the Maven extractor records, although Artifactory
// stores the files under a timestamped name.
func snapshotBuildInfo() *buildinfo.BuildInfo {
	return &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{{Artifacts: []buildinfo.Artifact{
			{Name: "app-1.0-SNAPSHOT.jar", Path: "com/example/app/1.0-SNAPSHOT/app-1.0-SNAPSHOT.jar", OriginalDeploymentRepo: snapshotRepo},
			{Name: "app-1.0-SNAPSHOT.pom", Path: "com/example/app/1.0-SNAPSHOT/app-1.0-SNAPSHOT.pom", OriginalDeploymentRepo: snapshotRepo},
		}}},
	}
}

// TestSetVcsPropsOnArtifacts_Snapshot_FallsBackToBuildLookup: the exact-path search finds nothing
// because Artifactory stores a timestamped file; the artifacts are tagged through the build lookup.
func TestSetVcsPropsOnArtifacts_Snapshot_FallsBackToBuildLookup(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	mockSM := new(mockServicesManager)
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t), nil)
	buildReader := newReaderWithItems(t, "app-1.0-20260101.120000-1.jar", "app-1.0-20260101.120000-1.pom")
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: buildReader}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Reader == buildReader
	})).Return(2, nil)

	runSetVcsProps(t, mockSM, snapshotBuildInfo())

	mockSM.AssertExpectations(t)
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	assert.Equal(t, 1, *buildApiCalls, "the build-artifacts lookup is the fallback for the exact-path misses")
	assert.Empty(t, warnLines(buf))
}

// TestSetVcsPropsOnArtifacts_Release_ExactSearchOnly: the exact-path search finds every artifact,
// so the build lookup is never used and SetProps runs once.
func TestSetVcsPropsOnArtifacts_Release_ExactSearchOnly(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	mockSM := new(mockServicesManager)
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t, jarPath, pomPath), nil)
	mockSM.On("SetProps", mock.Anything).Return(2, nil)
	buildApiCalls := stubBuildArtifacts(t, nil)

	runSetVcsProps(t, mockSM, releaseBuildInfo())

	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	assert.Equal(t, 0, *buildApiCalls, "the build lookup must not run when the exact search found everything")
	assert.Empty(t, warnLines(buf))
	assert.Empty(t, buf.String())
}

// TestSetVcsPropsOnArtifacts_SetProps403_WarnsOnce: a missing Annotate permission (403) tags nothing.
// The artifacts were found, so there is no fallback, and exactly one warning is logged.
func TestSetVcsPropsOnArtifacts_SetProps403_WarnsOnce(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	mockSM := new(mockServicesManager)
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t, jarPath, pomPath), nil)
	mockSM.On("SetProps", mock.Anything).Return(0, errors.New("server response: 403 Forbidden"))
	buildApiCalls := stubBuildArtifacts(t, nil)

	assert.NotPanics(t, func() { runSetVcsProps(t, mockSM, releaseBuildInfo()) })

	assert.Equal(t, 0, *buildApiCalls, "a SetProps failure alone must not trigger the build lookup")
	warnings := warnLines(buf)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "2 of 2")
	assert.Contains(t, warnings[0], "Annotate")
	assert.Contains(t, warnings[0], "403 Forbidden")
}

// TestSetVcsPropsOnArtifacts_ShortSearchAndShortBuildLookup_Warns: neither the exact search nor the
// build lookup returns everything the build info lists; the warning carries the untagged count.
func TestSetVcsPropsOnArtifacts_ShortSearchAndShortBuildLookup_Warns(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	bi := releaseBuildInfo()
	bi.Modules[0].Artifacts = append(bi.Modules[0].Artifacts,
		buildinfo.Artifact{Name: "app-1.0.sources.jar", Path: "com/example/app/1.0/app-1.0.sources.jar", OriginalDeploymentRepo: "libs-release"})

	mockSM := new(mockServicesManager)
	// The exact search finds 2 of the 3 artifacts.
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t, jarPath, pomPath), nil)
	// The build lookup also returns only 2 of the 3.
	buildReader := newReaderWithItems(t, "app-1.0.jar", "app-1.0.pom")
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: buildReader}})
	mockSM.On("SetProps", mock.Anything).Return(2, nil)

	runSetVcsProps(t, mockSM, bi)

	assert.Equal(t, 1, *buildApiCalls)
	warnings := warnLines(buf)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "1 of 3")
	assert.Contains(t, warnings[0], "Annotate")
}

// TestSetVcsPropsOnArtifacts_LookupFails_Warns: every lookup keeps failing, so nothing is tagged.
func TestSetVcsPropsOnArtifacts_LookupFails_Warns(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	mockSM := new(mockServicesManager)
	mockSM.On("SearchFiles", matchExactSearch()).Return(nil, errors.New("search backend down"))
	var failures []buildArtifactsResult
	for i := 0; i < maxRetries; i++ {
		failures = append(failures, buildArtifactsResult{err: errors.New("build api down")})
	}
	buildApiCalls := stubBuildArtifacts(t, failures)

	bi := releaseBuildInfo()
	runSetVcsProps(t, mockSM, bi)

	mockSM.AssertNotCalled(t, "SetProps")
	assert.Equal(t, maxRetries, *buildApiCalls, "a failed exact lookup falls back to the build lookup")
	warnings := warnLines(buf)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "2 of 2")
	assert.Contains(t, warnings[0], "build api down")
}

// TestSetVcsPropsOnArtifacts_PresentAndMissing_BuildLookupOnce: the build lookup serves the missing
// artifacts and also the short exact search, but it runs only once per publish.
func TestSetVcsPropsOnArtifacts_PresentAndMissing_BuildLookupOnce(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	bi := snapshotBuildInfo()
	bi.Modules[0].Artifacts = append(bi.Modules[0].Artifacts, buildinfo.Artifact{Name: "app-1.0-SNAPSHOT.module", Path: "com/example/app/1.0-SNAPSHOT/app-1.0-SNAPSHOT.module"})

	mockSM := new(mockServicesManager)
	// The exact search finds nothing: Artifactory stores timestamped files.
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t), nil)
	buildReader := newReaderWithItems(t, "app-1.jar", "app-1.pom", "app-1.module")
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: buildReader}})
	mockSM.On("SetProps", mock.Anything).Return(3, nil)

	runSetVcsProps(t, mockSM, bi)

	assert.Equal(t, 1, *buildApiCalls, "the build lookup runs once, never twice")
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	assert.Empty(t, warnLines(buf))
}

// TestSetVcsPropsOnArtifacts_SharedArtifactWithoutRepo_NoWarning: the same artifact without
// OriginalDeploymentRepo listed in two modules is counted once, so one tagged item is enough.
func TestSetVcsPropsOnArtifacts_SharedArtifactWithoutRepo_NoWarning(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	shared := buildinfo.Artifact{Name: "app-1.0.jar", Path: "com/example/app/1.0/app-1.0.jar"}
	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{shared}},
			{Artifacts: []buildinfo.Artifact{shared}},
		},
	}

	mockSM := new(mockServicesManager)
	buildReader := newReaderWithItems(t, "app-1.0.jar")
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: buildReader}})
	mockSM.On("SetProps", mock.Anything).Return(1, nil)

	runSetVcsProps(t, mockSM, bi)

	assert.Equal(t, 1, *buildApiCalls)
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	assert.Empty(t, buf.String(), "an artifact listed in two modules must not produce a false warning")
}

// TestSetVcsPropsOnArtifacts_AllTagged_NoWarning: the happy path logs nothing at warn level or
// above, and an artifact listed in several modules is searched and counted once.
func TestSetVcsPropsOnArtifacts_AllTagged_NoWarning(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)

	bi := releaseBuildInfo()
	bi.Modules = append(bi.Modules, buildinfo.Module{Artifacts: []buildinfo.Artifact{bi.Modules[0].Artifacts[0]}})

	mockSM := new(mockServicesManager)
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t, jarPath, pomPath), nil)
	mockSM.On("SetProps", mock.Anything).Return(2, nil)
	buildApiCalls := stubBuildArtifacts(t, nil)

	runSetVcsProps(t, mockSM, bi)

	mockSM.AssertNumberOfCalls(t, "SearchFiles", 2)
	mockSM.AssertNumberOfCalls(t, "SetProps", 1)
	assert.Equal(t, 0, *buildApiCalls)
	assert.Empty(t, buf.String(), "no log lines expected when every artifact is tagged")
}

// TestSetVcsPropsOnArtifacts_AllPresent_DirectOnly: all artifacts have OriginalDeploymentRepo.
// Only the direct-path branch fires; build-search is never invoked.
func TestSetVcsPropsOnArtifacts_AllPresent_DirectOnly(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	// Only pattern-based search calls (direct path) should appear; each artifact is found.
	mockSM.On("SearchFiles", matchExactSearch()).Return(
		exactSearchHits(t, "libs-release/com/example/file.jar", "libs-release/com/example/file2.jar"), nil)
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(2, nil)

	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
					{Name: "file2.jar", Path: "com/example/file2.jar", OriginalDeploymentRepo: "libs-release"},
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	// Build-scoped SearchFiles must NOT have been called.
	for _, call := range mockSM.Calls {
		if call.Method == "SearchFiles" {
			params, ok := call.Arguments.Get(0).(services.SearchParams)
			require.True(t, ok, "unexpected type for SearchFiles argument")
			assert.Empty(t, params.Build, "build-search branch must not fire when all artifacts have OriginalDeploymentRepo")
		}
	}
}

// TestSetVcsPropsOnArtifacts_AllMissing_BuildSearchOnly: no artifact has OriginalDeploymentRepo.
// Only the build-search branch fires; no repo-listing, no direct pattern-search.
// PRIMARY regression guard for RTECO-2025.
func TestSetVcsPropsOnArtifacts_AllMissing_BuildSearchOnly(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	searchReader := newReaderWithItems(t, "minimal-example-1.0.jar", "minimal-example-1.0.pom")
	// Only the build-artifacts API should be used; no AQL search.
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: searchReader}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps
	})).Return(2, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					// No OriginalDeploymentRepo on either artifact (Gradle extractor scenario).
					{Name: "minimal-example-1.0.jar", Path: "com/example/minimal-example/1.0/minimal-example-1.0.jar"},
					{Name: "minimal-example-1.0.pom", Path: "com/example/minimal-example/1.0/minimal-example-1.0.pom"},
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, 1, *buildApiCalls, "exactly one build-artifacts API call expected regardless of artifact count")
	mockSM.AssertNotCalled(t, "SearchFiles")
}

// TestSetVcsPropsOnArtifacts_Mixed_BothPaths: some artifacts have OriginalDeploymentRepo, some don't.
// Both branches fire: direct for present, build-search for missing.
func TestSetVcsPropsOnArtifacts_Mixed_BothPaths(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	buf := captureLog(t)
	nonGitDir := t.TempDir()
	expectedProps := civcs.GetCIVcsPropsString(nonGitDir)

	mockSM := new(mockServicesManager)
	// Direct-path call: pattern non-empty, Build empty.
	mockSM.On("SearchFiles", matchExactSearch()).Return(exactSearchHits(t, "libs-release/com/example/file.jar"), nil)
	// Build-search resolves through the build-artifacts API, not AQL, and lists the whole build.
	buildReader := newReaderWithItems(t, "file.jar", "missing.jar")
	buildApiCalls := stubBuildArtifacts(t, []buildArtifactsResult{{reader: buildReader}})
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps && params.Reader != buildReader
	})).Return(1, nil)
	mockSM.On("SetProps", mock.MatchedBy(func(params services.PropsParams) bool {
		return params.Props == expectedProps && params.Reader == buildReader
	})).Return(2, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{
				Artifacts: []buildinfo.Artifact{
					{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
					{Name: "missing.jar", Path: "com/example/missing.jar"}, // no repo
				},
			},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, 1, *buildApiCalls, "missing-repo artifacts resolve through the build-artifacts API")
	assert.Empty(t, warnLines(buf))
}

// TestSetVcsPropsOnArtifacts_Disabled: JFROG_CLI_CI_VCS_PROPS_DISABLED=true short-circuits everything.
func TestSetVcsPropsOnArtifacts_Disabled(t *testing.T) {
	t.Setenv("JFROG_CLI_CI_VCS_PROPS_DISABLED", "true")

	buf := captureLog(t)
	buildApiCalls := stubBuildArtifacts(t, nil)
	mockSM := new(mockServicesManager)
	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
			}},
		},
	}
	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(t.TempDir())
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertNotCalled(t, "SearchFiles")
	mockSM.AssertNotCalled(t, "SetProps")
	assert.Equal(t, 0, *buildApiCalls)
	assert.Empty(t, buf.String(), "a disabled or empty-props run must stay silent")
}

// TestSetVcsPropsOnArtifacts_EmptyProps: non-CI non-git dir → empty props → early return.
func TestSetVcsPropsOnArtifacts_EmptyProps(t *testing.T) {
	// Unset all CI env vars so no CI props are collected.
	for _, v := range []string{
		"CI", "GITHUB_ACTIONS", "GITHUB_WORKFLOW", "GITHUB_RUN_ID",
		"GITHUB_REPOSITORY_OWNER", "GITHUB_REPOSITORY",
		"GITLAB_CI", "JENKINS_URL", "CIRCLECI", "TRAVIS",
	} {
		t.Setenv(v, "")
	}

	buf := captureLog(t)
	buildApiCalls := stubBuildArtifacts(t, nil)
	mockSM := new(mockServicesManager)
	bi := &buildinfo.BuildInfo{
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar", OriginalDeploymentRepo: "libs-release"},
			}},
		},
	}
	// Use a temp dir with no .git — GetCIVcsPropsString will return "" when no CI env and no git.
	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(t.TempDir())
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertNotCalled(t, "SearchFiles")
	mockSM.AssertNotCalled(t, "SetProps")
	assert.Equal(t, 0, *buildApiCalls)
	assert.Empty(t, buf.String(), "a disabled or empty-props run must stay silent")
}

// TestSetVcsPropsOnArtifacts_ProjectPropagated: project key flows into the build spec.
func TestSetVcsPropsOnArtifacts_ProjectPropagated(t *testing.T) {
	setUpCIGitHubEnv(t)
	zeroRetryDelay(t)
	nonGitDir := t.TempDir()

	mockSM := new(mockServicesManager)
	searchReader, cleanup := createTestSearchReader(t)
	defer cleanup()

	// Capture what the build-artifacts API is asked for.
	var gotName, gotNumber, gotProject string
	originalResolver := resolveBuildArtifacts
	resolveBuildArtifacts = func(_ artifactory.ArtifactoryServicesManager, buildName, buildNumber, project string) (*content.ContentReader, error) {
		gotName, gotNumber, gotProject = buildName, buildNumber, project
		return searchReader, nil
	}
	t.Cleanup(func() { resolveBuildArtifacts = originalResolver })
	mockSM.On("SetProps", mock.Anything).Return(1, nil)

	bi := &buildinfo.BuildInfo{
		Name:   "mybuild",
		Number: "42",
		Modules: []buildinfo.Module{
			{Artifacts: []buildinfo.Artifact{
				{Name: "file.jar", Path: "com/example/file.jar"}, // no repo → build-search branch
			}},
		},
	}

	bpc := NewBuildPublishCommand()
	bpc.SetDotGitPath(nonGitDir)
	bpc.buildConfiguration = build.NewBuildConfiguration("mybuild", "42", "", "myproject")
	bpc.setVcsPropsOnArtifacts(mockSM, bi)

	mockSM.AssertExpectations(t)
	assert.Equal(t, "mybuild", gotName)
	assert.Equal(t, "42", gotNumber)
	assert.Equal(t, "myproject", gotProject, "project key must reach the build-artifacts API")
}

func TestPrintBuildInfoLink(t *testing.T) {
	timeNow := time.Now()
	buildTime := strconv.FormatInt(timeNow.UnixNano()/1000000, 10)
	var linkTypes = []struct {
		majorVersion  int
		buildTime     time.Time
		buildInfoConf *build.BuildConfiguration
		serverDetails config.ServerDetails
		expected      string
	}{
		// Test platform URL
		{5, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8081/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{6, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8081/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", ""),
			config.ServerDetails{Url: "http://localhost:8082/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=artifactory-build-info"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{Url: "http://localhost:8082/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=cli-build-info&projectKey=cli"},

		// Test Artifactory URL
		{5, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8081/artifactory"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{6, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8081/artifactory/"}, "http://localhost:8081/artifactory/webapp/#/builds/test/1"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", ""),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8082/artifactory"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=artifactory-build-info"},
		{7, timeNow, build.NewBuildConfiguration("test", "1", "6", "cli"),
			config.ServerDetails{ArtifactoryUrl: "http://localhost:8082/artifactory/"}, "http://localhost:8082/ui/builds/test/1/" + buildTime + "/published?buildRepo=cli-build-info&projectKey=cli"},
	}

	for i := range linkTypes {
		buildPubConf := &BuildPublishCommand{
			linkTypes[i].buildInfoConf,
			&linkTypes[i].serverDetails,
			nil,
			true,
			nil,
			false,
			false,
			nil,
			"",
			false,
			false,
			BuildAddGitCommand{},
		}
		buildPubComService, err := buildPubConf.getBuildInfoUiUrl(linkTypes[i].majorVersion, linkTypes[i].buildTime)
		assert.NoError(t, err)
		assert.Equal(t, buildPubComService, linkTypes[i].expected)
	}
}

func TestCalculateBuildNumberFrequency(t *testing.T) {
	tests := []struct {
		name     string
		runs     *buildinfo.BuildRuns
		expected map[string]int
	}{
		{
			name: "Single build number",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{{Uri: "/1"}},
			},
			expected: map[string]int{"1": 1},
		},
		{
			name: "Single build number with special characters",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{{Uri: "/1-"}},
			},
			expected: map[string]int{"1-": 1},
		},
		{
			name: "Multiple build numbers",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{
					{Uri: "/1"},
					{Uri: "/2"},
					{Uri: "/1"},
				},
			},
			expected: map[string]int{"1": 2, "2": 1},
		},
		{
			name: "No build numbers",
			runs: &buildinfo.BuildRuns{
				BuildsNumbers: []buildinfo.BuildRun{},
			},
			expected: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateBuildNumberFrequency(tt.runs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIs404Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "404 in message", err: errors.New("server returned 404"), expected: true},
		{name: "not found", err: errors.New("artifact not found"), expected: true},
		{name: "Not Found uppercase", err: errors.New("Not Found"), expected: true},
		{name: "500 error", err: errors.New("server returned 500"), expected: false},
		{name: "connection refused", err: errors.New("connection refused"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is404Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIs403Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "403 in message", err: errors.New("server returned 403"), expected: true},
		{name: "forbidden", err: errors.New("access forbidden"), expected: true},
		{name: "Forbidden uppercase", err: errors.New("Forbidden"), expected: true},
		{name: "500 error", err: errors.New("server returned 500"), expected: false},
		{name: "404 error", err: errors.New("not found 404"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is403Error(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCIVcsPropsString(t *testing.T) {
	tests := []struct {
		name     string
		info     cienv.CIVcsInfo
		expected string
	}{
		{
			name:     "all fields",
			info:     cienv.CIVcsInfo{Provider: "github", Org: "jfrog", Repo: "jfrog-cli"},
			expected: "vcs.provider=github;vcs.org=jfrog;vcs.repo=jfrog-cli",
		},
		{
			name:     "partial fields - provider and org",
			info:     cienv.CIVcsInfo{Provider: "github", Org: "jfrog"},
			expected: "vcs.provider=github;vcs.org=jfrog",
		},
		{
			name:     "only provider",
			info:     cienv.CIVcsInfo{Provider: "github"},
			expected: "vcs.provider=github",
		},
		{
			name:     "empty",
			info:     cienv.CIVcsInfo{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := civcs.BuildCIVcsPropsString(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExcludeDependenciesByScope(t *testing.T) {
	tests := []struct {
		name             string
		depExcludeScopes []string
		modules          []buildinfo.Module
		expectedDeps     [][]string
	}{
		{
			name:             "no scopes specified - all deps retained",
			depExcludeScopes: nil,
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep2"}},
		},
		{
			name:             "single scope exclusion",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
					{Id: "dep3", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep3"}},
		},
		{
			name:             "multiple scope exclusion",
			depExcludeScopes: []string{"test", "provided"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile"}},
					{Id: "dep2", Scopes: []string{"test"}},
					{Id: "dep3", Scopes: []string{"provided"}},
					{Id: "dep4", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep4"}},
		},
		{
			name:             "case insensitive matching",
			depExcludeScopes: []string{"Test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"test"}},
					{Id: "dep2", Scopes: []string{"TEST"}},
					{Id: "dep3", Scopes: []string{"Test"}},
					{Id: "dep4", Scopes: []string{"compile"}},
				}},
			},
			expectedDeps: [][]string{{"dep4"}},
		},
		{
			name:             "dependency with multiple scopes - excluded if any match",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"compile", "test"}},
					{Id: "dep2", Scopes: []string{"compile", "runtime"}},
				}},
			},
			expectedDeps: [][]string{{"dep2"}},
		},
		{
			name:             "dependencies with no scopes - never excluded",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: nil},
					{Id: "dep2", Scopes: []string{}},
					{Id: "dep3", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{"dep1", "dep2"}},
		},
		{
			name:             "multiple modules",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "mod1-dep1", Scopes: []string{"compile"}},
					{Id: "mod1-dep2", Scopes: []string{"test"}},
				}},
				{Dependencies: []buildinfo.Dependency{
					{Id: "mod2-dep1", Scopes: []string{"test"}},
					{Id: "mod2-dep2", Scopes: []string{"runtime"}},
				}},
			},
			expectedDeps: [][]string{{"mod1-dep1"}, {"mod2-dep2"}},
		},
		{
			name:             "all dependencies excluded",
			depExcludeScopes: []string{"test"},
			modules: []buildinfo.Module{
				{Dependencies: []buildinfo.Dependency{
					{Id: "dep1", Scopes: []string{"test"}},
					{Id: "dep2", Scopes: []string{"test"}},
				}},
			},
			expectedDeps: [][]string{{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bi := &buildinfo.BuildInfo{Modules: tt.modules}
			bpc := NewBuildPublishCommand().SetDepExcludeScopes(tt.depExcludeScopes)
			bpc.excludeDependenciesByScope(bi)

			require.Equal(t, len(tt.expectedDeps), len(bi.Modules))
			for i, expectedIds := range tt.expectedDeps {
				actualIds := make([]string, len(bi.Modules[i].Dependencies))
				for j, dep := range bi.Modules[i].Dependencies {
					actualIds[j] = dep.Id
				}
				require.Equal(t, expectedIds, actualIds)
			}
		})
	}
}

func TestHasScopeMatch(t *testing.T) {
	excludeSet := map[string]struct{}{"test": {}, "provided": {}}

	tests := []struct {
		name     string
		scopes   []string
		expected bool
	}{
		{name: "matching scope", scopes: []string{"test"}, expected: true},
		{name: "no matching scope", scopes: []string{"compile"}, expected: false},
		{name: "case insensitive match", scopes: []string{"TEST"}, expected: true},
		{name: "nil scopes", scopes: nil, expected: false},
		{name: "empty scopes", scopes: []string{}, expected: false},
		{name: "multiple scopes with match", scopes: []string{"compile", "provided"}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, matchesExcludeScope(tt.scopes, excludeSet))
		})
	}
}
