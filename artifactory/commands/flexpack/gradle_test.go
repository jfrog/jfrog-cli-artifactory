package flexpack

import (
	"os"
	"path/filepath"
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

func TestExtractRepoKeyFromArtifactoryUrl(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
		wantErr  bool
	}{
		{"Maven API", "http://localhost:8081/artifactory/api/maven/my-repo-local", "my-repo-local", false},
		{"Gradle API", "http://localhost:8081/artifactory/api/gradle/my-repo-local", "my-repo-local", false},
		{"Ivy API", "http://localhost:8081/artifactory/api/ivy/my-repo-local", "my-repo-local", false},
		{"Simple", "http://localhost:8081/artifactory/my-repo-local", "my-repo-local", false},
		{"Simple with trailing slash", "http://localhost:8081/artifactory/my-repo-local/", "my-repo-local", false},
		{"Cloud URL", "https://myorg.jfrog.io/artifactory/my-repo-local", "my-repo-local", false},
		{"Invalid URL", "not-a-url", "", true},
		{"Short path", "http://localhost:8081/artifactory", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRepoKeyFromArtifactoryUrl(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestResolveGradleProperty(t *testing.T) {
	props := map[string]string{
		"version":       "1.0.0",
		"group":         "com.example",
		"repo":          "libs-release",
		"nested.prop":   "nested-value",
		"project.prop":  "ignored-prefix", // props map usually doesn't have project. prefix for keys unless explicitly added
		"escaped":       "${escaped}",
	}

	tests := []struct {
		name     string
		val      string
		expected string
	}{
		{"Simple substitution", "${version}", "1.0.0"},
		{"Partial substitution", "prefix-${version}-suffix", "prefix-1.0.0-suffix"},
		{"Multiple substitution", "${group}:${version}", "com.example:1.0.0"},
		{"With project prefix", "${project.version}", "1.0.0"},
		{"With rootProject prefix", "${rootProject.version}", "1.0.0"},
		{"Simple variable", "$version", "1.0.0"},
		{"Dotted variable", "$nested.prop", "nested-value"},
		{"Unknown property", "${unknown}", "${unknown}"},
		{"Circular reference", "${escaped}", "${escaped}"},
		{"findProperty double quotes", `${findProperty("version")}`, "1.0.0"},
		{"findProperty single quotes", `${findProperty('version')}`, "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveGradleProperty(tt.val, props))
		})
	}
}

func TestParsePropertiesFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{"No properties", []string{"clean", "build"}, map[string]string{}},
		{"-P flag joined", []string{"-Pprop=val"}, map[string]string{"prop": "val"}},
		{"-D flag joined", []string{"-Dprop=val"}, map[string]string{"prop": "val"}},
		{"-P flag separate", []string{"-P", "prop", "val"}, map[string]string{"prop": "val"}},
		{"-D flag separate", []string{"-D", "prop", "val"}, map[string]string{"prop": "val"}},
		{"Quotes", []string{"-Pprop=\"val\""}, map[string]string{"prop": "val"}},
		{"Multiple properties", []string{"-Pprop1=val1", "-Dprop2=val2"}, map[string]string{"prop1": "val1", "prop2": "val2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parsePropertiesFromArgs(tt.args))
		})
	}
}

func TestGetGradleArtifactCoordinates(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	t.Run("Missing file", func(t *testing.T) {
		// Verify error when no build.gradle exists
		_, _, _, err := getGradleArtifactCoordinates(tmpDir)
		assert.Error(t, err)
	})

	t.Run("build.gradle with coordinates", func(t *testing.T) {
		// Verify extraction of group, name, version from Groovy DSL
		content := `
			group = 'com.example'
			version = '1.0.0'
			name = 'my-app'
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		g, a, v, err := getGradleArtifactCoordinates(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "com.example", g)
		assert.Equal(t, "my-app", a)
		assert.Equal(t, "1.0.0", v)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("build.gradle.kts with coordinates", func(t *testing.T) {
		// Verify extraction from Kotlin DSL with rootProject.name
		content := `
			group = "com.example.kts"
			version = "2.0.0"
			rootProject.name = "my-app-kts"
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(content), 0644)
		assert.NoError(t, err)

		g, a, v, err := getGradleArtifactCoordinates(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "com.example.kts", g)
		assert.Equal(t, "my-app-kts", a)
		assert.Equal(t, "2.0.0", v)

		os.Remove(filepath.Join(tmpDir, "build.gradle.kts"))
	})

	t.Run("Fallback artifactId", func(t *testing.T) {
		// Verify fallback to directory name when name is not specified
		content := `
			group = 'com.example'
			version = '1.0.0'
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		_, a, _, err := getGradleArtifactCoordinates(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, filepath.Base(tmpDir), a)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})
}

func TestGetGradleDeployRepository(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	// Setup basic valid dir with empty build.gradle
	err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
	assert.NoError(t, err)

	t.Run("From gradle.properties", func(t *testing.T) {
		// Verify repository detection from gradle.properties
		content := `
			repo = libs-release-local
		`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(content), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-local", repo)

		os.Remove(filepath.Join(tmpDir, "gradle.properties"))
	})

	t.Run("From build.gradle publishing", func(t *testing.T) {
		// Verify repository extraction from publishing block
		content := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/libs-snapshot-local"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot-local", repo)

		// Clean up for next test (overwrite with empty)
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)
	})

	t.Run("Snapshot vs Release priority", func(t *testing.T) {
		// Verify correct repo selection based on version type
		content := `
			snapshotRepo = libs-snapshot-local
			releaseRepo = libs-release-local
		`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(content), 0644)
		assert.NoError(t, err)

		// Expect snapshot repo for SNAPSHOT version
		repo, err := getGradleDeployRepository(tmpDir, "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot-local", repo)

		// Expect release repo for release version
		repo, err = getGradleDeployRepository(tmpDir, "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-local", repo)

		os.Remove(filepath.Join(tmpDir, "gradle.properties"))
	})
}

func TestFindRepoInProperties(t *testing.T) {
	tests := []struct {
		name       string
		props      map[string]string
		isSnapshot bool
		expected   string
		wantErr    bool
	}{
		{
			"Simple Repo Key",
			map[string]string{"repo": "my-repo"},
			false,
			"my-repo",
			false,
		},
		{
			"URL Property",
			map[string]string{"publishUrl": "http://localhost/artifactory/my-repo"},
			false,
			"my-repo",
			false,
		},
		{
			"Select Snapshot",
			map[string]string{
				"releaseRepo":  "libs-release",
				"snapshotRepo": "libs-snapshot",
			},
			true,
			"libs-snapshot",
			false,
		},
		{
			"Select Release",
			map[string]string{
				"releaseRepo":  "libs-release",
				"snapshotRepo": "libs-snapshot",
			},
			false,
			"libs-release",
			false,
		},
		{
			"Ignore booleans",
			map[string]string{"deploy": "true"},
			false,
			"",
			true,
		},
		{
			"Ignore relative paths",
			map[string]string{"url": "./local/dir"},
			false,
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findRepoInProperties(tt.props, tt.isSnapshot)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

// ============================================================================
// NEW EXTENSIVE TEST CASES
// ============================================================================

func TestExtractAllGradleBlocks(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		keyword  string
		expected []string
	}{
		{
			name: "Single publishing block",
			content: `
				publishing {
					repositories {
						maven { url "http://example.com" }
					}
				}
			`,
			keyword:  "publishing",
			expected: []string{"\n\t\t\t\t\trepositories {\n\t\t\t\t\t\tmaven { url \"http://example.com\" }\n\t\t\t\t\t}\n\t\t\t\t"},
		},
		{
			name: "Multiple ext blocks",
			content: `
				ext {
					version = "1.0"
				}
				ext {
					group = "com.example"
				}
			`,
			keyword:  "ext",
			expected: []string{"\n\t\t\t\t\tversion = \"1.0\"\n\t\t\t\t", "\n\t\t\t\t\tgroup = \"com.example\"\n\t\t\t\t"},
		},
		{
			name: "Nested braces",
			content: `
				publishing {
					repositories {
						maven {
							url "http://example.com"
							credentials {
								username "user"
							}
						}
					}
				}
			`,
			keyword:  "publishing",
			expected: []string{"\n\t\t\t\t\trepositories {\n\t\t\t\t\t\tmaven {\n\t\t\t\t\t\t\turl \"http://example.com\"\n\t\t\t\t\t\t\tcredentials {\n\t\t\t\t\t\t\t\tusername \"user\"\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\t\t\t\t"},
		},
		{
			name: "Block with line comment",
			content: `
				ext {
					// this is a comment with { braces }
					version = "1.0"
				}
			`,
			keyword:  "ext",
			expected: []string{"\n\t\t\t\t\t// this is a comment with { braces }\n\t\t\t\t\tversion = \"1.0\"\n\t\t\t\t"},
		},
		{
			name: "Block with block comment",
			content: `
				ext {
					/* block comment with { braces } */
					version = "1.0"
				}
			`,
			keyword:  "ext",
			expected: []string{"\n\t\t\t\t\t/* block comment with { braces } */\n\t\t\t\t\tversion = \"1.0\"\n\t\t\t\t"},
		},
		{
			name: "No matching block",
			content: `
				dependencies {
					implementation "com.example:lib:1.0"
				}
			`,
			keyword:  "publishing",
			expected: nil,
		},
		{
			name:     "Empty content",
			content:  "",
			keyword:  "ext",
			expected: nil,
		},
		{
			name: "Keyword in string should not match",
			content: `
				description = "publishing is great"
				ext {
					version = "1.0"
				}
			`,
			keyword:  "ext",
			expected: []string{"\n\t\t\t\t\tversion = \"1.0\"\n\t\t\t\t"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAllGradleBlocks(tt.content, tt.keyword)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPropertiesFromScript(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name: "ext block properties",
			content: `
				ext {
					artifactoryUrl = "http://localhost:8081"
					repoKey = "libs-release"
				}
			`,
			expected: map[string]string{
				"artifactoryUrl": "http://localhost:8081",
				"repoKey":        "libs-release",
			},
		},
		{
			name: "ext.key syntax",
			content: `
				ext.version = "1.0.0"
				ext.group = "com.example"
			`,
			expected: map[string]string{
				"version": "1.0.0",
				"group":   "com.example",
			},
		},
		{
			name: "project.ext.key syntax",
			content: `
				project.ext.customProp = "customValue"
			`,
			expected: map[string]string{
				"customProp": "customValue",
			},
		},
		{
			name: "Mixed syntax",
			content: `
				ext {
					prop1 = "value1"
				}
				ext.prop2 = "value2"
				project.ext.prop3 = "value3"
			`,
			expected: map[string]string{
				"prop1": "value1",
				"prop2": "value2",
				"prop3": "value3",
			},
		},
		{
			name:     "No ext properties",
			content:  `version = "1.0.0"`,
			expected: map[string]string{},
		},
		{
			name: "Single quotes",
			content: `
				ext {
					prop = 'singleQuoteValue'
				}
			`,
			expected: map[string]string{
				"prop": "singleQuoteValue",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPropertiesFromScript(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindUrlsInGradleScript(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		isKts        bool
		expectedUrls []string
	}{
		{
			name: "Groovy publishing block",
			content: `
				publishing {
					repositories {
						maven {
							url "http://localhost:8081/artifactory/libs-release"
						}
					}
				}
			`,
			isKts:        false,
			expectedUrls: []string{"http://localhost:8081/artifactory/libs-release"},
		},
		{
			name: "Kotlin DSL with url.set",
			content: `
				publishing {
					repositories {
						maven {
							url.set(uri("http://localhost:8081/artifactory/libs-release"))
						}
					}
				}
			`,
			isKts:        true,
			expectedUrls: []string{"http://localhost:8081/artifactory/libs-release"},
		},
		{
			name: "Kotlin DSL with url = uri()",
			content: `
				publishing {
					repositories {
						maven {
							url = uri("http://localhost:8081/artifactory/libs-release")
						}
					}
				}
			`,
			isKts:        true,
			expectedUrls: []string{"http://localhost:8081/artifactory/libs-release"},
		},
		{
			name: "uploadArchives block (legacy)",
			content: `
				uploadArchives {
					repositories {
						mavenDeployer {
							url "http://localhost:8081/artifactory/legacy-repo"
						}
					}
				}
			`,
			isKts:        false,
			expectedUrls: []string{"http://localhost:8081/artifactory/legacy-repo"},
		},
		{
			name: "dependencyResolutionManagement block",
			content: `
				dependencyResolutionManagement {
					repositories {
						maven {
							url "http://localhost:8081/artifactory/gradle-plugins"
						}
					}
				}
			`,
			isKts:        false,
			expectedUrls: []string{"http://localhost:8081/artifactory/gradle-plugins"},
		},
		{
			name: "Multiple repositories",
			content: `
				publishing {
					repositories {
						maven {
							url "http://localhost:8081/artifactory/libs-snapshot"
						}
						maven {
							url "http://localhost:8081/artifactory/libs-release"
						}
					}
				}
			`,
			isKts:        false,
			expectedUrls: []string{"http://localhost:8081/artifactory/libs-snapshot", "http://localhost:8081/artifactory/libs-release"},
		},
		{
			name: "URL with single quotes",
			content: `
				publishing {
					repositories {
						maven {
							url 'http://localhost:8081/artifactory/libs-release'
						}
					}
				}
			`,
			isKts:        false,
			expectedUrls: []string{"http://localhost:8081/artifactory/libs-release"},
		},
		{
			name: "No publishing block",
			content: `
				dependencies {
					implementation "com.example:lib:1.0"
				}
			`,
			isKts:        false,
			expectedUrls: nil,
		},
		{
			name: "Empty publishing repositories",
			content: `
				publishing {
					repositories {
					}
				}
			`,
			isKts:        false,
			expectedUrls: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := findUrlsInGradleScript([]byte(tt.content), tt.isKts)
			var urls []string
			for _, m := range matches {
				if len(m) > 1 {
					urls = append(urls, string(m[1]))
				}
			}
			assert.Equal(t, tt.expectedUrls, urls)
		})
	}
}

func TestReadPropertiesFile(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name: "Standard properties with equals",
			content: `
key1=value1
key2=value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with colon separator",
			content: `
key1:value1
key2:value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with spaces around equals",
			content: `
key1 = value1
key2  =  value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with comments",
			content: `
# This is a comment
key1=value1
# Another comment
key2=value2
`,
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "Properties with quoted values",
			content: `
key1="quoted value"
key2='single quoted'
`,
			expected: map[string]string{"key1": "quoted value", "key2": "single quoted"},
		},
		{
			name: "URL values",
			content: `
artifactoryUrl=http://localhost:8081/artifactory
repoKey=libs-release-local
`,
			expected: map[string]string{
				"artifactoryUrl": "http://localhost:8081/artifactory",
				"repoKey":        "libs-release-local",
			},
		},
		{
			name:     "Empty file",
			content:  "",
			expected: map[string]string{},
		},
		{
			name: "Values with special characters",
			content: `
password=p@ss=word!123
path=/usr/local/bin
`,
			expected: map[string]string{
				"password": "p@ss=word!123",
				"path":     "/usr/local/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			propsPath := filepath.Join(tmpDir, "test.properties")
			err := os.WriteFile(propsPath, []byte(tt.content), 0644)
			assert.NoError(t, err)

			result := readPropertiesFile(propsPath)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("Non-existent file", func(t *testing.T) {
		// Verify graceful handling of missing file
		result := readPropertiesFile(filepath.Join(tmpDir, "nonexistent.properties"))
		assert.Equal(t, map[string]string{}, result)
	})
}

func TestCollectAppliedScripts(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	t.Run("Groovy apply from", func(t *testing.T) {
		// Verify Groovy apply from: syntax
		content := `apply from: "gradle/publish.gradle"`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Kotlin apply from", func(t *testing.T) {
		// Verify Kotlin apply(from = ...) syntax
		content := `apply(from = "gradle/publish.gradle.kts")`
		result := collectAppliedScripts([]byte(content), true, map[string]string{}, filepath.Join(tmpDir, "build.gradle.kts"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Multiple applies", func(t *testing.T) {
		// Verify multiple apply statements are collected
		content := "apply from: \"gradle/a.gradle\"\napply from: \"gradle/b.gradle\""
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 2, len(result))
	})

	t.Run("Apply with property resolution", func(t *testing.T) {
		// Verify property substitution in paths
		content := `apply from: "${rootDir}/gradle/publish.gradle"`
		props := map[string]string{"rootDir": tmpDir}
		result := collectAppliedScripts([]byte(content), false, props, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Remote script should be skipped", func(t *testing.T) {
		// Verify remote URLs are filtered out
		content := `apply from: "https://example.com/script.gradle"`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 0, len(result))
	})

	t.Run("No apply statements", func(t *testing.T) {
		// Verify empty result when no apply statements
		content := `dependencies { }`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 0, len(result))
	})
}

func TestFindRepositoryKeyFromMatches(t *testing.T) {
	tests := []struct {
		name       string
		repoUrls   []string
		sourceName string
		isSnapshot bool
		expected   string
		wantErr    bool
	}{
		{
			name:       "Single URL",
			repoUrls:   []string{"http://localhost:8081/artifactory/libs-release"},
			sourceName: "test",
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name:       "Snapshot URL for snapshot version",
			repoUrls:   []string{"http://localhost:8081/artifactory/libs-snapshot", "http://localhost:8081/artifactory/libs-release"},
			sourceName: "test",
			isSnapshot: true,
			expected:   "libs-snapshot",
			wantErr:    false,
		},
		{
			name:       "Release URL for release version",
			repoUrls:   []string{"http://localhost:8081/artifactory/libs-snapshot", "http://localhost:8081/artifactory/libs-release"},
			sourceName: "test",
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name:       "Fallback to general when no snapshot match",
			repoUrls:   []string{"http://localhost:8081/artifactory/libs-local"},
			sourceName: "test",
			isSnapshot: true,
			expected:   "libs-local",
			wantErr:    false,
		},
		{
			name:       "Empty URL list",
			repoUrls:   []string{},
			sourceName: "test",
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name:       "Invalid URLs only",
			repoUrls:   []string{"not-a-url"},
			sourceName: "test",
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findRepositoryKeyFromMatches(tt.repoUrls, tt.sourceName, tt.isSnapshot)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFindRepoInGradleScript(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		isKts      bool
		props      map[string]string
		isSnapshot bool
		expected   string
		wantErr    bool
	}{
		{
			name: "Simple Groovy publishing",
			content: `
				publishing {
					repositories {
						maven {
							url "http://localhost:8081/artifactory/libs-release"
						}
					}
				}
			`,
			isKts:      false,
			props:      map[string]string{},
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name: "Kotlin DSL publishing",
			content: `
				publishing {
					repositories {
						maven {
							url = uri("http://localhost:8081/artifactory/libs-release")
						}
					}
				}
			`,
			isKts:      true,
			props:      map[string]string{},
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name: "With property substitution",
			content: `
				publishing {
					repositories {
						maven {
							url "${artifactoryUrl}/libs-release"
						}
					}
				}
			`,
			isKts:      false,
			props:      map[string]string{"artifactoryUrl": "http://localhost:8081/artifactory"},
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name: "URL with ext property",
			content: `
				ext {
					artifactoryUrl = "http://localhost:8081/artifactory"
				}
				publishing {
					repositories {
						maven {
							url "${artifactoryUrl}/libs-snapshot"
						}
					}
				}
			`,
			isKts:      false,
			props:      map[string]string{},
			isSnapshot: true,
			expected:   "libs-snapshot",
			wantErr:    false,
		},
		{
			name: "No publishing block",
			content: `
				dependencies {
					implementation "com.example:lib:1.0"
				}
			`,
			isKts:      false,
			props:      map[string]string{},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name: "Unresolved property should fail",
			content: `
				publishing {
					repositories {
						maven {
							url "${unknownProp}/libs-release"
						}
					}
				}
			`,
			isKts:      false,
			props:      map[string]string{},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findRepoInGradleScript([]byte(tt.content), tt.isKts, tt.props, tt.isSnapshot, "test.gradle")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetGradleDeployRepositoryExtended(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	t.Run("Empty working directory should error", func(t *testing.T) {
		// Verify error for empty path
		_, err := getGradleDeployRepository("", "1.0.0")
		assert.Error(t, err)
	})

	t.Run("Invalid working directory should error", func(t *testing.T) {
		// Verify error for non-existent path
		_, err := getGradleDeployRepository("/nonexistent/path/xyz", "1.0.0")
		assert.Error(t, err)
	})

	t.Run("From settings.gradle", func(t *testing.T) {
		// Verify repo detection from settings.gradle dependencyResolutionManagement
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		settingsContent := `
			dependencyResolutionManagement {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/gradle-plugins"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(tmpDir, "settings.gradle"), []byte(settingsContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "gradle-plugins", repo)

		os.Remove(filepath.Join(tmpDir, "settings.gradle"))
		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("From build.gradle.kts", func(t *testing.T) {
		// Verify Kotlin DSL publishing block parsing
		buildKtsContent := `
			publishing {
				repositories {
					maven {
						url = uri("http://localhost:8081/artifactory/api/maven/libs-release-kts")
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildKtsContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-kts", repo)

		os.Remove(filepath.Join(tmpDir, "build.gradle.kts"))
	})

	t.Run("Property with artifactory keyword", func(t *testing.T) {
		// Verify detection via artifactory-related property name
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		propsContent := `
artifactoryDeployRepo=custom-deploy-repo
`
		err = os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(propsContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "custom-deploy-repo", repo)

		os.Remove(filepath.Join(tmpDir, "gradle.properties"))
		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("No repository found should error", func(t *testing.T) {
		// Verify error when no repository configuration exists
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		_, err = getGradleDeployRepository(tmpDir, "1.0.0")
		assert.Error(t, err)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})
}

func TestGetGradleArtifactCoordinatesExtended(t *testing.T) {
	// Use t.TempDir() for automatic cleanup
	tmpDir := t.TempDir()

	t.Run("Coordinates with equals and space", func(t *testing.T) {
		// Verify parsing with spaces around equals sign
		content := `
group = "com.example.space"
version = "3.0.0"
name = "spaced-app"
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		g, a, v, err := getGradleArtifactCoordinates(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "com.example.space", g)
		assert.Equal(t, "spaced-app", a)
		assert.Equal(t, "3.0.0", v)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Missing group should error", func(t *testing.T) {
		// Verify error when group is not specified
		content := `
version = "1.0.0"
name = "app"
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		_, _, _, err = getGradleArtifactCoordinates(tmpDir)
		assert.Error(t, err)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Missing version should error", func(t *testing.T) {
		// Verify error when version is not specified
		content := `
group = "com.example"
name = "app"
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(content), 0644)
		assert.NoError(t, err)

		_, _, _, err = getGradleArtifactCoordinates(tmpDir)
		assert.Error(t, err)

		os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Kotlin DSL without name uses directory", func(t *testing.T) {
		// Verify directory name fallback for artifactId in Kotlin DSL
		content := `
group = "com.kotlin"
version = "1.0.0"
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(content), 0644)
		assert.NoError(t, err)

		g, a, v, err := getGradleArtifactCoordinates(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "com.kotlin", g)
		assert.Equal(t, filepath.Base(tmpDir), a)
		assert.Equal(t, "1.0.0", v)

		os.Remove(filepath.Join(tmpDir, "build.gradle.kts"))
	})
}

func TestParsePropertiesFromArgsExtended(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name:     "Single quotes",
			args:     []string{"-Pprop='single'"},
			expected: map[string]string{"prop": "single"},
		},
		{
			name:     "URL value",
			args:     []string{"-PartifactoryUrl=http://localhost:8081"},
			expected: map[string]string{"artifactoryUrl": "http://localhost:8081"},
		},
		{
			name:     "Empty value ignored",
			args:     []string{"-Pprop="},
			expected: map[string]string{},
		},
		{
			name:     "Flag followed by another flag",
			args:     []string{"-P", "-D", "something"},
			expected: map[string]string{},
		},
		{
			name:     "Mixed with gradle tasks",
			args:     []string{"clean", "-Prepo=myrepo", "publish", "-Dversion=1.0"},
			expected: map[string]string{"repo": "myrepo", "version": "1.0"},
		},
		{
			name:     "Value with equals sign",
			args:     []string{"-Pconnection=jdbc:mysql://host:3306?param=value"},
			expected: map[string]string{"connection": "jdbc:mysql://host:3306?param=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parsePropertiesFromArgs(tt.args))
		})
	}
}

func TestExtractRepoKeyFromArtifactoryUrlExtended(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
		wantErr  bool
	}{
		{
			name:     "URL with port",
			url:      "http://localhost:8081/artifactory/libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "HTTPS URL",
			url:      "https://artifactory.example.com/artifactory/libs-snapshot",
			expected: "libs-snapshot",
			wantErr:  false,
		},
		{
			name:     "Repo with dashes and underscores",
			url:      "http://localhost/artifactory/my-custom_repo-local",
			expected: "my-custom_repo-local",
			wantErr:  false,
		},
		{
			name:     "Nested path after artifactory",
			url:      "http://localhost/artifactory/api/maven/gradle-plugins-release",
			expected: "gradle-plugins-release",
			wantErr:  false,
		},
		{
			name:     "Empty string",
			url:      "",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Whitespace URL",
			url:      "   http://localhost/artifactory/libs-release   ",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "URL with query params",
			url:      "http://localhost/artifactory/libs-release?param=value",
			expected: "libs-release",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRepoKeyFromArtifactoryUrl(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestResolveGradlePropertyExtended(t *testing.T) {
	props := map[string]string{
		"host":          "localhost",
		"port":          "8081",
		"artifactory":   "http://localhost:8081/artifactory",
		"repoKey":       "libs-release",
		"nested.value":  "nestedResult",
		"with.dots.key": "dottedValue",
	}

	tests := []struct {
		name     string
		val      string
		expected string
	}{
		{
			name:     "Empty string",
			val:      "",
			expected: "",
		},
		{
			name:     "No substitution needed",
			val:      "plain-value",
			expected: "plain-value",
		},
		{
			name:     "Build URL from parts",
			val:      "http://${host}:${port}/artifactory/${repoKey}",
			expected: "http://localhost:8081/artifactory/libs-release",
		},
		{
			name:     "Complex nested dots",
			val:      "$with.dots.key",
			expected: "dottedValue",
		},
		{
			name:     "Mixed syntax",
			val:      "${host}:$port",
			expected: "localhost:8081",
		},
		{
			name:     "Partial match with suffix",
			val:      "$host.example.com",
			expected: "localhost.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveGradleProperty(tt.val, props))
		})
	}
}

func TestFindRepoInPropertiesExtended(t *testing.T) {
	tests := []struct {
		name       string
		props      map[string]string
		isSnapshot bool
		expected   string
		wantErr    bool
	}{
		{
			name: "Multiple repo properties - first by alpha order",
			props: map[string]string{
				"aRepo": "first-repo",
				"zRepo": "last-repo",
			},
			isSnapshot: false,
			expected:   "first-repo",
			wantErr:    false,
		},
		{
			name: "Properties with deploy keyword",
			props: map[string]string{
				"deployTarget": "deploy-repo-local",
			},
			isSnapshot: false,
			expected:   "deploy-repo-local",
			wantErr:    false,
		},
		{
			name: "Absolute path URL",
			props: map[string]string{
				"publishUrl": "/artifactory/local-repo",
			},
			isSnapshot: false,
			expected:   "local-repo",
			wantErr:    false,
		},
		{
			name: "Snapshot version prefers snapshot repo",
			props: map[string]string{
				"repo":              "general-repo",
				"snapshotPublishUrl": "http://localhost/artifactory/snapshot-repo",
			},
			isSnapshot: true,
			expected:   "snapshot-repo",
			wantErr:    false,
		},
		{
			name: "Release version prefers release repo",
			props: map[string]string{
				"repo":             "general-repo",
				"releasePublishUrl": "http://localhost/artifactory/release-repo",
			},
			isSnapshot: false,
			expected:   "release-repo",
			wantErr:    false,
		},
		{
			name: "Ignore parent relative paths",
			props: map[string]string{
				"url": "../parent/dir",
			},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name: "Ignore values with colons (non-URL)",
			props: map[string]string{
				"repo": "com.example:lib:1.0",
			},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name:       "Empty properties",
			props:      map[string]string{},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findRepoInProperties(tt.props, tt.isSnapshot)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestIsDelimiter(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{'{', true},
		{'}', true},
		{'(', true},
		{')', true},
		{';', true},
		{',', true},
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'1', false},
		{'_', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			assert.Equal(t, tt.expected, isDelimiter(tt.char))
		})
	}
}

func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'0', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			assert.Equal(t, tt.expected, isWhitespace(tt.char))
		})
	}
}

func TestWasPublishCommandExtended(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []string
		expected bool
	}{
		// NOTE: publishAllPublicationsToMavenRepository is not currently detected
		// The implementation only matches "publish" exactly or "publishTo*" prefix
		{"publishAllPublicationsToMavenRepository (not detected)", []string{"publishAllPublicationsToMavenRepository"}, false},
		{"publishToSonatype", []string{"publishToSonatype"}, true},
		{"assemble then publish", []string{"assemble", "check", "publish"}, true},
		{"deeply nested project publish", []string{":a:b:c:d:publish"}, true},
		{"publishMavenPublicationToMavenLocal", []string{"publishMavenPublicationToMavenLocal"}, false},
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

