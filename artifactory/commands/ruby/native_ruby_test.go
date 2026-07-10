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

	// gem push → --host (trailing slash stripped to avoid double-slash in push URL)
	args := rubyInjectSourceArg(toolGem, "push", []string{"push", "my.gem"}, sourceURL)
	assert.Contains(t, args, "--host")
	assert.Contains(t, args, "https://my.jfrog.io/artifactory/api/gems/gems-virtual")
	assert.NotContains(t, args, sourceURL) // trailing slash must be gone

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

	// gem push with URL that has no trailing slash — should pass through unchanged
	args6 := rubyInjectSourceArg(toolGem, "push", []string{"push", "my.gem"}, "https://host/api/gems/repo")
	assert.Contains(t, args6, "https://host/api/gems/repo")
}

func TestRubyStripHostTrailingSlash(t *testing.T) {
	// --host <url> form
	args := rubyStripHostTrailingSlash([]string{"push", "my.gem", "--host", "https://host/api/gems/repo/"})
	assert.Equal(t, "https://host/api/gems/repo", args[3])

	// --host=<url> form
	args2 := rubyStripHostTrailingSlash([]string{"push", "my.gem", "--host=https://host/api/gems/repo/"})
	assert.Equal(t, "--host=https://host/api/gems/repo", args2[2])

	// No trailing slash — unchanged
	args3 := rubyStripHostTrailingSlash([]string{"push", "my.gem", "--host", "https://host/api/gems/repo"})
	assert.Equal(t, "https://host/api/gems/repo", args3[3])

	// No --host at all — unchanged
	args4 := rubyStripHostTrailingSlash([]string{"push", "my.gem"})
	assert.Equal(t, []string{"push", "my.gem"}, args4)
}

func TestParseGemfileGroups(t *testing.T) {
	dir := t.TempDir()
	gemfile := `source "https://rubygems.org"

gem "rails"
gem "puma"

group :development do
  gem "pry"
  gem "rubocop"
end

group :test do
  gem "rspec"
end

group :development, :test do
  gem "faker"
end
`
	if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte(gemfile), 0644); err != nil {
		t.Fatal(err)
	}

	groups := parseGemfileGroups(dir)
	assert.NotNil(t, groups)

	// Top-level gems → production
	assert.Equal(t, []string{"production"}, groups["rails"])
	assert.Equal(t, []string{"production"}, groups["puma"])

	// Development group
	assert.Equal(t, []string{"development"}, groups["pry"])
	assert.Equal(t, []string{"development"}, groups["rubocop"])

	// Test group
	assert.Equal(t, []string{"test"}, groups["rspec"])

	// Multi-group
	assert.ElementsMatch(t, []string{"development", "test"}, groups["faker"])

	// No Gemfile → nil
	assert.Nil(t, parseGemfileGroups(t.TempDir()))
}

func TestParseGemDeclaration(t *testing.T) {
	assert.Equal(t, "rails", parseGemDeclaration(`gem "rails"`))
	assert.Equal(t, "rails", parseGemDeclaration(`gem 'rails'`))
	assert.Equal(t, "rails", parseGemDeclaration(`gem "rails", "~> 7.0"`))
	assert.Equal(t, "", parseGemDeclaration(`source "https://rubygems.org"`))
	assert.Equal(t, "", parseGemDeclaration(`# gem "commented"`))
}

