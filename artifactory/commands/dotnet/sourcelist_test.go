package dotnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNugetSourceList(t *testing.T) {
	dotnetOutput := "Registered Sources:\r\n" +
		"  1.  nuget.org [Enabled]\r\n" +
		"      https://api.nuget.org/v3/index.json\r\n" +
		"  2.  JFrogCli [Enabled]\r\n" +
		"      https://acme.jfrog.io/artifactory/api/nuget/v3/nuget-virtual/index.json\r\n"
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/nuget/v3/nuget-virtual/index.json", ParseNugetSourceList(dotnetOutput, SourceName))

	nugetOutput := "Registered Sources:\n\n  1.  JFrogCli [Aktiviert]\n      https://acme.jfrog.io/artifactory/api/nuget/nuget-local\n"
	assert.Equal(t, "https://acme.jfrog.io/artifactory/api/nuget/nuget-local", ParseNugetSourceList(nugetOutput, SourceName))

	assert.Empty(t, ParseNugetSourceList("Registered Sources:\n  1.  nuget.org [Enabled]\n      https://api.nuget.org/v3/index.json\n", SourceName))
	assert.Empty(t, ParseNugetSourceList("No sources found.", SourceName))
}

func TestIsNugetSourceEnabled(t *testing.T) {
	const source = "https://acme.jfrog.io/artifactory/api/nuget/v3/nuget-virtual/index.json"
	short := "EO https://api.nuget.org/v3/index.json\r\n" + "D " + source + "\r\n"
	assert.False(t, IsNugetSourceEnabled(short, source))
	assert.True(t, IsNugetSourceEnabled("E "+source+"\n", source))
	assert.True(t, IsNugetSourceEnabled("EM "+source+"\n", source))
	assert.True(t, IsNugetSourceEnabled("EO https://api.nuget.org/v3/index.json\n", source), "a source missing from the list counts as enabled")
	const folder = `C:\My Packages`
	assert.False(t, IsNugetSourceEnabled("D  "+folder+"\n", folder), "a local folder source may contain spaces")
}
