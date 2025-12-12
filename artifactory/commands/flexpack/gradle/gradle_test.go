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
