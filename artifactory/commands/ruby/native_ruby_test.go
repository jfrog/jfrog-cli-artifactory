package ruby

import (
	"os"
	"path/filepath"
	"testing"

	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
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
	assert.False(t, isRubyHelpRequest("", nil)) // empty subCommand is now caught before help check
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

func TestRubyRunNoArgs(t *testing.T) {
	cmd := NewRubyCommand().SetNativeTool("gem").SetArgs(nil)
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no subcommand provided")
}

func TestRubyEmbedCredsInSourceArg(t *testing.T) {
	server := &coreConfig.ServerDetails{
		User:           "myuser",
		Password:       "mypass",
		ArtifactoryUrl: "https://my.jfrog.io/artifactory/",
	}

	// --source= form
	args := []string{"install", "rake", "--source=https://my.jfrog.io/artifactory/api/gems/gems-virtual/"}
	result := rubyEmbedCredsInSourceArg(args, server)
	assert.Contains(t, result[2], "myuser:mypass@")
	assert.Contains(t, result[2], "--source=https://myuser:mypass@")

	// --source <url> form (separate arg)
	args2 := []string{"install", "rake", "--source", "https://my.jfrog.io/artifactory/api/gems/gems-virtual/"}
	result2 := rubyEmbedCredsInSourceArg(args2, server)
	assert.Contains(t, result2[3], "myuser:mypass@")

	// -s <url> short form
	args3 := []string{"fetch", "rake", "-s", "https://my.jfrog.io/artifactory/api/gems/gems-virtual/"}
	result3 := rubyEmbedCredsInSourceArg(args3, server)
	assert.Contains(t, result3[3], "myuser:mypass@")

	// URL already has credentials — should not double-embed
	args4 := []string{"install", "rake", "--source=https://other:creds@my.jfrog.io/api/gems/r/"}
	result4 := rubyEmbedCredsInSourceArg(args4, server)
	assert.Equal(t, args4[2], result4[2])

	// No --source/--host — args unchanged
	args5 := []string{"install", "rake"}
	result5 := rubyEmbedCredsInSourceArg(args5, server)
	assert.Equal(t, args5, result5)
}

func TestRubyConstructRepoURL(t *testing.T) {
	server := &coreConfig.ServerDetails{
		ArtifactoryUrl: "https://my.jfrog.io/artifactory/",
	}

	u, err := rubyConstructRepoURL(server, "gems-virtual")
	assert.NoError(t, err)
	assert.Equal(t, "https://my.jfrog.io/artifactory/api/gems/gems-virtual/", u)

	u2, err := rubyConstructRepoURL(server, "gems-local")
	assert.NoError(t, err)
	assert.Equal(t, "https://my.jfrog.io/artifactory/api/gems/gems-local/", u2)

	// Without trailing slash on base URL
	server2 := &coreConfig.ServerDetails{
		ArtifactoryUrl: "https://my.jfrog.io/artifactory",
	}
	u3, err := rubyConstructRepoURL(server2, "gems-remote")
	assert.NoError(t, err)
	assert.Equal(t, "https://my.jfrog.io/artifactory/api/gems/gems-remote/", u3)
}

func TestRubyInjectSourceArg(t *testing.T) {
	sourceURL := "https://my.jfrog.io/artifactory/api/gems/gems-virtual/"

	// gem push → --host
	args := rubyInjectSourceArg(toolGem, "push", []string{"push", "my.gem"}, sourceURL)
	assert.Contains(t, args, "--host")
	assert.Contains(t, args, sourceURL)

	// gem install → --source
	args2 := rubyInjectSourceArg(toolGem, "install", []string{"install", "rake"}, sourceURL)
	assert.Contains(t, args2, "--source")
	assert.Contains(t, args2, sourceURL)

	// gem fetch → --source
	args3 := rubyInjectSourceArg(toolGem, "fetch", []string{"fetch", "rake"}, sourceURL)
	assert.Contains(t, args3, "--source")
	assert.Contains(t, args3, sourceURL)

	// gem build → no injection (doesn't need source)
	args4 := rubyInjectSourceArg(toolGem, "build", []string{"build", "my.gemspec"}, sourceURL)
	assert.NotContains(t, args4, "--source")
	assert.NotContains(t, args4, "--host")

	// bundle install → no injection (bundle uses env var auth, not --source)
	args5 := rubyInjectSourceArg(toolBundle, "install", []string{"install"}, sourceURL)
	assert.NotContains(t, args5, "--source")
	assert.NotContains(t, args5, "--host")
}
