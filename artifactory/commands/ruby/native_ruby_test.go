package ruby

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	buildinfo "github.com/jfrog/build-info-go/entities"
	buildUtils "github.com/jfrog/jfrog-cli-core/v2/common/build"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, c.want, BundleEnvKeyForHost(c.host), "host %q", c.host)
	}
}

func TestRubyExtractRepoKeyFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://my.jfrog.io/artifactory/api/gems/gems-local/", "gems-local"},
		{"https://my.jfrog.io/api/gems/gems-remote", "gems-remote"},
		{"gems-local", "gems-local"},  // bare key passthrough
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

func TestRubyExtractInstallDir(t *testing.T) {
	assert.Equal(t, "/opt/gems", rubyExtractInstallDir([]string{"install", "rake", "-i", "/opt/gems"}))
	assert.Equal(t, "/opt/gems", rubyExtractInstallDir([]string{"install", "rake", "--install-dir", "/opt/gems"}))
	assert.Equal(t, "/opt/gems", rubyExtractInstallDir([]string{"install", "rake", "--install-dir=/opt/gems"}))
	assert.Equal(t, "", rubyExtractInstallDir([]string{"install", "rake"}))
}

// TestRubyDiffGemSnapshots_Install covers the pure diff logic that replaces parsing
// "Successfully installed X-Y" from stdout: whatever is new or changed version between
// the before/after installed-spec snapshots is reported, transitive dependencies included.
func TestRubyDiffGemSnapshots_Install(t *testing.T) {
	before := rubyGemSnapshot{installedVersions: map[string]string{"bundler": "1.17.2", "rake": "13.0.1"}}
	after := rubyGemSnapshot{installedVersions: map[string]string{
		"bundler":  "1.17.2", // unchanged — must not appear
		"rake":     "13.0.1", // unchanged — must not appear
		"rails":    "7.0.4",  // newly installed
		"railties": "7.0.4",  // newly installed transitive dependency
	}}
	deps := rubyDiffGemSnapshots("install", "", before, after)
	ids := make([]string, len(deps))
	for i, d := range deps {
		ids[i] = d.Id
		assert.Equal(t, gemDepArtifactType, d.Type)
	}
	assert.ElementsMatch(t, []string{"rails:7.0.4", "railties:7.0.4"}, ids)
}

// TestRubyDiffGemSnapshots_InstallVersionChange covers reinstalling an existing gem at a
// different version (e.g. `gem install rake -v 13.0.1` when 13.4.2 was already present).
func TestRubyDiffGemSnapshots_InstallVersionChange(t *testing.T) {
	before := rubyGemSnapshot{installedVersions: map[string]string{"rake": "13.4.2"}}
	after := rubyGemSnapshot{installedVersions: map[string]string{"rake": "13.0.1"}}
	deps := rubyDiffGemSnapshots("install", "", before, after)
	require.Len(t, deps, 1)
	assert.Equal(t, "rake:13.0.1", deps[0].Id)
}

func TestRubyDiffGemSnapshots_InstallNoChange(t *testing.T) {
	snap := rubyGemSnapshot{installedVersions: map[string]string{"rake": "13.4.2"}}
	assert.Empty(t, rubyDiffGemSnapshots("install", "", snap, snap))
}

