package flexpack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWasPublishCommand(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []string
		expected bool
	}{
		{"publish", []string{"publish"}, true},
		{"clean publish", []string{"clean", "publish"}, true},
		{"publishToMavenLocal", []string{"publishToMavenLocal"}, false},
		{"publishToSomethingElse", []string{"publishToSomethingElse"}, true},
		{"project:publish", []string{":project:publish"}, true},
		{"subproject:publish", []string{":sub:project:publish"}, true},
		{"clean", []string{"clean"}, false},
		{"build", []string{"build"}, false},
		{"empty", []string{}, false},
		{"publishToMavenLocal and publish", []string{"publishToMavenLocal", "publish"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, wasPublishCommand(tt.tasks))
		})
	}
}

func TestWasPublishCommandExtended(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []string
		expected bool
	}{
		// Publication-specific tasks with "To" pattern are now detected
		{"publishAllPublicationsToMavenRepository", []string{"publishAllPublicationsToMavenRepository"}, true},
		{"publishMavenPublicationToArtifactoryRepository", []string{"publishMavenPublicationToArtifactoryRepository"}, true},
		{"publishToSonatype", []string{"publishToSonatype"}, true},
		{"assemble then publish", []string{"assemble", "check", "publish"}, true},
		{"deeply nested project publish", []string{":a:b:c:d:publish"}, true},
		// Local publishing tasks should still be excluded
		{"publishMavenPublicationToMavenLocal", []string{"publishMavenPublicationToMavenLocal"}, false},
		{"publishAllPublicationsToLocal", []string{"publishAllPublicationsToLocal"}, false},
		{"only colon prefix", []string{":publish"}, true},
		{"case sensitive - Publish", []string{"Publish"}, false},
		{"partial match - publisher", []string{"publisher"}, false},
		{"task with publish suffix", []string{"doNotPublish"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, wasPublishCommand(tt.tasks))
		})
	}
}

func TestWasPublishCommandEdgeCases(t *testing.T) {
	t.Run("Empty task name in list", func(t *testing.T) {
		result := wasPublishCommand([]string{""})
		assert.False(t, result)
	})

	t.Run("Multiple colons with publish", func(t *testing.T) {
		result := wasPublishCommand([]string{"::publish"})
		assert.True(t, result)
	})

	t.Run("Publish in middle of list", func(t *testing.T) {
		result := wasPublishCommand([]string{"clean", "build", "publish", "check"})
		assert.True(t, result)
	})

	t.Run("Many tasks none publish", func(t *testing.T) {
		result := wasPublishCommand([]string{"clean", "build", "test", "check", "assemble", "jar"})
		assert.False(t, result)
	})

	t.Run("publishToMavenLocal variants", func(t *testing.T) {
		// All variations that should NOT trigger publish
		localVariants := []string{
			"publishToMavenLocal",
			":publishToMavenLocal",
			":app:publishToMavenLocal",
		}
		for _, task := range localVariants {
			result := wasPublishCommand([]string{task})
			assert.False(t, result, "Expected false for task: %s", task)
		}
	})

	t.Run("Various publish task patterns", func(t *testing.T) {
		// Tasks that SHOULD trigger publish
		publishTasks := []string{
			"publish",
			":publish",
			":app:publish",
			"publishToArtifactory",
			"publishToNexus",
			"publishAllPublicationsToMavenCentral",
		}
		for _, task := range publishTasks {
			result := wasPublishCommand([]string{task})
			assert.True(t, result, "Expected true for task: %s", task)
		}
	})
}