func TestExtractGemNamesFromArgs(t *testing.T) {
	// Simple install
	names := extractGemNamesFromArgs([]string{"install", "colorize"})
	assert.Equal(t, []string{"colorize"}, names)

	// Multiple gems
	names = extractGemNamesFromArgs([]string{"install", "colorize", "rake", "puma"})
	assert.Equal(t, []string{"colorize", "rake", "puma"}, names)

	// With --source flag (skip flag and value)
	names = extractGemNamesFromArgs([]string{"install", "colorize", "--source", "https://my.jfrog.io/api/gems/r/"})
	assert.Equal(t, []string{"colorize"}, names)

	// With --version flag
	names = extractGemNamesFromArgs([]string{"install", "colorize", "--version", "1.0.0"})
	assert.Equal(t, []string{"colorize"}, names)

	// With -v flag (short)
	names = extractGemNamesFromArgs([]string{"install", "rails", "-v", "7.0.0"})
	assert.Equal(t, []string{"rails"}, names)

	// Fetch subcommand
	names = extractGemNamesFromArgs([]string{"fetch", "rake"})
	assert.Equal(t, []string{"rake"}, names)

	// Boolean flags (no value)
	names = extractGemNamesFromArgs([]string{"install", "rake", "--no-document", "--conservative"})
	assert.Equal(t, []string{"rake"}, names)

	// Path-like arg skipped
	names = extractGemNamesFromArgs([]string{"install", "/path/to/some.gem"})
	assert.Empty(t, names)

	// No gem names (only flags)
	names = extractGemNamesFromArgs([]string{"install", "--source", "https://example.com"})
	assert.Empty(t, names)
}


func TestRubyEmbedCredsInHostArg(t *testing.T) {
	server := &coreConfig.ServerDetails{
		User:           "myuser",
		Password:       "mypass",
		ArtifactoryUrl: "https://my.jfrog.io/artifactory/",
	}

	// --host= form
	args := []string{"push", "my.gem", "--host=https://my.jfrog.io/artifactory/api/gems/gems-local/"}
	result := rubyEmbedCredsInHostArg(args, server)
	assert.Contains(t, result[2], "myuser:mypass@")
	assert.Contains(t, result[2], "--host=https://myuser:mypass@")

	// --host <url> form (separate arg)
	args2 := []string{"push", "my.gem", "--host", "https://my.jfrog.io/artifactory/api/gems/gems-local/"}
	result2 := rubyEmbedCredsInHostArg(args2, server)
	assert.Contains(t, result2[3], "myuser:mypass@")

	// URL already has credentials — no double-embed
	args3 := []string{"push", "my.gem", "--host=https://other:creds@my.jfrog.io/api/gems/r/"}
	result3 := rubyEmbedCredsInHostArg(args3, server)
	assert.Equal(t, args3[2], result3[2])

	// --source flag (not --host) — should NOT be modified
	args4 := []string{"install", "rake", "--source=https://my.jfrog.io/artifactory/api/gems/r/"}
	result4 := rubyEmbedCredsInHostArg(args4, server)
	assert.Equal(t, args4[2], result4[2])

	// No --host — args unchanged
	args5 := []string{"push", "my.gem"}
	result5 := rubyEmbedCredsInHostArg(args5, server)
	assert.Equal(t, args5, result5)
}

func TestCollectsDependencies(t *testing.T) {
	cmd := NewRubyCommand()

	cmd.nativeTool = toolBundle
	assert.True(t, cmd.collectsDependencies("install"))
	assert.True(t, cmd.collectsDependencies("update"))
	assert.True(t, cmd.collectsDependencies("lock"))
	assert.True(t, cmd.collectsDependencies("add"))
	assert.False(t, cmd.collectsDependencies("exec"))

	cmd.nativeTool = toolGem
	assert.True(t, cmd.collectsDependencies("install"))
	assert.True(t, cmd.collectsDependencies("fetch"))
	assert.False(t, cmd.collectsDependencies("build"))
	assert.False(t, cmd.collectsDependencies("push"))
}

func TestParseGemCommandOutput_Install(t *testing.T) {
	// Standard gem install output with transitive deps
	output := `Fetching: activesupport-7.0.4.gem (100%)
Successfully installed activesupport-7.0.4
Fetching: actionpack-7.0.4.gem (100%)
Successfully installed actionpack-7.0.4
Fetching: railties-7.0.4.gem (100%)
Successfully installed railties-7.0.4
Successfully installed rails-7.0.4
4 gems installed
`
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 4)
	assert.Equal(t, "activesupport:7.0.4", deps[0].Id)
	assert.Equal(t, "actionpack:7.0.4", deps[1].Id)
	assert.Equal(t, "railties:7.0.4", deps[2].Id)
	assert.Equal(t, "rails:7.0.4", deps[3].Id)
}

