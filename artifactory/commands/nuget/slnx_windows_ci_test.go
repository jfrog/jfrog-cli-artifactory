package nuget

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	dotnetutils "github.com/jfrog/build-info-go/build/utils/dotnet"
	"github.com/jfrog/gofrog/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSlnxGateWithRealNugetExe runs the .slnx gate against the real nuget.exe on PATH.
// It is skipped unless SLNX_EXPECT_NUGET_MAJOR is set (only the slnx-windows CI job sets it).
func TestSlnxGateWithRealNugetExe(t *testing.T) {
	expected := os.Getenv("SLNX_EXPECT_NUGET_MAJOR")
	if expected == "" {
		t.Skip("SLNX_EXPECT_NUGET_MAJOR not set")
	}
	expectedMajor, err := strconv.Atoi(expected)
	require.NoError(t, err)

	nugetVersion := nugetExeVersion()
	t.Logf("nuget.exe version detected: %q", nugetVersion)
	require.NotEmpty(t, nugetVersion, "the 'NuGet Version:' banner must be parsed from the real nuget.exe")
	major, err := strconv.Atoi(strings.SplitN(nugetVersion, ".", 2)[0])
	require.NoError(t, err)
	assert.Equal(t, expectedMajor, major)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "App"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Test.slnx"),
		[]byte("<Solution>\n  <Project Path=\"App/App.csproj\" />\n</Solution>\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "App", "App.csproj"), []byte(
		`<Project ToolsVersion="15.0" xmlns="http://schemas.microsoft.com/developer/msbuild/2003">
  <PropertyGroup><TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion></PropertyGroup>
</Project>
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "App", "packages.config"),
		[]byte(`<?xml version="1.0" encoding="utf-8"?><packages></packages>`), 0o600))

	runErr := NewNuGetFlexPackCommand().
		SetToolchainType(dotnetutils.Nuget).
		SetSubCommand("restore").
		SetArgs([]string{filepath.Join(dir, "Test.slnx")}).
		SetWorkingDir(dir).
		Run()

	if !version.NewVersion(nugetVersion).AtLeast(minSlnxNugetVersion) {
		require.Error(t, runErr)
		assert.Contains(t, runErr.Error(), ".slnx requires nuget.exe >= 7.0.0")
		return
	}
	// nuget.exe >= 7 must get past the gate and restore the solution on a runner with MSBuild >= 17.13.
	if runErr != nil {
		assert.NotContains(t, runErr.Error(), ".slnx requires nuget.exe", "the gate must not block nuget.exe >= 7")
	}
	assert.NoError(t, runErr, "nuget.exe >= 7 with MSBuild >= 17.13 must restore the .slnx")
}