// TestRubyGemFetchIntegration exercises rubyGemFilesIn, rubyReadGemFileSpec and the
// "fetch" branch of rubyDiffGemSnapshots end to end against a real .gem file built by
// the actual `gem` binary — the mechanism `gem fetch` itself would produce a file for,
// without needing network access to fetch a real gem.
func TestRubyGemFetchIntegration(t *testing.T) {
	if _, err := exec.LookPath("gem"); err != nil {
		t.Skip("gem not on PATH")
	}
	dir := t.TempDir()
	gemspec := "Gem::Specification.new do |s|\n" +
		"  s.name = \"jf-ruby-diff-test\"\n" +
		"  s.version = \"0.0.1\"\n" +
		"  s.summary = \"test\"\n" +
		"  s.authors = [\"test\"]\n" +
		"  s.files = []\n" +
		"end\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "jf-ruby-diff-test.gemspec"), []byte(gemspec), 0644))

	before, err := rubyGemFilesIn(dir)
	require.NoError(t, err)
	assert.Empty(t, before)

	cmd := exec.Command("gem", "build", "jf-ruby-diff-test.gemspec")
	cmd.Dir = dir
	out, buildErr := cmd.CombinedOutput()
	require.NoError(t, buildErr, "gem build failed: %s", string(out))

	after, err := rubyGemFilesIn(dir)
	require.NoError(t, err)
	assert.Len(t, after, 1)

	deps := rubyDiffGemSnapshots("fetch", dir, rubyGemSnapshot{gemFiles: before}, rubyGemSnapshot{gemFiles: after})
	require.Len(t, deps, 1)
	assert.Equal(t, "jf-ruby-diff-test:0.0.1", deps[0].Id)
}

// TestRubyInstalledGemVersions is a light integration check that the RubyGems
// Specification API query itself runs and returns parseable, non-trivial output.
func TestRubyInstalledGemVersions(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not on PATH")
	}
	versions, err := rubyInstalledGemVersions("")
	require.NoError(t, err)
	assert.NotEmpty(t, versions, "expected at least the default gems bundled with Ruby itself")
}

func TestCollectsDependencies(t *testing.T) {
	cmd := NewRubyCommand()

	cmd.nativeTool = toolBundle
	assert.True(t, cmd.collectsDependencies("install"))
	assert.True(t, cmd.collectsDependencies("update"))
	assert.False(t, cmd.collectsDependencies("lock"))
	assert.True(t, cmd.collectsDependencies("add"))
	assert.False(t, cmd.collectsDependencies("exec"))

	cmd.nativeTool = toolGem
	assert.True(t, cmd.collectsDependencies("install"))
	assert.True(t, cmd.collectsDependencies("fetch"))
	assert.False(t, cmd.collectsDependencies("build"))
	assert.False(t, cmd.collectsDependencies("push"))
}

