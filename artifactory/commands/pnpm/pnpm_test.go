package pnpm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/gofrog/version"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	cmd, err = NewCommand("i", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	cmd, err = NewCommand("publish", nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, cmd)

	_, err = NewCommand("p", nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported pnpm command")
}

// TestUserRequestedWorkspaceRoot verifies detection of the --workspace-root / -w
// flag, which signals the user wants pnpm install to operate at the workspace
// root and therefore build-info must NOT be scoped to a sub-package.
func TestUserRequestedWorkspaceRoot(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"no flags", nil, false},
		{"unrelated flags", []string{"--frozen-lockfile", "--prod"}, false},
		{"long form alone", []string{"--workspace-root"}, true},
		{"short form alone", []string{"-w"}, true},
		{"long form mixed", []string{"--prod", "--workspace-root", "--frozen-lockfile"}, true},
		{"short form mixed", []string{"--prod", "-w", "--frozen-lockfile"}, true},
		// -W is not the same flag — must not match.
		{"capital -W is not workspace-root", []string{"-W"}, false},
		// Substring of an unrelated flag must not match.
		{"longer flag containing 'workspace-root' substring", []string{"--no-workspace-root"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, userRequestedWorkspaceRoot(tt.args))
		})
	}
}

// TestExtractLsForwardFlags verifies that flags from `pnpm install` that affect
// the resolved dependency tree (workspace scope and dep-group filtering) are
// forwarded to `pnpm ls`, while install-only flags are dropped.
func TestExtractLsForwardFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"no flags", []string{}, nil},
		{"only install-only flags", []string{"--frozen-lockfile", "--no-color"}, nil},
		{"ignore-workspace", []string{"--ignore-workspace"}, []string{"--ignore-workspace"}},
		{"prod", []string{"--prod"}, []string{"--prod"}},
		{"production", []string{"--production"}, []string{"--production"}},
		{"dev", []string{"--dev"}, []string{"--dev"}},
		// --no-optional is intentionally NOT forwarded today: the build-info
		// parser doesn't read pnpm ls's `optionalDependencies` JSON key, so the
		// flag would be a no-op. Locked here so a future re-add is intentional.
		{"no-optional dropped (parser does not read optionalDeps)", []string{"--no-optional"}, nil},
		{"workspace-root", []string{"--workspace-root"}, []string{"--workspace-root"}},
		{
			name: "filter with separate value",
			args: []string{"--filter", "web-app"},
			want: []string{"--filter", "web-app"},
		},
		{
			name: "filter with attached value",
			args: []string{"--filter=web-app"},
			want: []string{"--filter=web-app"},
		},
		{
			name: "repeated filter",
			args: []string{"--filter=a", "--filter", "b"},
			want: []string{"--filter=a", "--filter", "b"},
		},
		{
			name: "all forwarded flags mixed with install-only",
			args: []string{"--frozen-lockfile", "--ignore-workspace", "--prod", "--no-optional", "--filter", "pkg", "--reporter=ndjson"},
			want: []string{"--ignore-workspace", "--prod", "--filter", "pkg"},
		},
		{
			// Defensive: `--filter` at the very end with no value should be dropped rather
			// than capturing the (non-existent) next arg or emitting a dangling flag.
			name: "filter without value at end is dropped",
			args: []string{"--ignore-workspace", "--filter"},
			want: []string{"--ignore-workspace"},
		},
		// Short-form aliases supported by pnpm install: -P (--prod), -D (--dev),
		// -F (--filter), -w (--workspace-root). All have the same semantics in
		// pnpm ls and must be forwarded so build-info matches the install scope.
		{"short-form -P forwarded", []string{"-P"}, []string{"-P"}},
		{"short-form -D forwarded", []string{"-D"}, []string{"-D"}},
		{"short-form -w forwarded", []string{"-w"}, []string{"-w"}},
		{"short-form -F with separate value", []string{"-F", "web-app"}, []string{"-F", "web-app"}},
		{"short-form -F with attached value", []string{"-F=web-app"}, []string{"-F=web-app"}},
		{"short-form -F without value at end is dropped", []string{"-F"}, nil},
		{
			// Unrelated `--foo=bar` style flags should not be forwarded just because
			// they contain '='.
			name: "unrelated --flag=value not forwarded",
			args: []string{"--reporter=ndjson"},
			want: nil,
		},
		// Boolean flags with `=value` form. Falsy values (false, 0, F) are dropped;
		// truthy values (true, 1, T) are forwarded; unparseable values are forwarded
		// so pnpm can validate. Matches strconv.ParseBool / jfrog-cli-core semantics.
		{"bool =true forwarded", []string{"--ignore-workspace=true"}, []string{"--ignore-workspace=true"}},
		{"bool =false dropped", []string{"--ignore-workspace=false"}, nil},
		{"bool =1 forwarded", []string{"--prod=1"}, []string{"--prod=1"}},
		{"bool =0 dropped", []string{"--prod=0"}, nil},
		{"bool =T forwarded", []string{"--prod=T"}, []string{"--prod=T"}},
		{"bool =F dropped", []string{"--prod=F"}, nil},
		{"bool =empty forwarded (unparseable, deferred to pnpm)", []string{"--ignore-workspace="}, []string{"--ignore-workspace="}},
		{"bool =garbage forwarded (unparseable, deferred to pnpm)", []string{"--prod=maybe"}, []string{"--prod=maybe"}},
		{
			name: "mixed bool =value with bare and value flags",
			args: []string{"--prod=true", "--dev", "--filter=pkg"},
			want: []string{"--prod=true", "--dev", "--filter=pkg"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractLsForwardFlags(tt.args))
		})
	}
}

