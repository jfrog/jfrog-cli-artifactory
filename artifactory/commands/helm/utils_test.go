package helm

import (
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/stretchr/testify/assert"
)

func TestNeedBuildInfo(t *testing.T) {
	tests := []struct {
		name     string
		cmdName  string
		expected bool
	}{
		{
			name:     "Dependency command needs build info",
			cmdName:  "dependency",
			expected: true,
		},
		{
			name:     "Package command needs build info",
			cmdName:  "package",
			expected: true,
		},
		{
			name:     "Push command needs build info",
			cmdName:  "push",
			expected: true,
		},
		{
			name:     "Other command does not need build info",
			cmdName:  "install",
			expected: false,
		},
		{
			name:     "Empty command does not need build info",
			cmdName:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needBuildInfo(tt.cmdName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPushChartPathAndRegistryURL(t *testing.T) {
	tests := []struct {
		name           string
		helmArgs       []string
		expectedPath   string
		expectedRegURL string
	}{
		{
			name:           "Simple chart path and registry URL",
			helmArgs:       []string{"chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Chart path and registry URL with flags",
			helmArgs:       []string{"chart.tgz", "oci://registry/repo", "--build-name=test"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Chart path and registry URL with flags before",
			helmArgs:       []string{"--build-name=test", "chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Skip push command",
			helmArgs:       []string{"push", "chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Only one positional arg",
			helmArgs:       []string{"chart.tgz"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "",
		},
		{
			name:           "No positional args",
			helmArgs:       []string{"--build-name=test"},
			expectedPath:   "",
			expectedRegURL: "",
		},
		{
			name:           "Empty args",
			helmArgs:       []string{},
			expectedPath:   "",
			expectedRegURL: "",
		},
		{
			name:           "Boolean flags are skipped",
			helmArgs:       []string{"--debug", "--plain-http", "chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
		{
			name:           "Flags with values are skipped",
			helmArgs:       []string{"--username=user", "--password=pass", "chart.tgz", "oci://registry/repo"},
			expectedPath:   "chart.tgz",
			expectedRegURL: "oci://registry/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath, registryURL := getPushChartPathAndRegistryURL(tt.helmArgs)
			assert.Equal(t, tt.expectedPath, chartPath)
			assert.Equal(t, tt.expectedRegURL, registryURL)
		})
	}
}

func TestGetUploadedFileDeploymentPath(t *testing.T) {
	tests := []struct {
		name         string
		registryURL  string
		expectedPath string
	}{
		{
			name:         "Simple OCI URL",
			registryURL:  "oci://example.com/my-repo",
			expectedPath: "my-repo",
		},
		{
			name:         "OCI URL with path",
			registryURL:  "oci://example.com/my-repo/folder",
			expectedPath: "my-repo/folder",
		},
		{
			name:         "Empty URL",
			registryURL:  "",
			expectedPath: "",
		},
		{
			name:         "Invalid OCI reference",
			registryURL:  "oci://",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUploadedFileDeploymentPath(tt.registryURL)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestParseOCIReference(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		expectedReg   string
		expectedRepo  string
		expectedRef   string
		expectedError bool
	}{
		{
			name:         "Valid OCI reference",
			raw:          "example.com/my-repo:1.0.0",
			expectedReg:  "example.com",
			expectedRepo: "my-repo",
			expectedRef:  "1.0.0",
		},
		{
			name:         "OCI reference without tag",
			raw:          "example.com/my-repo",
			expectedReg:  "example.com",
			expectedRepo: "my-repo",
			expectedRef:  "",
		},
		{
			name:         "OCI reference with nested path",
			raw:          "example.com/my-repo/folder:1.0.0",
			expectedReg:  "example.com",
			expectedRepo: "my-repo/folder",
			expectedRef:  "1.0.0",
		},
		{
			name:          "Invalid reference",
			raw:           "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseOCIReference(tt.raw)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedReg, result.Registry)
				assert.Equal(t, tt.expectedRepo, result.Repository)
				assert.Equal(t, tt.expectedRef, result.Reference)
			}
		})
	}
}

func TestAppendModuleAndBuildAgentIfAbsent(t *testing.T) {
	t.Run("Nil build info", func(t *testing.T) {
		appendModuleAndBuildAgentIfAbsent(nil, "test-chart", "1.0.0")
	})
	t.Run("Empty modules adds module and BuildAgent", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{},
		}
		appendModuleAndBuildAgentIfAbsent(buildInfo, "test-chart", "1.0.0")
		assert.Len(t, buildInfo.Modules, 1)
		assert.Equal(t, "test-chart:1.0.0", buildInfo.Modules[0].Id)
		assert.Equal(t, entities.ModuleType("helm"), buildInfo.Modules[0].Type)
		assert.NotNil(t, buildInfo.BuildAgent)
		assert.Equal(t, "Helm", buildInfo.BuildAgent.Name)
		assert.NotEmpty(t, buildInfo.BuildAgent.Version)
	})
	t.Run("Existing module does not add", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{Id: "existing:1.0.0", Type: "helm"},
			},
		}
		initialCount := len(buildInfo.Modules)
		appendModuleAndBuildAgentIfAbsent(buildInfo, "test-chart", "1.0.0")
		assert.Equal(t, initialCount, len(buildInfo.Modules))
	})
}

func TestGetHelmVersion(t *testing.T) {
	version := getHelmVersion()
	assert.NotEmpty(t, version)
}

func TestGetPaths(t *testing.T) {
	tests := []struct {
		name     string
		helmArgs []string
		expected []string
	}{
		{
			name:     "Simple paths without flags",
			helmArgs: []string{"chart.tgz", "oci://registry/repo"},
			expected: []string{"chart.tgz", "oci://registry/repo"},
		},
		{
			name:     "Paths with flags",
			helmArgs: []string{"--build-name=test", "chart.tgz", "--password=pass", "oci://registry/repo"},
			expected: []string{"chart.tgz", "oci://registry/repo"},
		},
		{
			name:     "Only flags",
			helmArgs: []string{"--build-name=test", "--password=pass"},
			expected: nil,
		},
		{
			name:     "Empty args",
			helmArgs: []string{},
			expected: nil,
		},
		{
			name:     "Mixed flags and paths",
			helmArgs: []string{"path1", "--flag", "path2", "--another-flag=value"},
			expected: []string{"path1", "path2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPaths(tt.helmArgs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveDuplicateDependencies(t *testing.T) {
	t.Run("Nil build info", func(t *testing.T) {
		removeDuplicateDependencies(nil)
	})

	t.Run("No duplicates", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Id: "dep2", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateDependencies(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 2)
	})

	t.Run("Removes duplicates by sha256", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Id: "dep2", Checksum: entities.Checksum{Sha256: "sha1"}}, // Duplicate sha256
						{Id: "dep3", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateDependencies(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 2)
		assert.Equal(t, "dep1", buildInfo.Modules[0].Dependencies[0].Id)
		assert.Equal(t, "dep3", buildInfo.Modules[0].Dependencies[1].Id)
	})

	t.Run("Filters out dependencies with empty sha256", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: ""}},
						{Id: "dep2", Checksum: entities.Checksum{Sha256: ""}},
					},
				},
			},
		}
		removeDuplicateDependencies(buildInfo)
		// Dependencies with empty sha256 are filtered out (cannot deduplicate without sha256)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 0)
	})

	t.Run("Multiple modules", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Id: "dep2", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
				{
					Dependencies: []entities.Dependency{
						{Id: "dep3", Checksum: entities.Checksum{Sha256: "sha2"}},
						{Id: "dep4", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateDependencies(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 1)
		assert.Len(t, buildInfo.Modules[1].Dependencies, 1)
	})
}

func TestAddArtifactsInBuildInfo(t *testing.T) {
	t.Run("Nil build info", func(t *testing.T) {
		addArtifactsInBuildInfo(nil, []entities.Artifact{}, "chart", "1.0.0")
	})

	t.Run("Add artifacts to matching module", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id:        "chart:1.0.0",
					Type:      "helm",
					Artifacts: []entities.Artifact{},
				},
			},
		}
		artifacts := []entities.Artifact{
			{Name: "artifact1", Checksum: entities.Checksum{Sha256: "sha1"}},
			{Name: "artifact2", Checksum: entities.Checksum{Sha256: "sha2"}},
		}
		addArtifactsInBuildInfo(buildInfo, artifacts, "chart", "1.0.0")
		assert.Len(t, buildInfo.Modules[0].Artifacts, 2)
		assert.Equal(t, "artifact1", buildInfo.Modules[0].Artifacts[0].Name)
		assert.Equal(t, "artifact2", buildInfo.Modules[0].Artifacts[1].Name)
	})

	t.Run("No matching module", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id:        "other:1.0.0",
					Type:      "helm",
					Artifacts: []entities.Artifact{},
				},
			},
		}
		artifacts := []entities.Artifact{
			{Name: "artifact1", Checksum: entities.Checksum{Sha256: "sha1"}},
		}
		addArtifactsInBuildInfo(buildInfo, artifacts, "chart", "1.0.0")
		assert.Len(t, buildInfo.Modules[0].Artifacts, 0)
	})

	t.Run("Append to existing artifacts", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id:   "chart:1.0.0",
					Type: "helm",
					Artifacts: []entities.Artifact{
						{Name: "existing", Checksum: entities.Checksum{Sha256: "sha0"}},
					},
				},
			},
		}
		artifacts := []entities.Artifact{
			{Name: "new1", Checksum: entities.Checksum{Sha256: "sha1"}},
			{Name: "new2", Checksum: entities.Checksum{Sha256: "sha2"}},
		}
		addArtifactsInBuildInfo(buildInfo, artifacts, "chart", "1.0.0")
		assert.Len(t, buildInfo.Modules[0].Artifacts, 3)
		assert.Equal(t, "existing", buildInfo.Modules[0].Artifacts[0].Name)
		assert.Equal(t, "new1", buildInfo.Modules[0].Artifacts[1].Name)
		assert.Equal(t, "new2", buildInfo.Modules[0].Artifacts[2].Name)
	})
}