func TestRubyWriteTempGemCredentials(t *testing.T) {
	// Use a temporary home directory to avoid touching real ~/.gem/credentials
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // os.UserHomeDir() reads USERPROFILE on Windows

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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // os.UserHomeDir() reads USERPROFILE on Windows

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
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // os.UserHomeDir() reads USERPROFILE on Windows

	// Create pre-existing credentials
	gemDir := filepath.Join(tmpHome, ".gem")
	require.NoError(t, os.MkdirAll(gemDir, 0700))
	existingContent := "---\n:rubygems_api_key: existing-key\n"
	require.NoError(t, os.WriteFile(filepath.Join(gemDir, "credentials"), []byte(existingContent), 0600))

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

// TestRubyEnrichDepsChecksums_NeverTrustsLocalCache guards against reintroducing a provenance
// bug: checksum enrichment must only ever come from a verified Artifactory (AQL) lookup, never
// from a file merely present in the local RubyGems cache. Without network access (serverDetails
// nil), enrichment must leave checksums empty rather than falling back to any local filesystem
// read — there must be no code path that can populate build-info checksums without going through
// Artifactory.
func TestRubyEnrichDepsChecksums_NeverTrustsLocalCache(t *testing.T) {
	deps := []buildinfo.Dependency{
		{Id: "rake:13.4.2", Type: "gem"},
	}

	rubyEnrichDepsChecksums(deps, "my-gems-repo", nil, nil)

	assert.Empty(t, deps[0].Sha1, "checksum must not be populated without a verified Artifactory lookup")
	assert.Empty(t, deps[0].Sha256, "checksum must not be populated without a verified Artifactory lookup")
	assert.Empty(t, deps[0].Md5, "checksum must not be populated without a verified Artifactory lookup")
}

// TestBundleCredentialKeys pins the key spellings against what real Bundler computes.
// Verified against Bundler 1.17.2 and 4.0.16: 1.x leaves dashes and colons intact, while
// 2.x+ turns dashes into "___". Both spellings must be emitted or one major silently
// fails to authenticate.
func TestBundleCredentialKeys(t *testing.T) {
	// Plain host: a single spelling, identical on every Bundler version.
	assert.Equal(t, []string{"BUNDLE_ACME__JFROG__IO"}, BundleCredentialKeys("acme.jfrog.io"))

	// Dashed host: Bundler 2.x+ wants "___", Bundler 1.x wants the dash preserved.
	assert.Equal(t,
		[]string{"BUNDLE_MY___COMPANY__JFROG__IO", "BUNDLE_MY-COMPANY__JFROG__IO"},
		BundleCredentialKeys("my-company.jfrog.io"))

	// Ported host: Bundler preserves the colon, which the env-var spelling cannot.
	assert.Equal(t,
		[]string{"BUNDLE_LOCALHOST_8081", "BUNDLE_LOCALHOST:8081"},
		BundleCredentialKeys("localhost:8081"))
}

// TestRubyGemfileDir_WalksUpAndHonorsEnv guards the subdirectory case: Bundler loads the
// Gemfile from a parent directory, so a command run from a subdirectory must resolve the
// same file or no credentials get injected and `bundle install` fails with exit status 16.
func TestRubyGemfileDir_WalksUpAndHonorsEnv(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Gemfile"), []byte("source \"https://rubygems.org\"\n"), 0644))
	nested := filepath.Join(root, "app", "models")
	require.NoError(t, os.MkdirAll(nested, 0755))

	// From a subdirectory, the Gemfile above must be found.
	assert.Equal(t, root, rubyGemfileDir(nested))
	assert.Equal(t, filepath.Join(root, "Gemfile"), rubyGemfilePath(nested))

	// With no Gemfile anywhere, the working directory is returned unchanged.
	bare := t.TempDir()
	assert.Equal(t, bare, rubyGemfileDir(bare))

	// BUNDLE_GEMFILE wins over the directory walk, as it does for Bundler itself.
	other := t.TempDir()
	custom := filepath.Join(other, "Custom.gemfile")
	require.NoError(t, os.WriteFile(custom, []byte("source \"https://rubygems.org\"\n"), 0644))
	t.Setenv("BUNDLE_GEMFILE", custom)
	assert.Equal(t, other, rubyGemfileDir(nested))
	assert.Equal(t, custom, rubyGemfilePath(nested))
}

// TestGemModuleID prefers the gemspec identity so module IDs are stable across machines,
// rather than the working-directory name (which produced IDs like "tmp.46NMMEmRLv") or the
// command name (which made every project report "gem-install").
func TestGemModuleID(t *testing.T) {
	rc := &RubyCommand{}

	// A gemspec with literal name and version wins.
	withSpec := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(withSpec, "Gemfile"), []byte("source \"x\"\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(withSpec, "demo.gemspec"), []byte(
		"Gem::Specification.new do |s|\n  s.name = \"my-gem\"\n  s.version = \"1.2.3\"\nend\n"), 0644))
	assert.Equal(t, "my-gem:1.2.3", rc.gemModuleID(withSpec))

	// No gemspec (a Rails-style app): fall back to the Gemfile's directory name.
	appDir := filepath.Join(t.TempDir(), "my-app")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "Gemfile"), []byte("source \"x\"\n"), 0644))
	assert.Equal(t, "my-app", rc.gemModuleID(appDir))

	// A gemspec that computes its version in Ruby cannot be read without executing it,
	// so the name alone is used rather than reporting a wrong version.
	computed := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(computed, "c.gemspec"), []byte(
		"Gem::Specification.new do |s|\n  s.name = \"calc-gem\"\n  s.version = Calc::VERSION\nend\n"), 0644))
	assert.Equal(t, "calc-gem", rc.gemModuleID(computed))
}

