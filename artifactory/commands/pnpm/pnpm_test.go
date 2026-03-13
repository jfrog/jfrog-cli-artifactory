package pnpm

import (
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/version"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
)

func TestResolveRepoFromRegistry(t *testing.T) {
	tests := []struct {
		depName      string
		registryRepos registryMap
		want         string
	}{
		{
			depName:      "@scope/pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{"@scope": "npm-scoped"}},
			want:         "npm-scoped",
		},
		{
			depName:      "@scope/pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{}},
			want:         "npm-default",
		},
		{
			depName:      "unscoped-pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{"@scope": "npm-scoped"}},
			want:         "npm-default",
		},
		{
			depName:      "@scopeOnly",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{}},
			want:         "npm-default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.depName, func(t *testing.T) {
			got := resolveRepoFromRegistry(tt.depName, tt.registryRepos)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractRepoFromRegistryURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://mycompany.jfrog.io/artifactory/api/npm/my-npm-repo/", "my-npm-repo"},
		{"https://artifactory.example.com/artifactory/api/npm/npm-local", "npm-local"},
		{"http://localhost:8081/artifactory/api/npm/cli-npm/", "cli-npm"},
		{"https://example.com/not-npm/repo/", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractRepoFromRegistryURL(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildTarballPartsFromName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantDir string
		wantFile string
	}{
		{"pkg", "1.0.0", "pkg/-", "pkg-1.0.0.tgz"},
		{"@scope/pkg", "1.0.0", "@scope/pkg/-", "pkg-1.0.0.tgz"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"@"+tt.version, func(t *testing.T) {
			parts := buildTarballPartsFromName(tt.name, tt.version)
			assert.Equal(t, tt.wantDir, parts.dirPath)
			assert.Equal(t, tt.wantFile, parts.fileName)
		})
	}
}

func TestParseTarballURL(t *testing.T) {
	tests := []struct {
		url        string
		wantDir    string
		wantFile   string
		wantErr    bool
	}{
		{
			url:      "https://artifactory.example.com/artifactory/api/npm/npm-repo/pkg/-/pkg-1.0.0.tgz",
			wantDir:  "pkg/-",
			wantFile: "pkg-1.0.0.tgz",
			wantErr:  false,
		},
		{
			url:      "https://artifactory.example.com/artifactory/api/npm/npm-repo/@scope/pkg/-/pkg-1.0.0.tgz",
			wantDir:  "@scope/pkg/-",
			wantFile: "pkg-1.0.0.tgz",
			wantErr:  false,
		},
		{
			url:     "invalid-url",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			parts, err := parseTarballURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantDir, parts.dirPath)
			assert.Equal(t, tt.wantFile, parts.fileName)
		})
	}
}

func TestSlicesEqual(t *testing.T) {
	assert.True(t, slicesEqual([]string{"a", "b"}, []string{"a", "b"}))
	assert.False(t, slicesEqual([]string{"a"}, []string{"a", "b"}))
	assert.False(t, slicesEqual([]string{"a", "b"}, []string{"a", "c"}))
	assert.True(t, slicesEqual(nil, nil))
	assert.True(t, slicesEqual([]string{}, nil)) // both have len 0
}

func TestExtractPublishFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantRec    bool
		wantDry    bool
		wantSum    bool
		wantJson   bool
		wantFilter int // expected len of filterArgs
	}{
		{"recursive", []string{"-r"}, true, false, false, false, 0},
		{"recursive long", []string{"--recursive"}, true, false, false, false, 0},
		{"dry-run", []string{"--dry-run"}, false, true, false, false, 0},
		{"report-summary", []string{"--report-summary"}, false, false, true, false, 0},
		{"json flag", []string{"--json"}, false, false, false, true, 0},
		{"filter arg", []string{"--filter", "pkg1"}, false, false, false, false, 2},
		{"filter equals", []string{"--filter=pkg1"}, false, false, false, false, 1},
		{"mixed", []string{"-r", "--filter", "pkg1", "--dry-run"}, true, true, false, false, 2},
		{"empty", []string{}, false, false, false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := extractPublishFlags(tt.args)
			assert.Equal(t, tt.wantRec, f.isRecursive)
			assert.Equal(t, tt.wantDry, f.isDryRun)
			assert.Equal(t, tt.wantSum, f.userProvidedSummary)
			assert.Equal(t, tt.wantJson, f.userProvidedJson)
			assert.Len(t, f.filterArgs, tt.wantFilter)
		})
	}
}

