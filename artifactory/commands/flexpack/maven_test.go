package flexpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	bidflexpack "github.com/jfrog/build-info-go/flexpack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateMavenCoordinate verifies that the path-traversal sanitizer accepts legitimate Maven
// coordinates/packaging types and rejects crafted values that could escape the target directory.
func TestValidateMavenCoordinate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		// Legitimate values - dots are valid separators, hyphens/underscores allowed.
		{name: "simple groupId", value: "com.example", wantErr: false},
		{name: "nested groupId", value: "org.apache.maven.plugins", wantErr: false},
		{name: "artifactId with hyphen", value: "my-app", wantErr: false},
		{name: "release version", value: "1.0.0", wantErr: false},
		{name: "snapshot version", value: "1.2.3-SNAPSHOT", wantErr: false},
		{name: "packaging jar", value: "jar", wantErr: false},
		{name: "packaging bundle", value: "bundle", wantErr: false},

		// Empty string is rejected by the allowlist guard.
		{name: "empty string", value: "", wantErr: true},

		// Path traversal sequences.
		{name: "double dot", value: "..", wantErr: true},
		{name: "parent traversal", value: "../../etc", wantErr: true},
		{name: "embedded double dot", value: "1.0..0", wantErr: true},

		// Path separators.
		{name: "forward slash", value: "com/example", wantErr: true},
		{name: "backslash", value: "com\\example", wantErr: true},
		{name: "absolute path", value: "/etc/passwd", wantErr: true},

		// Null byte and other characters outside the allowlist.
		{name: "null byte", value: "jar\x00", wantErr: true},
		{name: "newline", value: "jar\nwar", wantErr: true},
		{name: "space", value: "my app", wantErr: true},
		{name: "semicolon", value: "jar;rm", wantErr: true},
		{name: "colon", value: "C:jar", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMavenCoordinate(tt.value)
			if tt.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tt.value)
			} else {
				assert.NoError(t, err, "expected %q to be accepted", tt.value)
			}
		})
	}
}

func TestExtractResolutionArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "empty", args: nil, want: nil},
		{name: "goals only dropped", args: []string{"clean", "deploy"}, want: nil},
		{name: "profiles kept", args: []string{"deploy", "-Pprod,fast"}, want: []string{"-Pprod,fast"}},
		{name: "properties kept", args: []string{"install", "-DskipTests", "-Drevision=1.2.3"}, want: []string{"-DskipTests", "-Drevision=1.2.3"}},
		{name: "settings with separate value", args: []string{"deploy", "-s", "custom.xml"}, want: []string{"-s", "custom.xml"}},
		{name: "long settings with separate value", args: []string{"deploy", "--settings", "custom.xml"}, want: []string{"--settings", "custom.xml"}},
		{name: "settings attached form", args: []string{"deploy", "--settings=custom.xml"}, want: []string{"--settings=custom.xml"}},
		{name: "settings flag at end without value", args: []string{"deploy", "-s"}, want: []string{"-s"}},
		{name: "mixed", args: []string{"clean", "deploy", "-Pprod", "-s", "s.xml", "-Dfoo=bar", "-X"}, want: []string{"-Pprod", "-s", "s.xml", "-Dfoo=bar"}},
		{name: "alternate POM with separate value", args: []string{"deploy", "-f", "module/pom.xml"}, want: []string{"-f", "module/pom.xml"}},
		{name: "alternate POM attached form", args: []string{"deploy", "--file=module/pom.xml"}, want: []string{"--file=module/pom.xml"}},
		{name: "global settings separate value", args: []string{"deploy", "-gs", "global.xml"}, want: []string{"-gs", "global.xml"}},
		{name: "global settings attached form", args: []string{"deploy", "--global-settings=global.xml"}, want: []string{"--global-settings=global.xml"}},
		{name: "offline flag short", args: []string{"install", "-o"}, want: []string{"-o"}},
		{name: "offline flag long", args: []string{"install", "--offline"}, want: []string{"--offline"}},
		{name: "activate-profiles long form", args: []string{"deploy", "--activate-profiles", "prod,ci"}, want: []string{"--activate-profiles", "prod,ci"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractResolutionArgs(tt.args))
		})
	}
}

func TestSplitModuleId(t *testing.T) {
	tests := []struct {
		name                string
		id                  string
		wantOk              bool
		wantG, wantA, wantV string
	}{
		{name: "valid", id: "com.example:app:1.0.0", wantOk: true, wantG: "com.example", wantA: "app", wantV: "1.0.0"},
		{name: "too few parts", id: "com.example:app", wantOk: false},
		{name: "too many parts", id: "com.example:app:1.0:extra", wantOk: false},
		{name: "path traversal in version", id: "com.example:app:../../etc", wantOk: false},
		{name: "empty part", id: "com.example::1.0.0", wantOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, a, v, ok := splitModuleId(tt.id)
			assert.Equal(t, tt.wantOk, ok)
			if tt.wantOk {
				assert.Equal(t, tt.wantG, g)
				assert.Equal(t, tt.wantA, a)
				assert.Equal(t, tt.wantV, v)
			}
		})
	}
}

func TestSanitizePackaging(t *testing.T) {
	tests := []struct {
		name      string
		packaging string
		want      string
	}{
		{name: "empty defaults to jar", packaging: "", want: "jar"},
		{name: "jar", packaging: "jar", want: "jar"},
		{name: "war", packaging: "war", want: "war"},
		{name: "path traversal falls back", packaging: "../evil", want: "jar"},
		{name: "slash falls back", packaging: "a/b", want: "jar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizePackaging(tt.packaging))
		})
	}
}

