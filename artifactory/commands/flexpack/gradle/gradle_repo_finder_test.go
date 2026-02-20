package flexpack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGradleDeployRepository(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup basic valid dir with empty build.gradle
	err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
	assert.NoError(t, err)

	t.Run("From gradle.properties", func(t *testing.T) {
		content := `
			repo = libs-release-local
		`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(content), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-local", repo)

		_ = os.Remove(filepath.Join(tmpDir, "gradle.properties"))
	})

	t.Run("From build.gradle publishing", func(t *testing.T) {
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

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot-local", repo)

		// Clean up for next test
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)
	})

	t.Run("Snapshot vs Release priority", func(t *testing.T) {
		content := `
			snapshotRepo = libs-snapshot-local
			releaseRepo = libs-release-local
		`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(content), 0644)
		assert.NoError(t, err)

		// Expect snapshot repo for SNAPSHOT version
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot-local", repo)

		// Expect release repo for release version
		repo, err = getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-local", repo)

		_ = os.Remove(filepath.Join(tmpDir, "gradle.properties"))
	})
}

func TestGetGradleDeployRepositoryExtended(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Empty working directory should error", func(t *testing.T) {
		_, err := getGradleDeployRepository("", "", "1.0.0")
		assert.Error(t, err)
	})

	t.Run("Invalid working directory should error", func(t *testing.T) {
		_, err := getGradleDeployRepository("/nonexistent/path/xyz", "", "1.0.0")
		assert.Error(t, err)
	})

	t.Run("From settings.gradle", func(t *testing.T) {
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

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "gradle-plugins", repo)

		_ = os.Remove(filepath.Join(tmpDir, "settings.gradle"))
		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("From build.gradle.kts", func(t *testing.T) {
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

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release-kts", repo)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle.kts"))
	})

	t.Run("Property with artifactory keyword", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		propsContent := `
artifactoryDeployRepo=custom-deploy-repo
`
		err = os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(propsContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "custom-deploy-repo", repo)

		_ = os.Remove(filepath.Join(tmpDir, "gradle.properties"))
		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("No repository found should error", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		_, err = getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.Error(t, err)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})
}

func TestGetGradleDeployRepositoryComplexScenarios(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Property substitution in build.gradle", func(t *testing.T) {
		buildContent := `
			ext {
				artifactoryUrl = "http://localhost:8081/artifactory"
			}
			publishing {
				repositories {
					maven {
						url "${artifactoryUrl}/libs-release"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Multiple publishing repositories - snapshot version", func(t *testing.T) {
		buildContent := `
			publishing {
				repositories {
					maven {
						name = "snapshot"
						url "http://localhost:8081/artifactory/libs-snapshot"
					}
					maven {
						name = "release"
						url "http://localhost:8081/artifactory/libs-release"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot", repo)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Multiple publishing repositories - release version", func(t *testing.T) {
		buildContent := `
			publishing {
				repositories {
					maven {
						name = "snapshot"
						url "http://localhost:8081/artifactory/libs-snapshot"
					}
					maven {
						name = "release"
						url "http://localhost:8081/artifactory/libs-release"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	})

	t.Run("Kotlin DSL with url.set syntax", func(t *testing.T) {
		buildContent := `
			publishing {
				repositories {
					maven {
						url.set(uri("http://localhost:8081/artifactory/kotlin-repo"))
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "kotlin-repo", repo)

		_ = os.Remove(filepath.Join(tmpDir, "build.gradle.kts"))
	})
}

func TestGetGradleDeployRepositoryVersionDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup with both snapshot and release repos
	propsContent := `
snapshotRepo=snapshot-repo
releaseRepo=release-repo
`
	err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(""), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(propsContent), 0644)
	assert.NoError(t, err)
	defer func() {
		_ = os.Remove(filepath.Join(tmpDir, "gradle.properties"))
		_ = os.Remove(filepath.Join(tmpDir, "build.gradle"))
	}()

	t.Run("Version ending with -SNAPSHOT", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "snapshot-repo", repo)
	})

	t.Run("Version with lowercase snapshot", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-snapshot")
		assert.NoError(t, err)
		assert.Equal(t, "snapshot-repo", repo)
	})

	t.Run("Version with snapshot in middle", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT-1")
		assert.NoError(t, err)
		assert.Equal(t, "snapshot-repo", repo)
	})

	t.Run("Release version", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "release-repo", repo)
	})

	t.Run("Version with RC suffix", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-RC1")
		assert.NoError(t, err)
		assert.Equal(t, "release-repo", repo)
	})

	t.Run("Version with RELEASE suffix", func(t *testing.T) {
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0.RELEASE")
		assert.NoError(t, err)
		assert.Equal(t, "release-repo", repo)
	})
}