func TestParsePackOutput(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantLen int
		wantErr bool
	}{
		{"array", `[{"name":"pkg1","version":"1.0.0","filename":"pkg1-1.0.0.tgz"}]`, 1, false},
		{"array multi", `[{"name":"a","version":"1.0.0","filename":"a.tgz"},{"name":"b","version":"2.0.0","filename":"b.tgz"}]`, 2, false},
		{"single object", `{"name":"pkg1","version":"1.0.0","filename":"pkg1-1.0.0.tgz"}`, 1, false},
		{"empty", ``, 0, false},
		{"whitespace", `  `, 0, false},
		{"invalid json", `{invalid}`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePackOutput([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

func TestBuildPnpmDeployPath(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		version string
		wantPath string
		wantName string
	}{
		{"unscoped", "pkg", "1.0.0", "pkg/-", "pkg-1.0.0.tgz"},
		{"scoped", "@scope/pkg", "1.0.0", "@scope/pkg/-", "@scope/pkg-1.0.0.tgz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, name := buildPnpmDeployPath(tt.pkg, tt.version)
			assert.Equal(t, tt.wantPath, path)
			assert.Equal(t, tt.wantName, name)
		})
	}
}

func TestFormatModuleId(t *testing.T) {
	tests := []struct {
		name, version, want string
	}{
		{"pkg", "1.0.0", "pkg:1.0.0"},
		{"pkg", "v2.0.0", "pkg:2.0.0"},
		{"pkg", "=3.0.0", "pkg:3.0.0"},
		{"@scope/pkg", "1.0.0", "scope:pkg:1.0.0"},
		{"@scope/pkg", "=1.0.0", "scope:pkg:1.0.0"},
		{"@scope/pkg", "v1.0.0", "scope:pkg:1.0.0"},
		{"", "1.0.0", ""},
		{"pkg", "", "pkg"},
	}
	for _, tt := range tests {
		t.Run(tt.name+":"+tt.version, func(t *testing.T) {
			assert.Equal(t, tt.want, formatModuleId(tt.name, tt.version))
		})
	}
}

func TestParsePnpmLsProjects(t *testing.T) {
	projects := []pnpmLsProject{
		{
			Name: "proj1", Version: "1.0.0", Path: "/proj1",
			Dependencies: map[string]pnpmLsDependency{
				"pkg": {Version: "1.0.0", Resolved: "https://reg/pkg-1.0.0.tgz"},
			},
		},
		{
			Name: "proj2", Version: "2.0.0", Path: "/proj2",
			Dependencies: map[string]pnpmLsDependency{},
		},
	}
	mods := parsePnpmLsProjects(projects)
	assert.Len(t, mods, 1) // proj2 has no deps, skipped
	assert.Equal(t, "proj1:1.0.0", mods[0].id)
	assert.Len(t, mods[0].dependencies, 1)
	assert.Equal(t, "pkg:1.0.0", mods[0].dependencies[0].Id)
}

func TestParseSingleProjectEmptyName(t *testing.T) {
	proj := pnpmLsProject{
		Name: "", Version: "1.0.0",
		Dependencies: map[string]pnpmLsDependency{
			"pkg": {Version: "1.0.0"},
		},
	}
	mod := parseSingleProject(proj)
	assert.NotNil(t, mod)
	assert.Equal(t, "pnpm-project", mod.id)
}

func TestAddRequestedBy(t *testing.T) {
	dep := &depInfo{name: "pkg", version: "1.0.0", requestedBy: [][]string{{"root"}}}
	addRequestedBy(dep, []string{"root"})
	assert.Len(t, dep.requestedBy, 1) // duplicate not added
	addRequestedBy(dep, []string{"other"})
	assert.Len(t, dep.requestedBy, 2)
}

func TestAddScope(t *testing.T) {
	dep := &depInfo{name: "pkg", version: "1.0.0", scopes: []string{"prod"}}
	addScope(dep, "prod")
	assert.Len(t, dep.scopes, 1) // duplicate not added
	addScope(dep, "dev")
	assert.Len(t, dep.scopes, 2)
}

func TestBuildBatchAQLQuery(t *testing.T) {
	deps := []parsedDep{
		{dep: depInfo{name: "pkg1", version: "1.0.0"}, parts: tarballParts{dirPath: "pkg1/-", fileName: "pkg1-1.0.0.tgz"}},
		{dep: depInfo{name: "pkg2", version: "2.0.0"}, parts: tarballParts{dirPath: "pkg2/-", fileName: "pkg2-2.0.0.tgz"}},
	}
	q := buildBatchAQLQuery("npm-repo", deps)
	assert.Contains(t, q, `"repo":"npm-repo"`)
	assert.Contains(t, q, `"path":"pkg1/-"`)
	assert.Contains(t, q, `"name":"pkg1-1.0.0.tgz"`)
	assert.Contains(t, q, `"path":"pkg2/-"`)
	assert.Contains(t, q, `"name":"pkg2-2.0.0.tgz"`)
	assert.Contains(t, q, "actual_sha1")
	assert.Contains(t, q, "sha256")
	assert.Contains(t, q, "actual_md5")
}

func TestWalkSingleDepSkipsLink(t *testing.T) {
	depMap := make(map[string]*depInfo)
	walkSingleDep("linkpkg", pnpmLsDependency{Version: "link:../local"}, "prod", "root", depMap)
	assert.Empty(t, depMap)
}

func TestWalkDependenciesWithTransitive(t *testing.T) {
	depMap := make(map[string]*depInfo)
	walkDependencies(map[string]pnpmLsDependency{
		"parent": {
			Version: "1.0.0",
			Dependencies: map[string]pnpmLsDependency{
				"child": {Version: "2.0.0"},
			},
		},
	}, "prod", "root", depMap)
	assert.Len(t, depMap, 2) // parent + child (transitive)
	assert.Contains(t, depMap, "parent:1.0.0")
	assert.Contains(t, depMap, "child:2.0.0")
	assert.Equal(t, "transitive", depMap["child:2.0.0"].scopes[0])
}

func TestReadPublishSummary(t *testing.T) {
	// Non-existent file returns nil, nil
	got, err := readPublishSummary("/nonexistent/path")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

// TestResolvePublishRepoPriority verifies registry priority: publishConfig.registry > pnpm config.
func TestResolvePublishRepoPriority(t *testing.T) {
	fallback := registryMap{
		defaultRepo: "npm-default",
		scoped:      map[string]string{"@scope": "npm-scoped"},
	}
	tests := []struct {
		name        string
		pkgName     string
		publishRepos map[string]string
		want        string
	}{
		{"publishConfig wins", "pkg1", map[string]string{"pkg1": "npm-publish-local"}, "npm-publish-local"},
		{"fallback to scoped", "@scope/pkg", map[string]string{}, "npm-scoped"},
		{"fallback to default", "unscoped", map[string]string{}, "npm-default"},
		{"publishConfig overrides scoped", "@scope/pkg", map[string]string{"@scope/pkg": "npm-custom"}, "npm-custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePublishRepo(tt.pkgName, tt.publishRepos, fallback)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectAllDepsFromModules(t *testing.T) {
	mod1 := &moduleInfo{
		rawDeps: []depInfo{{name: "pkg1", version: "1.0.0"}, {name: "pkg2", version: "2.0.0"}},
	}
	mod2 := &moduleInfo{
		rawDeps: []depInfo{{name: "pkg2", version: "2.0.0"}, {name: "pkg3", version: "3.0.0"}},
	}
	all := collectAllDepsFromModules([]*moduleInfo{mod1, mod2})
	assert.Len(t, all, 3) // pkg1, pkg2, pkg3 (pkg2 deduplicated)
	ids := make(map[string]bool)
	for _, d := range all {
		ids[d.name+":"+d.version] = true
	}
	assert.True(t, ids["pkg1:1.0.0"])
	assert.True(t, ids["pkg2:2.0.0"])
	assert.True(t, ids["pkg3:3.0.0"])
}

func TestApplyChecksumsToModules(t *testing.T) {
	mod := &moduleInfo{
		dependencies: []entities.Dependency{
			{Id: "pkg1:1.0.0"},
			{Id: "pkg2:2.0.0"},
		},
	}
	checksumMap := map[string]entities.Checksum{
		"pkg1:1.0.0": {Sha1: "abc", Md5: "def", Sha256: "ghi"},
	}
	applyChecksumsToModules([]*moduleInfo{mod}, checksumMap)
	assert.False(t, mod.dependencies[0].IsEmpty())
	assert.True(t, mod.dependencies[1].IsEmpty())
}

// TestNewCommandUnsupported verifies correct identification of pnpm command (RTECO-918).
func TestNewCommandUnsupported(t *testing.T) {
	_, err := NewCommand("add", nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pnpm command")

	_, err = NewCommand("run", nil, nil, nil)
	assert.Error(t, err)

	cmd, err := NewCommand("install", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmd, err = NewCommand("i", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmd, err = NewCommand("publish", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmd, err = NewCommand("p", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)
}

// TestParsePnpmLsProjectsEmpty verifies handling of empty/minimal pnpm ls output (RTECO-903).
func TestParsePnpmLsProjectsEmpty(t *testing.T) {
	mods := parsePnpmLsProjects([]pnpmLsProject{})
	assert.Empty(t, mods)

	// Project with no dependencies is skipped
	mods = parsePnpmLsProjects([]pnpmLsProject{
		{Name: "empty", Version: "1.0.0", Dependencies: map[string]pnpmLsDependency{}},
	})
	assert.Empty(t, mods)
}

func TestParseNpmPublishJson(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantName string
		wantVer  string
		wantNil  bool
		wantErr  bool
	}{
		{
			name:     "valid output",
			data:     `{"id":"pkg@1.0.0","name":"pkg","version":"1.0.0","filename":"pkg-1.0.0.tgz"}`,
			wantName: "pkg",
			wantVer:  "1.0.0",
		},
		{
			name:     "scoped package",
			data:     `{"id":"@scope/pkg@1.0.0","name":"@scope/pkg","version":"1.0.0","filename":"scope-pkg-1.0.0.tgz"}`,
			wantName: "@scope/pkg",
			wantVer:  "1.0.0",
		},
		{"empty input", "", "", "", true, false},
		{"whitespace only", "  \n  ", "", "", true, false},
		{"empty name", `{"id":"","name":"","version":"1.0.0"}`, "", "", true, false},
		{"invalid json", `{bad}`, "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNpmPublishJson([]byte(tt.data))
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantVer, got.Version)
		})
	}
}

func TestMapAQLResults(t *testing.T) {
	deps := []parsedDep{
		{dep: depInfo{name: "pkg1", version: "1.0.0"}, parts: tarballParts{dirPath: "pkg1/-", fileName: "pkg1-1.0.0.tgz"}},
		{dep: depInfo{name: "pkg2", version: "2.0.0"}, parts: tarballParts{dirPath: "pkg2/-", fileName: "pkg2-2.0.0.tgz"}},
	}
	results := []servicesUtils.ResultItem{
		{Path: "pkg1/-", Name: "pkg1-1.0.0.tgz", Actual_Sha1: "sha1a", Actual_Md5: "md5a", Sha256: "sha256a"},
	}
	checksumMap := make(map[string]entities.Checksum)
	mapAQLResults(deps, results, checksumMap)
	assert.Len(t, checksumMap, 1)
	assert.Equal(t, "sha1a", checksumMap["pkg1:1.0.0"].Sha1)
	assert.Equal(t, "md5a", checksumMap["pkg1:1.0.0"].Md5)
}

// TestValidatePnpmPrerequisites verifies that pnpm and Node.js version validation works (RTECO-918).
func TestValidatePnpmPrerequisites(t *testing.T) {
	// This test runs against the actual pnpm and Node.js installed on the machine.
	// It will pass if pnpm >= 10.0.0 and Node.js >= 18.12.0 are installed.
	err := validatePnpmPrerequisites()
	assert.NoError(t, err, "pnpm and Node.js should meet minimum version requirements in CI")
}

// TestPnpmVersionValidation verifies the pnpm 10 version range check logic.
func TestPnpmVersionValidation(t *testing.T) {
	// pnpm 9.x should be below minimum
	belowPnpm := version.NewVersion("9.15.9")
	assert.Greater(t, belowPnpm.Compare(minSupportedPnpmVersion), 0, "pnpm 9.x should be below minimum")

	// pnpm 10.x should be within supported range
	pnpm10 := version.NewVersion("10.32.1")
	assert.LessOrEqual(t, pnpm10.Compare(minSupportedPnpmVersion), 0, "pnpm 10.32.1 should meet minimum")
	assert.Greater(t, pnpm10.Compare(minUnsupportedPnpmVersion), 0, "pnpm 10.32.1 should be below max")

	// pnpm 11.x should be rejected (above max)
	pnpm11 := version.NewVersion("11.0.0")
	assert.LessOrEqual(t, pnpm11.Compare(minUnsupportedPnpmVersion), 0, "pnpm 11.0.0 should be at or above max")

	// Exact minimum should pass
	exactPnpm := version.NewVersion(minSupportedPnpmVersion)
	assert.Equal(t, 0, exactPnpm.Compare(minSupportedPnpmVersion), "exact minimum should pass")
}

// TestNodeVersionValidation verifies Node.js version checks for pnpm 10.
func TestNodeVersionValidation(t *testing.T) {
	assert.LessOrEqual(t, version.NewVersion("20.20.1").Compare(minRequiredNodeVersion), 0, "Node 20.x should be valid")
	assert.LessOrEqual(t, version.NewVersion("18.12.0").Compare(minRequiredNodeVersion), 0, "Node 18.12.0 should be valid")
	assert.Greater(t, version.NewVersion("16.14.0").Compare(minRequiredNodeVersion), 0, "Node 16.x should be rejected")
	assert.Greater(t, version.NewVersion("18.11.0").Compare(minRequiredNodeVersion), 0, "Node 18.11.0 should be rejected")
}

// TestInstallBuildInfoGracefulDegradation verifies that collectAndSaveBuildInfo returns an error
// when server details are nil. In Run(), this error is caught and logged as a warning,
// allowing the install to succeed even when build info collection fails (RTECO-912).
func TestInstallBuildInfoGracefulDegradation(t *testing.T) {
	cmd := &PnpmInstallCommand{
		workingDirectory: t.TempDir(),
		serverDetails:    nil,
		collectBuildInfo: true,
	}
	err := cmd.collectAndSaveBuildInfo()
	assert.Error(t, err, "collectAndSaveBuildInfo should fail with nil server details")
	assert.Contains(t, err.Error(), "no server configuration")
}

// TestPublishBuildInfoGracefulDegradation verifies that collectSinglePublishBuildInfo returns
// an error when given malformed output. In publishSingleWithBuildInfo(), this error is caught
// and logged as a warning, allowing the publish to succeed (RTECO-912).
func TestPublishBuildInfoGracefulDegradation(t *testing.T) {
	cmd := &PnpmPublishCommand{
		workingDirectory: t.TempDir(),
	}
	err := cmd.collectSinglePublishBuildInfo([]byte("{invalid json"))
	assert.Error(t, err, "collectSinglePublishBuildInfo should fail with invalid JSON")
	assert.Contains(t, err.Error(), "parsing pnpm publish --json output")
}