// writeFile creates a file (and parents) with the given content, for artifact fixtures.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestCollectModuleArtifacts(t *testing.T) {
	t.Run("jar module gets jar + pom", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pom.xml"), "<project/>")
		writeFile(t, filepath.Join(dir, "target", "app-1.0.0.jar"), "jar-bytes")

		got := collectModuleArtifacts("com.example:app:1.0.0", bidflexpack.ModuleLocation{Dir: dir, Packaging: "jar"})

		require.Len(t, got, 2)
		names := []string{got[0].Name, got[1].Name}
		assert.Contains(t, names, "app-1.0.0.jar")
		assert.Contains(t, names, "app-1.0.0.pom")
		for _, a := range got {
			assert.NotEmpty(t, a.Sha256, "sha256 computed for %s", a.Name)
		}
	})

	t.Run("pom aggregator (no target) yields pom only", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pom.xml"), "<project/>")

		got := collectModuleArtifacts("com.example:parent:1.0.0", bidflexpack.ModuleLocation{Dir: dir, Packaging: "pom"})

		require.Len(t, got, 1)
		assert.Equal(t, "parent-1.0.0.pom", got[0].Name)
		assert.Equal(t, "pom", got[0].Type)
	})

	t.Run("invalid module id returns nothing", func(t *testing.T) {
		assert.Nil(t, collectModuleArtifacts("bad-id", bidflexpack.ModuleLocation{Dir: t.TempDir(), Packaging: "jar"}))
	})

	t.Run("war packaging picks war not jar", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pom.xml"), "<project/>")
		writeFile(t, filepath.Join(dir, "target", "web-2.0.war"), "war-bytes")
		writeFile(t, filepath.Join(dir, "target", "web-2.0.jar"), "intermediate-jar") // must be excluded

		got := collectModuleArtifacts("com.example:web:2.0", bidflexpack.ModuleLocation{Dir: dir, Packaging: "war"})

		var names []string
		for _, a := range got {
			names = append(names, a.Name)
		}
		assert.Contains(t, names, "web-2.0.war")
		assert.NotContains(t, names, "web-2.0.jar")
	})
}

func TestAddDeployedArtifactsToBuildInfo(t *testing.T) {
	libDir := t.TempDir()
	writeFile(t, filepath.Join(libDir, "pom.xml"), "<project/>")
	writeFile(t, filepath.Join(libDir, "target", "lib-1.0.0.jar"), "lib-jar")

	appDir := t.TempDir()
	writeFile(t, filepath.Join(appDir, "pom.xml"), "<project/>")
	writeFile(t, filepath.Join(appDir, "target", "app-1.0.0.jar"), "app-jar")

	buildInfo := &entities.BuildInfo{Modules: []entities.Module{
		{Id: "com.example:lib:1.0.0", Type: entities.Maven},
		{Id: "com.example:app:1.0.0", Type: entities.Maven},
		{Id: "com.example:ghost:1.0.0", Type: entities.Maven}, // no location -> skipped, no artifacts
	}}
	locations := map[string]bidflexpack.ModuleLocation{
		"com.example:lib:1.0.0": {Dir: libDir, Packaging: "jar"},
		"com.example:app:1.0.0": {Dir: appDir, Packaging: "jar"},
	}

	addDeployedArtifactsToBuildInfo(buildInfo, locations)

	// Each located module gets its OWN artifacts (jar + pom), not dumped into Modules[0].
	assert.Len(t, buildInfo.Modules[0].Artifacts, 2)
	assert.Len(t, buildInfo.Modules[1].Artifacts, 2)
	assert.Empty(t, buildInfo.Modules[2].Artifacts, "module without a location gets no artifacts")

	// Verify attribution is per-module (lib's jar on lib, app's jar on app).
	assert.Equal(t, "lib-1.0.0.jar", firstJar(buildInfo.Modules[0].Artifacts))
	assert.Equal(t, "app-1.0.0.jar", firstJar(buildInfo.Modules[1].Artifacts))
}

func firstJar(artifacts []entities.Artifact) string {
	for _, a := range artifacts {
		if a.Type == "jar" {
			return a.Name
		}
	}
	return ""
}

func TestChecksumAql(t *testing.T) {
	t.Run("single checksum scoped to repo", func(t *testing.T) {
		assert.JSONEq(t,
			`{"repo":"my-local","$or":[{"sha256":"abc"}]}`,
			checksumAql("my-local", []string{"abc"}))
	})
	t.Run("multiple checksums", func(t *testing.T) {
		assert.JSONEq(t,
			`{"repo":"my-local","$or":[{"sha256":"abc"},{"sha256":"def"}]}`,
			checksumAql("my-local", []string{"abc", "def"}))
	})
}

func TestExtractRepoKeyFromUrl(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{name: "standard", url: "https://acme.jfrog.io/artifactory/maven-local", want: "maven-local"},
		{name: "trailing slash", url: "https://acme.jfrog.io/artifactory/maven-local/", want: "maven-local"},
		{name: "api/maven form", url: "https://acme.jfrog.io/artifactory/api/maven/maven-virtual", want: "maven-virtual"},
		{name: "host-root single segment", url: "https://artifactory.acme.com/maven-local", want: "maven-local"},
		{name: "empty", url: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRepoKeyFromUrl(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