// The snapshot resolution logic is currently disabled in the implementation.
// Commenting related tests and helpers until/if that flow is re-enabled.
//
// type mockSearchFilesResponder struct {
// 	t *testing.T
// 	// pattern -> file path containing ResultItem JSON lines
// 	patternToFile map[string]string
// 	calls         []string
// }
//
// func (m *mockSearchFilesResponder) SearchFiles(params services.SearchParams) (*content.ContentReader, error) {
// 	m.calls = append(m.calls, params.Pattern)
// 	file, ok := m.patternToFile[params.Pattern]
// 	if !ok {
// 		// Return empty results if pattern isn't mapped.
// 		tmp, err := os.CreateTemp("", "jfrog-empty-*.json")
// 		assert.NoError(m.t, err)
// 		_ = tmp.Close()
// 		return content.NewContentReader(tmp.Name(), content.DefaultKey), nil
// 	}
// 	return content.NewContentReader(file, content.DefaultKey), nil
// }
//
// func writeSearchResultsFile(t *testing.T, items ...servicesutils.ResultItem) string {
// 	w, err := content.NewContentWriter(content.DefaultKey, true, false)
// 	assert.NoError(t, err)
// 	for _, it := range items {
// 		w.Write(it)
// 	}
// 	assert.NoError(t, w.Close())
// 	return w.GetFilePath()
// }
//
// func TestResolveGradleUniqueSnapshotArtifact_PrefersExactSnapshotIfExists_IvySafe(t *testing.T) {
// 	// Exact exists (common for ivy-publish or non-unique snapshot repos)
// 	exactFile := writeSearchResultsFile(t, servicesutils.ResultItem{
// 		Path:     "org/example/gradle-projecy/1.0-SNAPSHOT",
// 		Name:     "gradle-projecy-1.0-SNAPSHOT.jar",
// 		Type:     "file",
// 		Modified: "2025-12-18T10:00:00.000Z",
// 	})
// 	defer func() { _ = os.Remove(exactFile) }()
//
// 	searcher := &mockSearchFilesResponder{
// 		t: t,
// 		patternToFile: map[string]string{
// 			"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-SNAPSHOT.jar": exactFile,
// 		},
// 	}
//
// 	in := servicesutils.ResultItem{
// 		Repo: "gradle",
// 		Path: "org/example/gradle-projecy/1.0-SNAPSHOT",
// 		Name: "gradle-projecy-1.0-SNAPSHOT.jar",
// 	}
//
// 	out, ok, err := resolveGradleUniqueSnapshotArtifact(searcher, in, map[string]bool{}, map[string]bool{}, map[string]*servicesutils.ResultItem{})
// 	assert.NoError(t, err)
// 	assert.False(t, ok)
// 	assert.Equal(t, in, out)
//
// 	// Ensure we did NOT fall back to wildcard lookup when exact exists.
// 	assert.Equal(t, []string{
// 		"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-SNAPSHOT.jar",
// 	}, searcher.calls)
// }
//
// func TestResolveGradleUniqueSnapshotArtifact_ResolvesToNewestTimestampedUniqueSnapshot(t *testing.T) {
// 	// Exact does not exist.
// 	exactEmpty := writeSearchResultsFile(t /* no items */)
// 	defer func() { _ = os.Remove(exactEmpty) }()
//
// 	// Wildcard returns multiple timestamped snapshots; we should pick the newest by Modified.
// 	wildcardFile := writeSearchResultsFile(t,
// 		servicesutils.ResultItem{
// 			Path:     "org/example/gradle-projecy/1.0-SNAPSHOT",
// 			Name:     "gradle-projecy-1.0-20251218.100000-1.jar",
// 			Type:     "file",
// 			Modified: "2025-12-18T10:00:00.000Z",
// 		},
// 		servicesutils.ResultItem{
// 			Path:     "org/example/gradle-projecy/1.0-SNAPSHOT",
// 			Name:     "gradle-projecy-1.0-20251218.101000-2.jar",
// 			Type:     "file",
// 			Modified: "2025-12-18T10:10:00.000Z",
// 		},
// 	)
// 	defer func() { _ = os.Remove(wildcardFile) }()
//
// 	searcher := &mockSearchFilesResponder{
// 		t: t,
// 		patternToFile: map[string]string{
// 			"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-SNAPSHOT.jar": exactEmpty,
// 			"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-*.jar":        wildcardFile,
// 		},
// 	}
//
// 	in := servicesutils.ResultItem{
// 		Repo: "gradle",
// 		Path: "org/example/gradle-projecy/1.0-SNAPSHOT",
// 		Name: "gradle-projecy-1.0-SNAPSHOT.jar",
// 	}
//
// 	out, ok, err := resolveGradleUniqueSnapshotArtifact(searcher, in, map[string]bool{}, map[string]bool{}, map[string]*servicesutils.ResultItem{})
// 	assert.NoError(t, err)
// 	assert.True(t, ok)
// 	assert.Equal(t, "gradle", out.Repo)
// 	assert.Equal(t, "org/example/gradle-projecy/1.0-SNAPSHOT", out.Path)
// 	assert.Equal(t, "gradle-projecy-1.0-20251218.101000-2.jar", out.Name)
// }
//
// func TestResolveGradleUniqueSnapshotArtifact_UsesCachesToAvoidRepeatedSearch(t *testing.T) {
// 	exactEmpty := writeSearchResultsFile(t /* no items */)
// 	defer func() { _ = os.Remove(exactEmpty) }()
// 	wildcardFile := writeSearchResultsFile(t, servicesutils.ResultItem{
// 		Path:     "org/example/gradle-projecy/1.0-SNAPSHOT",
// 		Name:     "gradle-projecy-1.0-20251218.101000-2.pom",
// 		Type:     "file",
// 		Modified: "2025-12-18T10:10:00.000Z",
// 	})
// 	defer func() { _ = os.Remove(wildcardFile) }()
//
// 	searcher := &mockSearchFilesResponder{
// 		t: t,
// 		patternToFile: map[string]string{
// 			"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-SNAPSHOT.pom": exactEmpty,
// 			"gradle/org/example/gradle-projecy/1.0-SNAPSHOT/gradle-projecy-1.0-*.pom":        wildcardFile,
// 		},
// 	}
//
// 	exactCache := map[string]bool{}
// 	wildcardCache := map[string]*servicesutils.ResultItem{}
//
// 	in := servicesutils.ResultItem{
// 		Repo: "gradle",
// 		Path: "org/example/gradle-projecy/1.0-SNAPSHOT",
// 		Name: "gradle-projecy-1.0-SNAPSHOT.pom",
// 	}
//
// 	// First call hits SearchFiles twice (exact + wildcard).
// 	wildcardSearched := map[string]bool{}
// 	out1, ok1, err := resolveGradleUniqueSnapshotArtifact(searcher, in, exactCache, wildcardSearched, wildcardCache)
// 	assert.NoError(t, err)
// 	assert.True(t, ok1)
// 	assert.Equal(t, "gradle-projecy-1.0-20251218.101000-2.pom", out1.Name)
// 	assert.Len(t, searcher.calls, 2)
//
// 	// Second call should be served from caches (no new SearchFiles calls).
// 	out2, ok2, err := resolveGradleUniqueSnapshotArtifact(searcher, in, exactCache, wildcardSearched, wildcardCache)
// 	assert.NoError(t, err)
// 	assert.True(t, ok2)
// 	assert.Equal(t, out1, out2)
// 	assert.Len(t, searcher.calls, 2)
// }
