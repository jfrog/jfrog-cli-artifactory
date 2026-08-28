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
		depName       string
		registryRepos registryMap
		want          string
	}{
		{
			depName:       "@scope/pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{"@scope": "npm-scoped"}},
			want:          "npm-scoped",
		},
		{
			depName:       "@scope/pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{}},
			want:          "npm-default",
		},
		{
			depName:       "unscoped-pkg",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{"@scope": "npm-scoped"}},
			want:          "npm-default",
		},
		{
			depName:       "@scopeOnly",
			registryRepos: registryMap{defaultRepo: "npm-default", scoped: map[string]string{}},
			want:          "npm-default",
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
		name     string
		version  string
		wantDir  string
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
		url      string
		wantDir  string
		wantFile string
		wantErr  bool
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
		name     string
		pkg      string
		version  string
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
	assert.Equal(t, defaultModuleId, mod.id)
}

func TestAddRequestedBy(t *testing.T) {
	dep := &depInfo{name: "pkg", version: "1.0.0", requestedBy: [][]string{{"root"}}}
	addRequestedBy(dep, []string{"root"})
	assert.Len(t, dep.requestedBy, 1) // duplicate not added
	addRequestedBy(dep, []string{"other"})
	assert.Len(t, dep.requestedBy, 2)
}

func TestGetRegistryScope(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"@scope/pkg", "@scope"},
		{"@babel/core", "@babel"},
		{"lodash", ""},
		{"@scopeOnly", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getRegistryScope(tt.name))
		})
	}
}