func TestRemoveDuplicateArtifacts(t *testing.T) {
	t.Run("Nil build info", func(t *testing.T) {
		removeDuplicateArtifacts(nil)
	})

	t.Run("No duplicates", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Artifacts: []entities.Artifact{
						{Name: "art1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Name: "art2", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateArtifacts(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 2)
	})

	t.Run("Removes duplicates by sha256", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Artifacts: []entities.Artifact{
						{Name: "art1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Name: "art2", Checksum: entities.Checksum{Sha256: "sha1"}}, // Duplicate sha256
						{Name: "art3", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateArtifacts(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 2)
		assert.Equal(t, "art1", buildInfo.Modules[0].Artifacts[0].Name)
		assert.Equal(t, "art3", buildInfo.Modules[0].Artifacts[1].Name)
	})

	t.Run("Filters out artifacts with empty sha256", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Artifacts: []entities.Artifact{
						{Name: "art1", Checksum: entities.Checksum{Sha256: ""}},
						{Name: "art2", Checksum: entities.Checksum{Sha256: ""}},
					},
				},
			},
		}
		removeDuplicateArtifacts(buildInfo)
		// Artifacts with empty sha256 are filtered out (cannot deduplicate without sha256)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 0)
	})

	t.Run("Multiple modules", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Artifacts: []entities.Artifact{
						{Name: "art1", Checksum: entities.Checksum{Sha256: "sha1"}},
						{Name: "art2", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
				{
					Artifacts: []entities.Artifact{
						{Name: "art3", Checksum: entities.Checksum{Sha256: "sha2"}},
						{Name: "art4", Checksum: entities.Checksum{Sha256: "sha2"}},
					},
				},
			},
		}
		removeDuplicateArtifacts(buildInfo)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 1)
		assert.Len(t, buildInfo.Modules[1].Artifacts, 1)
	})
}

