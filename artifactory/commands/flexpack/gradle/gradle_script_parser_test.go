package flexpack

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractAllGradleBlocks(t *testing.T) {
	// These tests validate functional behavior: that blocks are correctly identified
	// and their content is extracted. We use Contains checks rather than exact string
	// matching to avoid brittle tests tied to whitespace implementation details.

	t.Run("Single publishing block", func(t *testing.T) {
		content := `
publishing {
    repositories {
        maven { url "http://example.com" }
    }
}`
		result := extractAllGradleBlocks(content, "publishing")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "repositories")
		assert.Contains(t, result[0], "maven")
		assert.Contains(t, result[0], "http://example.com")
	})

	t.Run("Multiple ext blocks", func(t *testing.T) {
		content := `
ext {
    version = "1.0"
}
ext {
    group = "com.example"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 2, len(result))
		assert.Contains(t, result[0], "version")
		assert.Contains(t, result[1], "group")
	})

	t.Run("Nested braces with credentials", func(t *testing.T) {
		// Real-world pattern: publishing with credentials
		content := `
publishing {
    repositories {
        maven {
            url "http://artifactory.example.com/libs-release"
            credentials {
                username = artifactoryUser
                password = artifactoryPassword
            }
        }
    }
}`
		result := extractAllGradleBlocks(content, "publishing")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "credentials")
		assert.Contains(t, result[0], "username")
		assert.Contains(t, result[0], "password")
	})

	t.Run("Block with comments containing braces", func(t *testing.T) {
		// Comments with braces should not break parsing
		content := `
ext {
    // Comment with { braces } should be ignored
    version = "1.0"
    /* Multi-line
       comment with { braces }
    */
    group = "com.example"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "version")
		assert.Contains(t, result[0], "group")
	})

	t.Run("No matching block returns nil", func(t *testing.T) {
		content := `
dependencies {
    implementation "com.example:lib:1.0"
}`
		result := extractAllGradleBlocks(content, "publishing")
		assert.Nil(t, result)
	})

	t.Run("Empty content returns nil", func(t *testing.T) {
		result := extractAllGradleBlocks("", "ext")
		assert.Nil(t, result)
	})

	t.Run("Keyword in string literal should not match", func(t *testing.T) {
		content := `
description = "This describes publishing behavior"
ext {
    version = "1.0"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "version")
	})

	t.Run("allprojects block with nested publishing", func(t *testing.T) {
		// Real-world pattern: Multi-project builds
		content := `
allprojects {
    apply plugin: 'java'
    apply plugin: 'maven-publish'

    group = 'org.example'
    version = '1.0.0'
}

subprojects {
    publishing {
        repositories {
            maven {
                url "https://repo.example.com/releases"
            }
        }
    }
}`
		// Extract subprojects block
		result := extractAllGradleBlocks(content, "subprojects")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "publishing")
		assert.Contains(t, result[0], "maven")
	})

	t.Run("buildscript repositories block", func(t *testing.T) {
		// Real-world pattern: buildscript for plugins
		content := `
buildscript {
    repositories {
        mavenCentral()
        gradlePluginPortal()
    }
    dependencies {
        classpath 'com.example:gradle-plugin:1.0'
    }
}`
		result := extractAllGradleBlocks(content, "buildscript")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "repositories")
		assert.Contains(t, result[0], "mavenCentral")
	})
}

func TestExtractAllGradleBlocksEdgeCases(t *testing.T) {
	t.Run("Keyword at start of content", func(t *testing.T) {
		content := `ext {
	version = "1.0"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "version")
	})

	t.Run("Multiple nested levels", func(t *testing.T) {
		content := `publishing {
	repositories {
		maven {
			url "http://example.com"
			authentication {
				basic(BasicAuthentication) {
					credentials {
						username "user"
					}
				}
			}
		}
	}
}`
		result := extractAllGradleBlocks(content, "publishing")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "authentication")
	})

	t.Run("Braces in strings should be ignored", func(t *testing.T) {
		content := `ext {
	pattern = "test{a,b,c}"
	version = "1.0"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "pattern")
		assert.Contains(t, result[0], "version")
	})

	t.Run("Mixed comments and code", func(t *testing.T) {
		content := `ext {
	// comment { with brace
	version = "1.0"
	/* another { comment */
	group = "com.example"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "version")
		assert.Contains(t, result[0], "group")
	})
}

func TestExtractAllGradleBlocksMoreEdgeCases(t *testing.T) {
	t.Run("Keyword as part of longer word should not match", func(t *testing.T) {
		content := `
subprojects {
	extProperty = "value"
}`
		// "ext" should not match because it's part of "extProperty"
		result := extractAllGradleBlocks(content, "ext")
		assert.Nil(t, result)
	})

	t.Run("Keyword followed by parenthesis instead of brace", func(t *testing.T) {
		content := `
publishing()
ext {
	version = "1.0"
}`
		// "publishing" is followed by () not {}, so should not match
		// Only "ext" block should be found
		result := extractAllGradleBlocks(content, "publishing")
		assert.Nil(t, result)
	})

	t.Run("Block with escaped quotes", func(t *testing.T) {
		content := `ext {
	pattern = "test\"value"
	version = "1.0"
}`
		result := extractAllGradleBlocks(content, "ext")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "pattern")
		assert.Contains(t, result[0], "version")
	})

	t.Run("Deeply nested blocks", func(t *testing.T) {
		content := `
outer {
	level1 {
		level2 {
			level3 {
				value = "deep"
			}
		}
	}
}`
		result := extractAllGradleBlocks(content, "outer")
		assert.Equal(t, 1, len(result))
		assert.Contains(t, result[0], "level1")
		assert.Contains(t, result[0], "level2")
		assert.Contains(t, result[0], "level3")
		assert.Contains(t, result[0], "deep")
	})

	t.Run("Empty block - filtered out by design", func(t *testing.T) {
		content := `ext {}`
		result := extractAllGradleBlocks(content, "ext")
		// Empty blocks are filtered out by design
		assert.Nil(t, result)
	})

	t.Run("Block with only whitespace - kept", func(t *testing.T) {
		content := `ext {
   
}`
		result := extractAllGradleBlocks(content, "ext")
		// Block with whitespace is kept (whitespace is not empty string)
		assert.Equal(t, 1, len(result))
	})
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

