package maven

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveResolutionCommand(t *testing.T) {
	assert.Equal(t, "resolve", DeriveResolutionCommand([]string{"install"}))
	assert.Equal(t, "resolve", DeriveResolutionCommand([]string{"clean", "verify"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"help"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"clean"}))
	assert.Equal(t, "", DeriveResolutionCommand([]string{"dependency:tree"}))
}

func TestShouldSkipResolution(t *testing.T) {
	assert.True(t, ShouldSkipResolution([]string{"help"}))
	assert.False(t, ShouldSkipResolution([]string{"install"}))
}

func TestParseMavenCLIArgs_FileFlag(t *testing.T) {
	pom, projects := parseMavenCLIArgs([]string{"-f", "sub/pom.xml", "install"})
	assert.Equal(t, "sub/pom.xml", pom)
	assert.Nil(t, projects)

	pom, _ = parseMavenCLIArgs([]string{"--file", "other/pom.xml", "clean", "install"})
	assert.Equal(t, "other/pom.xml", pom)
}

func TestParseMavenCLIArgs_ProjectsFlag(t *testing.T) {
	_, projects := parseMavenCLIArgs([]string{"-pl", "mod-a,mod-b", "install"})
	assert.Equal(t, []string{"mod-a", "mod-b"}, projects)

	_, projects = parseMavenCLIArgs([]string{"--projects", "services/api", "verify"})
	assert.Equal(t, []string{"services/api"}, projects)
}

func TestParseMavenCLIArgs_NoFlags(t *testing.T) {
	pom, projects := parseMavenCLIArgs([]string{"clean", "install"})
	assert.Empty(t, pom)
	assert.Nil(t, projects)
}