// TestGemspecField covers the layouts a real gemspec uses: one assignment per line, several
// separated by semicolons, and attributes whose names merely contain "version".
func TestGemspecField(t *testing.T) {
	perLine := "Gem::Specification.new do |spec|\n  spec.name    = \"a-gem\"\n  spec.version = \"2.0.1\"\nend\n"
	assert.Equal(t, "a-gem", gemspecField(perLine, "name"))
	assert.Equal(t, "2.0.1", gemspecField(perLine, "version"))

	semicolons := "Gem::Specification.new do |s|\n  s.name=\"b-gem\"; s.version=\"3.1.4\"; s.summary=\"x\"\nend\n"
	assert.Equal(t, "b-gem", gemspecField(semicolons, "name"))
	assert.Equal(t, "3.1.4", gemspecField(semicolons, "version"))

	// required_ruby_version must not be mistaken for version.
	tricky := "Gem::Specification.new do |s|\n  s.required_ruby_version = \">= 3.0\"\n  s.name = \"c-gem\"\nend\n"
	assert.Equal(t, "", gemspecField(tricky, "version"))
	assert.Equal(t, "c-gem", gemspecField(tricky, "name"))
}

// TestRubySubCommand: global flags may precede the subcommand. Treating args[0] as the
// subcommand made `gem --backtrace install rake` skip auth injection and build-info while
// still exiting 0.
func TestRubySubCommand(t *testing.T) {
	assert.Equal(t, "install", rubySubCommand([]string{"install", "rake"}))
	assert.Equal(t, "install", rubySubCommand([]string{"--backtrace", "install", "rake"}))
	assert.Equal(t, "push", rubySubCommand([]string{"--debug", "--norc", "push", "a.gem"}))
	// A flag's separate value must not be mistaken for the subcommand.
	assert.Equal(t, "install", rubySubCommand([]string{"--config-file", "/tmp/gemrc", "install", "rake"}))
	assert.Equal(t, "install", rubySubCommand([]string{"--retry", "3", "install"}))
	// Flags only: no subcommand at all.
	assert.Equal(t, "", rubySubCommand([]string{"--version"}))
	assert.Equal(t, "", rubySubCommand(nil))
}

// TestRubyAppendToolArgs: everything after "--" is forwarded to the C extension build, so
// injected flags must land before it or gem never sees them.
func TestRubyAppendToolArgs(t *testing.T) {
	assert.Equal(t,
		[]string{"install", "rake", "--source", "https://u@h/api/gems/r"},
		rubyAppendToolArgs([]string{"install", "rake"}, "--source", "https://u@h/api/gems/r"))

	assert.Equal(t,
		[]string{"install", "nokogiri", "--source", "https://u@h/api/gems/r", "--", "--use-system-libraries"},
		rubyAppendToolArgs([]string{"install", "nokogiri", "--", "--use-system-libraries"}, "--source", "https://u@h/api/gems/r"))
}

// TestRubyPublishHint: the project key must be repeated on `jf rt bp`, because partials are
// stored per project and omitting it publishes an empty build instead.
func TestRubyPublishHint(t *testing.T) {
	assert.Equal(t, "jf rt bp mybuild 7",
		rubyPublishHint(buildUtils.NewBuildConfiguration("mybuild", "7", "", ""), "mybuild", "7"))
	assert.Equal(t, "jf rt bp mybuild 7 --project=proj1",
		rubyPublishHint(buildUtils.NewBuildConfiguration("mybuild", "7", "", "proj1"), "mybuild", "7"))
	assert.Equal(t, "jf rt bp mybuild 7", rubyPublishHint(nil, "mybuild", "7"))
}

// TestUserHomeDir_PrefersHome pins the resolution RubyGems and Bundler themselves use.
// On Windows os.UserHomeDir reads %USERPROFILE%, but Ruby's Dir.home prefers $HOME, so
// writing to the former would put ~/.gemrc and ~/.bundle/config where neither tool looks.
func TestUserHomeDir_PrefersHome(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("HOME", custom)
	home, err := UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, custom, home)

	// With no HOME set, fall back to whatever the platform reports.
	t.Setenv("HOME", "")
	fallback, fallbackErr := UserHomeDir()
	expected, expectedErr := os.UserHomeDir()
	assert.Equal(t, expectedErr == nil, fallbackErr == nil)
	if expectedErr == nil {
		assert.Equal(t, expected, fallback)
	}
}