func TestFindUrlsInGradleScriptEdgeCases(t *testing.T) {
	t.Run("URL with property placeholder", func(t *testing.T) {
		content := `
			publishing {
				repositories {
					maven {
						url "${artifactoryUrl}/libs-release"
					}
				}
			}
		`
		matches := findUrlsInGradleScript([]byte(content), false)
		assert.Equal(t, 1, len(matches))
		assert.Equal(t, "${artifactoryUrl}/libs-release", string(matches[0][1]))
	})

	t.Run("Groovy with url = syntax", func(t *testing.T) {
		content := `
			publishing {
				repositories {
					maven {
						url = "http://localhost/artifactory/repo"
					}
				}
			}
		`
		matches := findUrlsInGradleScript([]byte(content), false)
		assert.Equal(t, 1, len(matches))
	})

	t.Run("Mixed quote styles in same file", func(t *testing.T) {
		content := `
			publishing {
				repositories {
					maven {
						url "http://localhost/repo1"
					}
					maven {
						url 'http://localhost/repo2'
					}
				}
			}
		`
		matches := findUrlsInGradleScript([]byte(content), false)
		assert.Equal(t, 2, len(matches))
	})
}

func TestCollectAppliedScripts(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Groovy apply from", func(t *testing.T) {
		content := `apply from: "gradle/publish.gradle"`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Kotlin apply from", func(t *testing.T) {
		content := `apply(from = "gradle/publish.gradle.kts")`
		result := collectAppliedScripts([]byte(content), true, map[string]string{}, filepath.Join(tmpDir, "build.gradle.kts"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Multiple applies", func(t *testing.T) {
		content := "apply from: \"gradle/a.gradle\"\napply from: \"gradle/b.gradle\""
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 2, len(result))
	})

	t.Run("Apply with property resolution", func(t *testing.T) {
		content := `apply from: "${rootDir}/gradle/publish.gradle"`
		props := map[string]string{"rootDir": tmpDir}
		result := collectAppliedScripts([]byte(content), false, props, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 1, len(result))
	})

	t.Run("Remote script should be skipped", func(t *testing.T) {
		content := `apply from: "https://example.com/script.gradle"`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 0, len(result))
	})

	t.Run("No apply statements", func(t *testing.T) {
		content := `dependencies { }`
		result := collectAppliedScripts([]byte(content), false, map[string]string{}, filepath.Join(tmpDir, "build.gradle"))
		assert.Equal(t, 0, len(result))
	})
}

func TestScriptParserRegexPatterns(t *testing.T) {
	t.Run("URL regex patterns", func(t *testing.T) {
		groovyTestCases := []struct {
			input    string
			expected string
		}{
			{`url "http://example.com"`, "http://example.com"},
			{`url 'http://example.com'`, "http://example.com"},
			{`url = "http://example.com"`, "http://example.com"},
			{`url: "http://example.com"`, "http://example.com"},
		}

		for _, tc := range groovyTestCases {
			matches := urlGroovyRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "URL mismatch for: %s", tc.input)
			}
		}

		ktsTestCases := []struct {
			input    string
			expected string
		}{
			{`url("http://example.com")`, "http://example.com"},
			{`url = uri("http://example.com")`, "http://example.com"},
			{`url.set(uri("http://example.com"))`, "http://example.com"},
		}

		for _, tc := range ktsTestCases {
			matches := urlKtsRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "URL mismatch for: %s", tc.input)
			}
		}
	})
}

func TestApplyFromRegexPatterns(t *testing.T) {
	t.Run("Groovy apply from patterns", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{`apply from: "script.gradle"`, "script.gradle"},
			{`apply from: 'script.gradle'`, "script.gradle"},
			{`apply from: "gradle/publish.gradle"`, "gradle/publish.gradle"},
			{`apply from: "${rootDir}/script.gradle"`, "${rootDir}/script.gradle"},
		}

		for _, tc := range testCases {
			matches := applyFromGroovyRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "Path mismatch for: %s", tc.input)
			}
		}
	})

	t.Run("Kotlin apply from patterns", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{`apply(from = "script.gradle.kts")`, "script.gradle.kts"},
			{`apply(from = 'script.gradle.kts')`, "script.gradle.kts"},
			{`apply(from = "gradle/publish.gradle.kts")`, "gradle/publish.gradle.kts"},
		}

		for _, tc := range testCases {
			matches := applyFromKtsRe.FindStringSubmatch(tc.input)
			assert.NotNil(t, matches, "Expected match for: %s", tc.input)
			if matches != nil {
				assert.Equal(t, tc.expected, matches[1], "Path mismatch for: %s", tc.input)
			}
		}
	})
}