func TestGradleIntegrationScenarios(t *testing.T) {
	t.Run("Typical Groovy project structure", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create gradle.properties
		propsContent := `
group=com.example
version=1.0.0
artifactoryUrl=http://localhost:8081/artifactory
`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(propsContent), 0644)
		assert.NoError(t, err)

		// Create build.gradle
		buildContent := `
plugins {
    id 'java'
    id 'maven-publish'
}

group = 'com.example'
version = '1.0.0'

publishing {
    repositories {
        maven {
            url "${artifactoryUrl}/libs-release"
        }
    }
}
`
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)
	})

	t.Run("Typical Kotlin DSL project structure", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create gradle.properties
		propsContent := `
group=com.example
version=2.0.0
`
		err := os.WriteFile(filepath.Join(tmpDir, "gradle.properties"), []byte(propsContent), 0644)
		assert.NoError(t, err)

		// Create build.gradle.kts
		buildContent := `
plugins {
    kotlin("jvm") version "1.9.0"
    id("maven-publish")
}

group = "com.example"
version = "2.0.0"

publishing {
    repositories {
        maven {
            url = uri("http://localhost:8081/artifactory/kotlin-libs")
        }
    }
}
`
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle.kts"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "2.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "kotlin-libs", repo)
	})

	t.Run("Project with separate snapshot and release repos in publishing block", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Note: The parser does static text analysis, not Groovy evaluation.
		// So we use a common pattern of defining separate maven blocks with named repos
		buildContent := `
publishing {
    repositories {
        maven {
            name = "snapshot"
            url "http://localhost/artifactory/libs-snapshot"
        }
        maven {
            name = "release"
            url "http://localhost/artifactory/libs-release"
        }
    }
}
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		// For SNAPSHOT version, should pick snapshot repo based on URL naming
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0-SNAPSHOT")
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot", repo)

		// For release version, should pick release repo based on URL naming
		repo, err = getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)
	})

	t.Run("Legacy uploadArchives configuration", func(t *testing.T) {
		tmpDir := t.TempDir()

		buildContent := `
uploadArchives {
    repositories {
        mavenDeployer {
            url "http://localhost:8081/artifactory/legacy-maven"
        }
    }
}
`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "legacy-maven", repo)
	})
}

func TestErrorHandling(t *testing.T) {
	t.Run("getGradleDeployRepository with file as working directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "somefile.txt")
		err := os.WriteFile(filePath, []byte("content"), 0644)
		assert.NoError(t, err)

		_, err = getGradleDeployRepository(filePath, "", "1.0.0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("getGradleDeployRepository with permission denied", func(t *testing.T) {
		// Skip on systems where we can't test permission errors
		if os.Getuid() == 0 {
			t.Skip("Skipping permission test when running as root")
		}

		tmpDir := t.TempDir()
		restrictedDir := filepath.Join(tmpDir, "restricted")
		err := os.Mkdir(restrictedDir, 0000)
		if err != nil {
			t.Skip("Could not create restricted directory")
		}
		defer func() { _ = os.Chmod(restrictedDir, 0755) }()

		_, err = getGradleDeployRepository(restrictedDir, "", "1.0.0")
		// Should error because we can't read files in the directory
		assert.Error(t, err)
	})
}

func TestGradleDeployRepositoryWithInitScripts(t *testing.T) {
	t.Run("Repository from init script via GRADLE_USER_HOME", func(t *testing.T) {
		tmpDir := t.TempDir()
		gradleHome := filepath.Join(tmpDir, "gradle-home")
		err := os.Mkdir(gradleHome, 0755)
		assert.NoError(t, err)

		// Create project directory
		projectDir := filepath.Join(tmpDir, "project")
		err = os.Mkdir(projectDir, 0755)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(projectDir, "build.gradle"), []byte(""), 0644)
		assert.NoError(t, err)

		// Create init script
		initContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/init-script-repo"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(gradleHome, "init.gradle"), []byte(initContent), 0644)
		assert.NoError(t, err)

		// Set GRADLE_USER_HOME
		originalHome := os.Getenv("GRADLE_USER_HOME")
		_ = os.Setenv("GRADLE_USER_HOME", gradleHome)
		defer func() {
			if originalHome != "" {
				_ = os.Setenv("GRADLE_USER_HOME", originalHome)
			} else {
				_ = os.Unsetenv("GRADLE_USER_HOME")
			}
		}()

		repo, err := getGradleDeployRepository(projectDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "init-script-repo", repo)
	})

	t.Run("Property from environment variable", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create build.gradle that references a property
		buildContent := `
			publishing {
				repositories {
					maven {
						url "${artifactoryUrl}/libs-release"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		// Set property via ORG_GRADLE_PROJECT_ env var
		_ = os.Setenv("ORG_GRADLE_PROJECT_artifactoryUrl", "http://localhost:8081/artifactory")
		defer func() { _ = os.Unsetenv("ORG_GRADLE_PROJECT_artifactoryUrl") }()

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)
	})
}