func TestParseGemCommandOutput_InstallSingle(t *testing.T) {
	output := "Successfully installed colorize-1.1.0\n1 gem installed\n"
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 1)
	assert.Equal(t, "colorize:1.1.0", deps[0].Id)
}

func TestParseGemCommandOutput_InstallVersionPin(t *testing.T) {
	// When installing an older version explicitly
	output := "Successfully installed rake-13.0.1\n1 gem installed\n"
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 1)
	assert.Equal(t, "rake:13.0.1", deps[0].Id)
}

func TestParseGemCommandOutput_Fetch(t *testing.T) {
	// gem fetch output
	output := "Downloaded httparty-0.21.0.gem\n"
	deps := parseGemCommandOutput(output, "fetch")
	assert.Len(t, deps, 1)
	assert.Equal(t, "httparty:0.21.0", deps[0].Id)
}

func TestParseGemCommandOutput_FetchMultiple(t *testing.T) {
	output := "Downloaded colorize-1.1.0.gem\nDownloaded rake-13.4.2.gem\n"
	deps := parseGemCommandOutput(output, "fetch")
	assert.Len(t, deps, 2)
	assert.Equal(t, "colorize:1.1.0", deps[0].Id)
	assert.Equal(t, "rake:13.4.2", deps[1].Id)
}

func TestParseGemCommandOutput_FetchOlderFormat(t *testing.T) {
	// Older RubyGems fetch format
	output := "Fetching: rspec-core-3.12.0.gem (100%)\n"
	deps := parseGemCommandOutput(output, "fetch")
	assert.Len(t, deps, 1)
	assert.Equal(t, "rspec-core:3.12.0", deps[0].Id)
}

func TestParseGemCommandOutput_HyphenatedGemName(t *testing.T) {
	// Gem name with hyphens (e.g., rspec-core, net-http)
	output := "Successfully installed rspec-core-3.12.0\nSuccessfully installed net-http-0.4.1\n"
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 2)
	assert.Equal(t, "rspec-core:3.12.0", deps[0].Id)
	assert.Equal(t, "net-http:0.4.1", deps[1].Id)
}

func TestParseGemCommandOutput_Empty(t *testing.T) {
	deps := parseGemCommandOutput("", "install")
	assert.Nil(t, deps)
}

func TestParseGemCommandOutput_NoDeps(t *testing.T) {
	// Output with no install/download lines (e.g., already installed)
	output := "Successfully installed colorize-1.1.0\nBut this line has no prefix\n"
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 1)
}

func TestParseGemCommandOutput_Deduplication(t *testing.T) {
	// Same gem mentioned twice (should deduplicate)
	output := "Successfully installed rake-13.4.2\nSuccessfully installed rake-13.4.2\n"
	deps := parseGemCommandOutput(output, "install")
	assert.Len(t, deps, 1)
}

func TestSplitGemNameVersion(t *testing.T) {
	cases := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"colorize-1.1.0", "colorize", "1.1.0"},
		{"rspec-core-3.12.0", "rspec-core", "3.12.0"},
		{"net-http-0.4.1", "net-http", "0.4.1"},
		{"rails-7.0.4.2", "rails", "7.0.4.2"},
		{"nokogiri-1.13.9-x86_64-linux", "nokogiri", "1.13.9-x86_64-linux"}, // platform suffix in version
		{"", "", ""},
		{"noversion", "", ""},
		{"rake-13.4.2", "rake", "13.4.2"},
	}
	for _, c := range cases {
		name, version := splitGemNameVersion(c.input)
		assert.Equal(t, c.wantName, name, "input %q name", c.input)
		assert.Equal(t, c.wantVersion, version, "input %q version", c.input)
	}
}