func TestAppendModuleInExistingBuildInfo(t *testing.T) {
	t.Run("Nil build info", func(t *testing.T) {
		module := &entities.Module{Id: "test:1.0.0"}
		appendModuleInExistingBuildInfo(nil, module)
	})

	t.Run("Nil module", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{},
		}
		appendModuleInExistingBuildInfo(buildInfo, nil)
		assert.Len(t, buildInfo.Modules, 0)
	})

	t.Run("Add new module when not exists", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{Id: "existing:1.0.0"},
			},
		}
		moduleToAdd := &entities.Module{
			Id: "new:2.0.0",
			Dependencies: []entities.Dependency{
				{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
			},
		}
		appendModuleInExistingBuildInfo(buildInfo, moduleToAdd)
		assert.Len(t, buildInfo.Modules, 2)
		assert.Equal(t, "new:2.0.0", buildInfo.Modules[1].Id)
	})

	t.Run("Append dependencies to existing module", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id: "test:1.0.0",
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
			},
		}
		moduleToAdd := &entities.Module{
			Id: "test:1.0.0",
			Dependencies: []entities.Dependency{
				{Id: "dep2", Checksum: entities.Checksum{Sha256: "sha2"}},
				{Id: "dep3", Checksum: entities.Checksum{Sha256: "sha3"}},
			},
		}
		appendModuleInExistingBuildInfo(buildInfo, moduleToAdd)
		assert.Len(t, buildInfo.Modules, 1)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 3)
		assert.Equal(t, "dep1", buildInfo.Modules[0].Dependencies[0].Id)
		assert.Equal(t, "dep2", buildInfo.Modules[0].Dependencies[1].Id)
		assert.Equal(t, "dep3", buildInfo.Modules[0].Dependencies[2].Id)
	})

	t.Run("Replace artifacts in existing module", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id: "test:1.0.0",
					Artifacts: []entities.Artifact{
						{Name: "old1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
			},
		}
		moduleToAdd := &entities.Module{
			Id: "test:1.0.0",
			Artifacts: []entities.Artifact{
				{Name: "new1", Checksum: entities.Checksum{Sha256: "sha2"}},
				{Name: "new2", Checksum: entities.Checksum{Sha256: "sha3"}},
			},
		}
		appendModuleInExistingBuildInfo(buildInfo, moduleToAdd)
		assert.Len(t, buildInfo.Modules, 1)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 2)
		assert.Equal(t, "new1", buildInfo.Modules[0].Artifacts[0].Name)
		assert.Equal(t, "new2", buildInfo.Modules[0].Artifacts[1].Name)
	})

	t.Run("Empty dependencies and artifacts do not modify", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id: "test:1.0.0",
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
					Artifacts: []entities.Artifact{
						{Name: "art1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
			},
		}
		moduleToAdd := &entities.Module{
			Id:           "test:1.0.0",
			Dependencies: []entities.Dependency{},
			Artifacts:    []entities.Artifact{},
		}
		appendModuleInExistingBuildInfo(buildInfo, moduleToAdd)
		assert.Len(t, buildInfo.Modules, 1)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 1)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 1)
	})

	t.Run("Append dependencies and replace artifacts together", func(t *testing.T) {
		buildInfo := &entities.BuildInfo{
			Modules: []entities.Module{
				{
					Id: "test:1.0.0",
					Dependencies: []entities.Dependency{
						{Id: "dep1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
					Artifacts: []entities.Artifact{
						{Name: "old1", Checksum: entities.Checksum{Sha256: "sha1"}},
					},
				},
			},
		}
		moduleToAdd := &entities.Module{
			Id: "test:1.0.0",
			Dependencies: []entities.Dependency{
				{Id: "dep2", Checksum: entities.Checksum{Sha256: "sha2"}},
			},
			Artifacts: []entities.Artifact{
				{Name: "new1", Checksum: entities.Checksum{Sha256: "sha3"}},
			},
		}
		appendModuleInExistingBuildInfo(buildInfo, moduleToAdd)
		assert.Len(t, buildInfo.Modules, 1)
		assert.Len(t, buildInfo.Modules[0].Dependencies, 2)
		assert.Len(t, buildInfo.Modules[0].Artifacts, 1)
		assert.Equal(t, "dep1", buildInfo.Modules[0].Dependencies[0].Id)
		assert.Equal(t, "dep2", buildInfo.Modules[0].Dependencies[1].Id)
		assert.Equal(t, "new1", buildInfo.Modules[0].Artifacts[0].Name)
	})
}
