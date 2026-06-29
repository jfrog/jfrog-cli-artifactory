package ruby

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBundleEnvKeyForHost(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"mycompany.jfrog.io", "BUNDLE_MYCOMPANY__JFROG__IO"},
		{"my-art.example.com", "BUNDLE_MY___ART__EXAMPLE__COM"},
		{"localhost:8081", "BUNDLE_LOCALHOST_8081"},
		{"artifactory", "BUNDLE_ARTIFACTORY"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, bundleEnvKeyForHost(c.host), "host %q", c.host)
	}
}

func TestRubyExtractRepoKeyFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://my.jfrog.io/artifactory/api/gems/gems-local/", "gems-local"},
		{"https://my.jfrog.io/api/gems/gems-remote", "gems-remote"},
		{"gems-local", "gems-local"}, // bare key passthrough
		{"https://rubygems.org/", ""}, // no /api/gems/ segment
		{"", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, rubyExtractRepoKeyFromURL(c.in), "input %q", c.in)
	}
}

func TestRubySourceFromArgs(t *testing.T) {
	assert.Equal(t, "https://h/api/gems/r/", rubySourceFromArgs([]string{"install", "--source", "https://h/api/gems/r/"}))
	assert.Equal(t, "https://h/api/gems/r/", rubySourceFromArgs([]string{"push", "x.gem", "--host=https://h/api/gems/r/"}))
	assert.Equal(t, "https://h/api/gems/r/", rubySourceFromArgs([]string{"install", "-s", "https://h/api/gems/r/"}))
	assert.Equal(t, "", rubySourceFromArgs([]string{"install", "rake"}))
}

func TestRubySourceFromGemfile(t *testing.T) {
	dir := t.TempDir()
	gemfile := `source "https://rubygems.org"
source 'https://my.jfrog.io/artifactory/api/gems/gems-virtual/'

gem "rails"
`
	if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte(gemfile), 0644); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "https://my.jfrog.io/artifactory/api/gems/gems-virtual/", rubySourceFromGemfile(dir))

	// No artifactory source → empty.
	empty := t.TempDir()
	_ = os.WriteFile(filepath.Join(empty, "Gemfile"), []byte(`source "https://rubygems.org"`), 0644)
	assert.Equal(t, "", rubySourceFromGemfile(empty))

	// No Gemfile → empty.
	assert.Equal(t, "", rubySourceFromGemfile(t.TempDir()))
}

func TestIsRubyHelpRequest(t *testing.T) {
	assert.True(t, isRubyHelpRequest("help", []string{"help"}))
	assert.True(t, isRubyHelpRequest("", nil))
	assert.True(t, isRubyHelpRequest("install", []string{"install", "--help"}))
	assert.True(t, isRubyHelpRequest("install", []string{"install", "-h"}))
	assert.False(t, isRubyHelpRequest("install", []string{"install", "rake"}))
}

func TestParseBundleListLine(t *testing.T) {
	name, version := parseBundleListLine("rake (13.0.6)")
	assert.Equal(t, "rake", name)
	assert.Equal(t, "13.0.6", version)

	name, version = parseBundleListLine("nokogiri (1.13.9-x86_64-linux)")
	assert.Equal(t, "nokogiri", name)
	assert.Equal(t, "1.13.9-x86_64-linux", version)

	name, _ = parseBundleListLine("Gems included by the bundle:")
	assert.Equal(t, "", name)
}

func TestExtractQuotedURL(t *testing.T) {
	assert.Equal(t, "https://x/y", extractQuotedURL(`source "https://x/y"`))
	assert.Equal(t, "https://x/y", extractQuotedURL(`source 'https://x/y'`))
	assert.Equal(t, "", extractQuotedURL(`source https://x/y`))
}

func TestRubyCommandSettersAndName(t *testing.T) {
	cmd := NewRubyCommand().
		SetNativeTool("bundle").
		SetArgs([]string{"install"}).
		SetServerID("my-server").
		SetRepo("gems-local")
	assert.Equal(t, "rt_ruby_native", cmd.CommandName())
	assert.Equal(t, "bundle", cmd.nativeTool)
	assert.Equal(t, "gems-local", cmd.repository)
	assert.Equal(t, "my-server", cmd.serverID)
}

func TestRubyHostMatchesServer(t *testing.T) {
	assert.True(t, rubyHostMatchesServer("https://my.jfrog.io/artifactory/api/gems/r/", "https://my.jfrog.io/artifactory"))
	assert.False(t, rubyHostMatchesServer("https://other.com/api/gems/r/", "https://my.jfrog.io/artifactory"))
	assert.False(t, rubyHostMatchesServer("", "https://my.jfrog.io/artifactory"))
}

func TestRubyRunUnsupportedTool(t *testing.T) {
	cmd := NewRubyCommand().SetNativeTool("npm").SetArgs([]string{"install"})
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ruby tool")
}