func TestExtractVersionFromArgs(t *testing.T) {
	// -v flag
	assert.Equal(t, "13.0.1", extractVersionFromArgs([]string{"install", "rake", "-v", "13.0.1"}))

	// --version flag
	assert.Equal(t, "7.0.0", extractVersionFromArgs([]string{"install", "rails", "--version", "7.0.0"}))

	// --version= form
	assert.Equal(t, "1.2.3", extractVersionFromArgs([]string{"install", "gem", "--version=1.2.3"}))

	// No version flag
	assert.Equal(t, "", extractVersionFromArgs([]string{"install", "rake"}))

	// -v without space (unusual but valid)
	assert.Equal(t, "1.0.0", extractVersionFromArgs([]string{"install", "gem", "-v1.0.0"}))
}

func TestRubyWriteTempGemCredentials(t *testing.T) {
	// Use a temporary home directory to avoid touching real ~/.gem/credentials
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	server := &coreConfig.ServerDetails{
		User:           "admin",
		Password:       "secret",
		ArtifactoryUrl: "https://my.jfrog.io/artifactory/",
	}

	// Host URL WITH trailing slash — must be preserved exactly in the credentials key.
	hostURL := "https://my.jfrog.io/artifactory/api/gems/gems-local/"
	cleanup, err := rubyWriteTempGemCredentials(hostURL, server)
	assert.NoError(t, err)
	assert.NotNil(t, cleanup)

	// Verify credentials file was written with EXACT host URL (trailing slash preserved).
	credFile := filepath.Join(tmpHome, ".gem", "credentials")
	content, readErr := os.ReadFile(credFile)
	assert.NoError(t, readErr)
	// Key MUST include the trailing slash — RubyGems does exact string match against --host value.
	assert.Contains(t, string(content), "https://my.jfrog.io/artifactory/api/gems/gems-local/: Basic ")

	// Run cleanup
	cleanup()

	// File should be removed (didn't exist before)
	_, statErr := os.Stat(credFile)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRubyWriteTempGemCredentials_TrailingSlashPreserved(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	server := &coreConfig.ServerDetails{
		User:     "admin",
		Password: "pass",
	}

	// With trailing slash
	cleanup1, err := rubyWriteTempGemCredentials("https://host/api/gems/repo/", server)
	assert.NoError(t, err)
	content, _ := os.ReadFile(filepath.Join(tmpHome, ".gem", "credentials"))
	assert.Contains(t, string(content), "https://host/api/gems/repo/: Basic ")
	cleanup1()

	// Without trailing slash
	cleanup2, err := rubyWriteTempGemCredentials("https://host/api/gems/repo", server)
	assert.NoError(t, err)
	content, _ = os.ReadFile(filepath.Join(tmpHome, ".gem", "credentials"))
	assert.Contains(t, string(content), "https://host/api/gems/repo: Basic ")
	assert.NotContains(t, string(content), "https://host/api/gems/repo/: Basic ")
	cleanup2()
}

func TestRubyWriteTempGemCredentials_PreservesExisting(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create pre-existing credentials
	gemDir := filepath.Join(tmpHome, ".gem")
	os.MkdirAll(gemDir, 0700)
	existingContent := "---\n:rubygems_api_key: existing-key\n"
	os.WriteFile(filepath.Join(gemDir, "credentials"), []byte(existingContent), 0600)

	server := &coreConfig.ServerDetails{
		User:     "admin",
		Password: "token123",
	}

	cleanup, err := rubyWriteTempGemCredentials("https://host/api/gems/repo/", server)
	assert.NoError(t, err)

	// File should have both entries (key preserves trailing slash)
	content, _ := os.ReadFile(filepath.Join(gemDir, "credentials"))
	assert.Contains(t, string(content), ":rubygems_api_key: existing-key")
	assert.Contains(t, string(content), "https://host/api/gems/repo/: Basic ")

	// Cleanup should restore original
	cleanup()
	restored, _ := os.ReadFile(filepath.Join(gemDir, "credentials"))
	assert.Equal(t, existingContent, string(restored))
}