// TestBuildPnpmLsArgs verifies the pnpm ls command line is assembled correctly
// based on workspace context and forwarded flags.
func TestBuildPnpmLsArgs(t *testing.T) {
	tests := []struct {
		name                  string
		extraArgs             []string
		scopeToCurrentPackage bool
		want                  []string
	}{
		{
			name:                  "workspace root (no scoping, no extra flags)",
			extraArgs:             nil,
			scopeToCurrentPackage: false,
			want:                  []string{"ls", "-r", "--depth", "Infinity", "--json"},
		},
		{
			name:                  "sub-package (scoped, no extra flags)",
			extraArgs:             nil,
			scopeToCurrentPackage: true,
			want:                  []string{"ls", "--depth", "Infinity", "--json"},
		},
		{
			// `pnpm ls -r --ignore-workspace` prints one JSON array per project concatenated
			// on stdout, which is not parseable. When --ignore-workspace is forwarded we must
			// drop -r regardless of cwd so the output stays a single JSON array.
			name:                  "workspace root with ignore-workspace forwarded (must drop -r)",
			extraArgs:             []string{"--ignore-workspace"},
			scopeToCurrentPackage: false,
			want:                  []string{"ls", "--depth", "Infinity", "--json", "--ignore-workspace"},
		},
		{
			name:                  "sub-package with ignore-workspace forwarded",
			extraArgs:             []string{"--ignore-workspace"},
			scopeToCurrentPackage: true,
			want:                  []string{"ls", "--depth", "Infinity", "--json", "--ignore-workspace"},
		},
		{
			// `--ignore-workspace=true` must drop -r the same way the bare flag does,
			// otherwise pnpm emits unparseable concatenated JSON.
			name:                  "workspace root with --ignore-workspace=true (must drop -r)",
			extraArgs:             []string{"--ignore-workspace=true"},
			scopeToCurrentPackage: false,
			want:                  []string{"ls", "--depth", "Infinity", "--json", "--ignore-workspace=true"},
		},
		{
			// `--ignore-workspace=false` is a no-op — `-r` must still be added.
			name:                  "workspace root with --ignore-workspace=false (keeps -r)",
			extraArgs:             []string{"--ignore-workspace=false"},
			scopeToCurrentPackage: false,
			want:                  []string{"ls", "-r", "--depth", "Infinity", "--json", "--ignore-workspace=false"},
		},
		{
			// `--ignore-workspace=0` is also falsy (strconv.ParseBool) — keep -r.
			name:                  "workspace root with --ignore-workspace=0 (keeps -r)",
			extraArgs:             []string{"--ignore-workspace=0"},
			scopeToCurrentPackage: false,
			want:                  []string{"ls", "-r", "--depth", "Infinity", "--json", "--ignore-workspace=0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildPnpmLsArgs(tt.extraArgs, tt.scopeToCurrentPackage))
		})
	}
}

// TestSamePath verifies that symlinked paths are recognized as the same directory.
// This matters on macOS where /var is a symlink to /private/var, and pnpm resolves
// symlinks in its output while our workingDir is typically unresolved.
func TestSamePath(t *testing.T) {
	t.Run("identical strings", func(t *testing.T) {
		assert.True(t, samePath("/foo/bar", "/foo/bar"))
	})

	t.Run("different strings", func(t *testing.T) {
		assert.False(t, samePath("/foo/bar", "/foo/baz"))
	})

	t.Run("symlinked directories resolve equal", func(t *testing.T) {
		real := t.TempDir()
		linkDir := filepath.Join(t.TempDir(), "link")
		require.NoError(t, os.Symlink(real, linkDir))

		assert.True(t, samePath(real, linkDir),
			"paths pointing to the same directory via a symlink should be treated as equal")
	})

	t.Run("non-existent paths fall back to string comparison", func(t *testing.T) {
		assert.False(t, samePath("/does/not/exist/a", "/does/not/exist/b"))
		assert.True(t, samePath("/does/not/exist", "/does/not/exist"))
	})
}

// TestIsPnpmWorkspaceSubPackage exercises the detection helper against three
// realistic filesystem layouts. It shells out to pnpm (available in CI per
// TestValidatePnpmPrerequisites) but does not require any packages to be installed.
func TestIsPnpmWorkspaceSubPackage(t *testing.T) {
	t.Run("non-workspace directory returns false", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, isPnpmWorkspaceSubPackage(dir),
			"a plain directory with no pnpm-workspace.yaml must not be treated as a sub-package")
	})

	t.Run("workspace root returns false", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"),
			[]byte("packages:\n  - 'apps/*'\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"),
			[]byte(`{"name":"root","version":"1.0.0"}`), 0o644))

		assert.False(t, isPnpmWorkspaceSubPackage(root),
			"workingDir equal to the workspace root must not be treated as a sub-package")
	})

	t.Run("workspace sub-package returns true", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"),
			[]byte("packages:\n  - 'apps/*'\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"),
			[]byte(`{"name":"root","version":"1.0.0"}`), 0o644))

		subPkg := filepath.Join(root, "apps", "web-app")
		require.NoError(t, os.MkdirAll(subPkg, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subPkg, "package.json"),
			[]byte(`{"name":"web-app","version":"1.0.0"}`), 0o644))

		assert.True(t, isPnpmWorkspaceSubPackage(subPkg),
			"workingDir inside a workspace package must be treated as a sub-package")
	})
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
	assert.Greater(t, pnpm10.Compare(firstUnsupportedPnpmVersion), 0, "pnpm 10.32.1 should be below max")

	// pnpm 11.x should be rejected (above max)
	pnpm11 := version.NewVersion("11.0.0")
	assert.LessOrEqual(t, pnpm11.Compare(firstUnsupportedPnpmVersion), 0, "pnpm 11.0.0 should be at or above max")

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