func TestAddScope(t *testing.T) {
	dep := &depInfo{name: "pkg", version: "1.0.0", scopes: []string{"transitive"}}
	addScope(dep, "transitive")
	assert.Equal(t, []string{"transitive"}, dep.scopes, "no change for same scope")

	addScope(dep, "dev")
	assert.Equal(t, []string{"dev"}, dep.scopes, "dev wins over transitive")

	addScope(dep, "prod")
	assert.Equal(t, []string{"prod"}, dep.scopes, "prod wins over dev")

	addScope(dep, "dev")
	assert.Equal(t, []string{"prod"}, dep.scopes, "prod not downgraded to dev")
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
		name         string
		pkgName      string
		publishRepos map[string]string
		want         string
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
	installCmd, ok := cmd.(*PnpmInstallCommand)
	assert.True(t, ok, "expected *PnpmInstallCommand")
	assert.NotNil(t, installCmd.pnpmVersion, "NewCommand should wire the resolved pnpm version onto the install command")

	cmd, err = NewCommand("i", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmd, err = NewCommand("publish", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)
	publishCmd, ok := cmd.(*PnpmPublishCommand)
	assert.True(t, ok, "expected *PnpmPublishCommand")
	assert.NotNil(t, publishCmd.pnpmVersion, "NewCommand should wire the resolved pnpm version onto the publish command")

	_, err = NewCommand("p", nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pnpm command")
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
	_, err := validatePnpmPrerequisites()
	assert.NoError(t, err, "pnpm and Node.js should meet minimum version requirements in CI")
}

// TestPnpmVersionValidation verifies the pnpm minimum version check logic.
func TestPnpmVersionValidation(t *testing.T) {
	// pnpm 9.x should be below minimum
	belowPnpm := version.NewVersion("9.15.9")
	assert.Greater(t, belowPnpm.Compare(minSupportedPnpmVersion), 0, "pnpm 9.x should be below minimum")

	// pnpm 10.x should meet minimum
	pnpm10 := version.NewVersion("10.32.1")
	assert.LessOrEqual(t, pnpm10.Compare(minSupportedPnpmVersion), 0, "pnpm 10.32.1 should meet minimum")

	// pnpm 11.x should also meet minimum (no upper bound)
	pnpm11 := version.NewVersion("11.0.0")
	assert.LessOrEqual(t, pnpm11.Compare(minSupportedPnpmVersion), 0, "pnpm 11.0.0 should meet minimum")

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

// TestNodeVersionValidationForPnpm11 verifies the Node.js floor is raised for pnpm >= 11 (RTECO-1644).
func TestNodeVersionValidationForPnpm11(t *testing.T) {
	// pnpm 10.x stays below the pnpm11Version threshold, so it keeps the lower Node floor
	assert.Greater(t, version.NewVersion("10.34.5").Compare(pnpm11Version), 0, "pnpm 10.x should be below the pnpm11Version threshold")
	// pnpm 11.x meets the pnpm11Version threshold, so it should use the higher Node floor
	assert.LessOrEqual(t, version.NewVersion("11.0.0").Compare(pnpm11Version), 0, "pnpm 11.x should meet the pnpm11Version threshold")

	// Node 20.x is valid for pnpm 10 but not for pnpm 11
	assert.LessOrEqual(t, version.NewVersion("20.20.1").Compare(minRequiredNodeVersion), 0, "Node 20.x should be valid for pnpm 10")
	assert.Greater(t, version.NewVersion("20.20.1").Compare(minRequiredNodeVersionForPnpm11), 0, "Node 20.x should be rejected for pnpm 11")

	// pnpm 11's actual floor: Node 22.13.0 valid, just below it rejected
	assert.LessOrEqual(t, version.NewVersion("22.13.0").Compare(minRequiredNodeVersionForPnpm11), 0, "Node 22.13.0 should be valid for pnpm 11")
	assert.Greater(t, version.NewVersion("22.12.9").Compare(minRequiredNodeVersionForPnpm11), 0, "Node 22.12.9 should be rejected for pnpm 11")
}

// TestInstallBuildInfoGracefulDegradation verifies that collectAndSaveBuildInfo returns an error
// when server details are nil. In Run(), this error is caught and logged as a warning,
// allowing the install to succeed even when build info collection fails (RTECO-912).
func TestInstallBuildInfoGracefulDegradation(t *testing.T) {
	cmd := &PnpmInstallCommand{
		workingDirectory: t.TempDir(),
		serverDetails:    nil,
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
	err := cmd.
		collectSinglePublishBuildInfo([]byte("{invalid json"))
	assert.Error(t, err, "collectSinglePublishBuildInfo should fail with invalid JSON")
	assert.Contains(t, err.Error(), "parsing pnpm publish --json output")
}

// TestBuildCommandMetadataEnv verifies the pnpm.command/pnpm.version build-info
// properties are built correctly, including when the version is unknown.
func TestBuildCommandMetadataEnv(t *testing.T) {
	env := buildCommandMetadataEnv(version.NewVersion("11.13.0"), []string{"install", "--frozen-lockfile"})
	assert.Equal(t, "pnpm install --frozen-lockfile", env["pnpm.command"])
	assert.Equal(t, "11.13.0", env["pnpm.version"])

	env = buildCommandMetadataEnv(nil, []string{"publish", "--json"})
	assert.Equal(t, "pnpm publish --json", env["pnpm.command"])
	_, hasVersion := env["pnpm.version"]
	assert.False(t, hasVersion, "pnpm.version should be omitted when version is unknown")
}

// TestRunPnpmInstallSetsExecutedArgs verifies runPnpmInstall records the full
// command (including the "install" subcommand) for build-info metadata (RTECO-918),
// so it matches PnpmPublishCommand's executedArgs, which already includes the
// subcommand.
func TestRunPnpmInstallSetsExecutedArgs(t *testing.T) {
	cmd := &PnpmInstallCommand{
		pnpmArgs:         []string{"--help"},
		workingDirectory: t.TempDir(),
	}
	// --help exits immediately without touching the network or requiring a package.json.
	_ = cmd.runPnpmInstall()
	assert.Equal(t, []string{"install", "--help"}, cmd.executedArgs)
}

// TestRunPnpmPublishSetsExecutedArgs verifies both publish execution paths record
// the full command (subcommand + flags) for build-info metadata.
func TestRunPnpmPublishSetsExecutedArgs(t *testing.T) {
	cmd := &PnpmPublishCommand{workingDirectory: t.TempDir()}

	_, _ = cmd.runPnpmPublishCaptured(publishFlags{publishArgs: []string{"--help"}})
	assert.Equal(t, []string{"publish", "--help"}, cmd.executedArgs)

	_ = cmd.runPnpmPublishNative(publishFlags{isRecursive: true, publishArgs: []string{"--help"}})
	assert.Equal(t, []string{"publish", "-r", "--help"}, cmd.executedArgs)
}
