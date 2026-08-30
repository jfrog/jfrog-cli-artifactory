package cargo

import (
	"reflect"
	"testing"

	"github.com/jfrog/build-info-go/entities"
)

func TestPublishedCrateFileName(t *testing.T) {
	mk := func(id string) *entities.BuildInfo {
		return &entities.BuildInfo{Modules: []entities.Module{{Id: id}}}
	}
	// Single-module (single-crate) projects: no -p needed, use the sole module.
	single := map[string]string{
		"jf-cargo-sample:0.1.0": "jf-cargo-sample-0.1.0.crate",
		"serde:1.0.228":         "serde-1.0.228.crate",
		"noversion":             "", // no ":" -> cannot derive
		"trailing:":             "", // empty version
	}
	for id, want := range single {
		if got := publishedCrateFileName(mk(id), nil); got != want {
			t.Errorf("publishedCrateFileName(%q) = %q, want %q", id, got, want)
		}
	}
	if got := publishedCrateFileName(&entities.BuildInfo{}, nil); got != "" {
		t.Errorf("no modules: got %q, want empty", got)
	}

	// Workspace (multi-module): the published crate is selected by -p/--package.
	ws := &entities.BuildInfo{Modules: []entities.Module{
		{Id: "jf-ws-app:0.1.0"},
		{Id: "jf-ws-lib:0.2.0"},
	}}
	if got := publishedCrateFileName(ws, []string{"publish", "-p", "jf-ws-lib"}); got != "jf-ws-lib-0.2.0.crate" {
		t.Errorf("-p jf-ws-lib: got %q, want jf-ws-lib-0.2.0.crate", got)
	}
	if got := publishedCrateFileName(ws, []string{"publish", "--package=jf-ws-app"}); got != "jf-ws-app-0.1.0.crate" {
		t.Errorf("--package=jf-ws-app: got %q, want jf-ws-app-0.1.0.crate", got)
	}
	// Multi-module with no -p is ambiguous -> empty.
	if got := publishedCrateFileName(ws, []string{"publish"}); got != "" {
		t.Errorf("multi-module no -p: got %q, want empty", got)
	}
	// -p naming a package not present in any module -> empty.
	if got := publishedCrateFileName(ws, []string{"publish", "-p", "nope"}); got != "" {
		t.Errorf("-p unknown: got %q, want empty", got)
	}
}

func TestPackageNameFromArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"publish", "-p", "mycrate"}, "mycrate"},
		{[]string{"publish", "--package", "mycrate"}, "mycrate"},
		{[]string{"publish", "--package=mycrate"}, "mycrate"},
		{[]string{"publish", "-p=mycrate"}, "mycrate"},
		{[]string{"publish", "--registry", "jfrog"}, ""},
	}
	for _, c := range cases {
		if got := packageNameFromArgs(c.args); got != c.want {
			t.Errorf("packageNameFromArgs(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestModuleIndexForCrate(t *testing.T) {
	bi := &entities.BuildInfo{Modules: []entities.Module{
		{Id: "jf-ws-app:0.1.0"},
		{Id: "jf-ws-lib:0.2.0"},
	}}
	if got := moduleIndexForCrate(bi, "jf-ws-lib-0.2.0.crate"); got != 1 {
		t.Errorf("lib crate -> module index %d, want 1", got)
	}
	if got := moduleIndexForCrate(bi, "jf-ws-app-0.1.0.crate"); got != 0 {
		t.Errorf("app crate -> module index %d, want 0", got)
	}
	// No match -> fallback to 0.
	if got := moduleIndexForCrate(bi, "unrelated-9.9.9.crate"); got != 0 {
		t.Errorf("no match -> %d, want 0 (fallback)", got)
	}
}

func TestApplyModuleOverride(t *testing.T) {
	// --module renames the module at the given index.
	bi := &entities.BuildInfo{Modules: []entities.Module{{Id: "orig:1.0.0"}, {Id: "second:2.0.0"}}}
	applyModuleOverride(bi, "custom-mod", 0)
	if bi.Modules[0].Id != "custom-mod" {
		t.Errorf("module[0].Id = %q, want custom-mod", bi.Modules[0].Id)
	}
	if bi.Modules[1].Id != "second:2.0.0" {
		t.Errorf("module[1].Id changed to %q, want second:2.0.0 (only idx 0 renamed)", bi.Modules[1].Id)
	}
	// Override a non-zero index (workspace member).
	applyModuleOverride(bi, "renamed-second", 1)
	if bi.Modules[1].Id != "renamed-second" {
		t.Errorf("module[1].Id = %q, want renamed-second", bi.Modules[1].Id)
	}
	// Empty override -> unchanged.
	applyModuleOverride(bi, "", 0)
	if bi.Modules[0].Id != "custom-mod" {
		t.Errorf("empty override changed id to %q", bi.Modules[0].Id)
	}
	// Out-of-range / no modules / nil -> no panic.
	applyModuleOverride(bi, "x", 5)
	applyModuleOverride(bi, "x", -1)
	applyModuleOverride(&entities.BuildInfo{}, "x", 0)
	applyModuleOverride(nil, "x", 0)
}

func TestApplyModuleOverrideRewritesRequestedBy(t *testing.T) {
	// requestedBy paths anchor at the module id; renaming the module must rewrite those anchors so
	// the "requestedBy terminal == module.Id" invariant holds.
	bi := &entities.BuildInfo{Modules: []entities.Module{{
		Id: "abc:0.1.0",
		Dependencies: []entities.Dependency{
			{Id: "serde_json-1.0.0.crate", RequestedBy: [][]string{{"abc:0.1.0"}}},
			{Id: "itoa-1.0.0.crate", RequestedBy: [][]string{{"serde_json-1.0.0.crate", "abc:0.1.0"}}},
		},
	}}}
	applyModuleOverride(bi, "abc-custom", 0)
	if bi.Modules[0].Id != "abc-custom" {
		t.Fatalf("module id = %q, want abc-custom", bi.Modules[0].Id)
	}
	// Direct dep: terminal anchor rewritten.
	if got := bi.Modules[0].Dependencies[0].RequestedBy; got[0][0] != "abc-custom" {
		t.Errorf("direct dep requestedBy = %v, want terminal abc-custom", got)
	}
	// Transitive dep: only the terminal anchor (old module id) is rewritten, intermediate crate ids stay.
	tr := bi.Modules[0].Dependencies[1].RequestedBy[0]
	if tr[0] != "serde_json-1.0.0.crate" || tr[1] != "abc-custom" {
		t.Errorf("transitive requestedBy = %v, want [serde_json-1.0.0.crate abc-custom]", tr)
	}
}

func TestIsDryRunPublish(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"publish", "--dry-run"}, true},
		{[]string{"publish", "-n"}, true},
		{[]string{"publish", "--registry", "jfrog", "--dry-run", "--allow-dirty"}, true},
		{[]string{"publish", "--registry", "jfrog"}, false},
		{[]string{"install", "--path", "."}, false},
	}
	for _, c := range cases {
		if got := isDryRunPublish(c.args); got != c.want {
			t.Errorf("isDryRunPublish(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestModuleOverrideIndex(t *testing.T) {
	bi := &entities.BuildInfo{Modules: []entities.Module{
		{Id: "member-x:0.1.0"},
		{Id: "member-y:0.1.0"},
	}}
	// -p selects the member whose module should be renamed.
	if got := moduleOverrideIndex(bi, []string{"publish", "-p", "member-y"}); got != 1 {
		t.Errorf("-p member-y -> index %d, want 1", got)
	}
	if got := moduleOverrideIndex(bi, []string{"publish", "--package=member-x"}); got != 0 {
		t.Errorf("--package=member-x -> index %d, want 0", got)
	}
	// No -p -> primary module (index 0).
	if got := moduleOverrideIndex(bi, []string{"install"}); got != 0 {
		t.Errorf("no -p -> index %d, want 0", got)
	}
	// -p naming an unknown package -> fallback to 0.
	if got := moduleOverrideIndex(bi, []string{"publish", "-p", "nope"}); got != 0 {
		t.Errorf("-p unknown -> index %d, want 0", got)
	}
	// Empty build-info -> 0.
	if got := moduleOverrideIndex(&entities.BuildInfo{}, []string{"publish", "-p", "x"}); got != 0 {
		t.Errorf("empty bi -> index %d, want 0", got)
	}
}

func TestRegistryNameFromArgs(t *testing.T) {
	if got := registryNameFromArgs([]string{"publish", "--registry", "my-crates"}); got != "my-crates" {
		t.Errorf("got %q", got)
	}
	if got := registryNameFromArgs([]string{"publish", "--registry=other"}); got != "other" {
		t.Errorf("eq form: got %q", got)
	}
	if got := registryNameFromArgs([]string{"build"}); got != "" {
		t.Errorf("none: got %q", got)
	}
}

func TestMetadataFlagsFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "bool flag in args",
			args: []string{"build", "--all-features", "-v"},
			want: []string{"--all-features"},
		},
		{
			name: "value flag with space",
			args: []string{"build", "--features", "a,b", "--locked"},
			want: []string{"--features", "a,b", "--locked"},
		},
		{
			name: "value flag with equals",
			args: []string{"build", "--features=x", "--manifest-path", "./Cargo.toml"},
			want: []string{"--features=x", "--manifest-path", "./Cargo.toml"},
		},
		{
			name: "no metadata flags",
			args: []string{"build"},
			want: nil,
		},
		{
			name: "value flag at end with no following token",
			args: []string{"build", "--features"},
			want: []string{"--features"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataFlagsFromArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