func TestGradleDeployRepositoryWithAppliedScripts(t *testing.T) {
	t.Run("Repository from applied script", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create gradle directory
		gradleDir := filepath.Join(tmpDir, "gradle")
		err := os.Mkdir(gradleDir, 0755)
		assert.NoError(t, err)

		// Create publish.gradle
		publishContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/applied-script-repo"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(gradleDir, "publish.gradle"), []byte(publishContent), 0644)
		assert.NoError(t, err)

		// Create build.gradle that applies publish.gradle
		buildContent := `
			apply from: "gradle/publish.gradle"
		`
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "applied-script-repo", repo)
	})

	t.Run("Circular script include protection", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create two scripts that include each other
		scriptAContent := `
			apply from: "script-b.gradle"
		`
		scriptBContent := `
			apply from: "script-a.gradle"
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/circular-repo"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "script-a.gradle"), []byte(scriptAContent), 0644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "script-b.gradle"), []byte(scriptBContent), 0644)
		assert.NoError(t, err)

		// Create build.gradle that applies script-a
		buildContent := `apply from: "script-a.gradle"`
		err = os.WriteFile(filepath.Join(tmpDir, "build.gradle"), []byte(buildContent), 0644)
		assert.NoError(t, err)

		// Should not hang or stack overflow due to circular include protection
		repo, err := getGradleDeployRepository(tmpDir, "", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "circular-repo", repo)
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
				"repo":               "general-repo",
				"snapshotPublishUrl": "http://localhost/artifactory/snapshot-repo",
			},
			isSnapshot: true,
			expected:   "snapshot-repo",
			wantErr:    false,
		},
		{
			name: "Release version prefers release repo",
			props: map[string]string{
				"repo":              "general-repo",
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
		{
			name:     "URL with fragment",
			url:      "http://localhost/artifactory/libs-release#section",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "Deep nested path",
			url:      "http://localhost/artifactory/api/maven/my-org/my-repo",
			expected: "my-repo",
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

func TestExtractRepoKeyFromArtifactoryUrlEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
		wantErr  bool
	}{
		{
			name:     "URL with userinfo",
			url:      "http://user:placeholder@localhost:8081/artifactory/libs-release", // #nosec G101 -- test data with placeholder credential
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "URL with percent encoding - auto decoded",
			url:      "http://localhost:8081/artifactory/libs%2Drelease",
			expected: "libs-release", // Go's URL parser auto-decodes percent encoding
			wantErr:  false,
		},
		{
			name:     "URL with IPv6 address",
			url:      "http://[::1]:8081/artifactory/libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "Just scheme",
			url:      "http://",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Path with only artifactory",
			url:      "http://localhost/artifactory/",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Multiple slashes in path",
			url:      "http://localhost/artifactory//libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "URL with just path no scheme - treated as path",
			url:      "not-a-url",
			expected: "",
			wantErr:  true,
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

func TestExtractRepoKeyCandidate(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple repo key",
			val:      "libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "Full URL with scheme",
			val:      "http://localhost:8081/artifactory/libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "Absolute path",
			val:      "/artifactory/libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
		{
			name:     "Relative path with dot - ignored",
			val:      "./local/repo",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Parent relative path - ignored",
			val:      "../parent/repo",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Maven coordinates with colons - ignored",
			val:      "com.example:lib:1.0.0",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Path with slash - ignored",
			val:      "group/artifact",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Empty string",
			val:      "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "HTTPS URL",
			val:      "https://example.jfrog.io/artifactory/libs-release",
			expected: "libs-release",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractRepoKeyCandidate(tt.val)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
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
			// Note: Simple strings without "/" or ":" are treated as valid repo names
			name:       "Simple string treated as repo key",
			repoUrls:   []string{"my-repo-name"},
			sourceName: "test",
			isSnapshot: false,
			expected:   "my-repo-name",
			wantErr:    false,
		},
		{
			name:       "Invalid URL with special characters",
			repoUrls:   []string{"com.example:lib:1.0"},
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

func TestSelectBestRepo(t *testing.T) {
	tests := []struct {
		name       string
		candidates []string
		isSnapshot bool
		expected   string
		wantErr    bool
	}{
		{
			name:       "Single candidate",
			candidates: []string{"my-repo"},
			isSnapshot: false,
			expected:   "my-repo",
			wantErr:    false,
		},
		{
			name:       "Snapshot version picks snapshot repo",
			candidates: []string{"libs-release", "libs-snapshot"},
			isSnapshot: true,
			expected:   "libs-snapshot",
			wantErr:    false,
		},
		{
			name:       "Release version picks release repo",
			candidates: []string{"libs-release", "libs-snapshot"},
			isSnapshot: false,
			expected:   "libs-release",
			wantErr:    false,
		},
		{
			name:       "Snapshot version fallback to general",
			candidates: []string{"libs-local"},
			isSnapshot: true,
			expected:   "libs-local",
			wantErr:    false,
		},
		{
			name:       "Release version fallback to general",
			candidates: []string{"libs-local"},
			isSnapshot: false,
			expected:   "libs-local",
			wantErr:    false,
		},
		{
			name:       "Empty candidates",
			candidates: []string{},
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name:       "Nil candidates",
			candidates: nil,
			isSnapshot: false,
			expected:   "",
			wantErr:    true,
		},
		{
			name:       "Only snapshot repos for release version - fallback",
			candidates: []string{"only-snapshot-repo"},
			isSnapshot: false,
			expected:   "only-snapshot-repo",
			wantErr:    false,
		},
		{
			name:       "Only release repos for snapshot version - fallback",
			candidates: []string{"only-release-repo"},
			isSnapshot: true,
			expected:   "only-release-repo",
			wantErr:    false,
		},
		{
			name:       "Multiple snapshot repos - first wins",
			candidates: []string{"snapshot-1", "snapshot-2"},
			isSnapshot: true,
			expected:   "snapshot-1",
			wantErr:    false,
		},
		{
			name:       "Duplicates are deduplicated",
			candidates: []string{"repo", "repo", "repo"},
			isSnapshot: false,
			expected:   "repo",
			wantErr:    false,
		},
		{
			name:       "Case insensitive snapshot detection",
			candidates: []string{"LIBS-SNAPSHOT", "libs-release"},
			isSnapshot: true,
			expected:   "LIBS-SNAPSHOT",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := selectBestRepo(tt.candidates, tt.isSnapshot)
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

func TestCheckInitScripts(t *testing.T) {
	t.Run("init.gradle with publishing config", func(t *testing.T) {
		tmpDir := t.TempDir()
		initContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/libs-release"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "init.gradle"), []byte(initContent), 0644)
		assert.NoError(t, err)

		repo, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "libs-release", repo)
	})

	t.Run("init.gradle.kts with publishing config", func(t *testing.T) {
		tmpDir := t.TempDir()
		initContent := `
			publishing {
				repositories {
					maven {
						url = uri("http://localhost:8081/artifactory/kotlin-repo")
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "init.gradle.kts"), []byte(initContent), 0644)
		assert.NoError(t, err)

		repo, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "kotlin-repo", repo)
	})

	t.Run("Scripts in init.d directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		initDDir := filepath.Join(tmpDir, "init.d")
		err := os.Mkdir(initDDir, 0755)
		assert.NoError(t, err)

		// Create script in init.d
		scriptContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/init-d-repo"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(initDDir, "publish.gradle"), []byte(scriptContent), 0644)
		assert.NoError(t, err)

		repo, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "init-d-repo", repo)
	})

	t.Run("init.d scripts precedence - later alphabetically wins", func(t *testing.T) {
		tmpDir := t.TempDir()
		initDDir := filepath.Join(tmpDir, "init.d")
		err := os.Mkdir(initDDir, 0755)
		assert.NoError(t, err)

		// Create two scripts
		script1 := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/a-repo"
					}
				}
			}
		`
		script2 := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/z-repo"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(initDDir, "a-script.gradle"), []byte(script1), 0644)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(initDDir, "z-script.gradle"), []byte(script2), 0644)
		assert.NoError(t, err)

		// Later alphabetically (z-script) should have higher precedence
		repo, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "z-repo", repo)
	})

	t.Run("No init scripts found", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no repository found")
	})

	t.Run("init.gradle takes precedence over init.d", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create init.gradle
		initContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/init-gradle-repo"
					}
				}
			}
		`
		err := os.WriteFile(filepath.Join(tmpDir, "init.gradle"), []byte(initContent), 0644)
		assert.NoError(t, err)

		// Create init.d with different repo
		initDDir := filepath.Join(tmpDir, "init.d")
		err = os.Mkdir(initDDir, 0755)
		assert.NoError(t, err)
		scriptContent := `
			publishing {
				repositories {
					maven {
						url "http://localhost:8081/artifactory/init-d-repo"
					}
				}
			}
		`
		err = os.WriteFile(filepath.Join(initDDir, "script.gradle"), []byte(scriptContent), 0644)
		assert.NoError(t, err)

		repo, err := checkInitScripts(tmpDir, false, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "init-gradle-repo", repo)
	})

	t.Run("Snapshot version selection", func(t *testing.T) {
		tmpDir := t.TempDir()
		initContent := `
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
		`
		err := os.WriteFile(filepath.Join(tmpDir, "init.gradle"), []byte(initContent), 0644)
		assert.NoError(t, err)

		repo, err := checkInitScripts(tmpDir, true, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, "libs-snapshot", repo)
	})
}
