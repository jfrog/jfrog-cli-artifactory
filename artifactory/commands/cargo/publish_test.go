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
	cases := map[string]string{
		"jf-cargo-sample:0.1.0": "jf-cargo-sample-0.1.0.crate",
		"serde:1.0.228":         "serde-1.0.228.crate",
		"noversion":             "", // no ":" -> cannot derive
		"trailing:":             "", // empty version
	}
	for id, want := range cases {
		if got := publishedCrateFileName(mk(id)); got != want {
			t.Errorf("publishedCrateFileName(%q) = %q, want %q", id, got, want)
		}
	}
	if got := publishedCrateFileName(&entities.BuildInfo{}); got != "" {
		t.Errorf("no modules: got %q, want empty", got)
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

func TestSetModuleCommandProperties(t *testing.T) {
	t.Run("set command and args", func(t *testing.T) {
		bi := &entities.BuildInfo{
			Modules: []entities.Module{
				{Id: "m", Type: entities.Cargo},
			},
		}
		setModuleCommandProperties(bi, "build", []string{"build", "--all-features"})

		props, ok := bi.Modules[0].Properties.(map[string]string)
		if !ok {
			t.Fatalf("Properties is not map[string]string, got %T", bi.Modules[0].Properties)
		}
		if props["cargo.command"] != "build" {
			t.Errorf("cargo.command: got %q, want %q", props["cargo.command"], "build")
		}
		if props["cargo.args"] != "build --all-features" {
			t.Errorf("cargo.args: got %q, want %q", props["cargo.args"], "build --all-features")
		}
	})

	t.Run("nil build info", func(t *testing.T) {
		setModuleCommandProperties(nil, "build", nil)
	})

	t.Run("empty modules", func(t *testing.T) {
		bi := &entities.BuildInfo{}
		setModuleCommandProperties(bi, "build", nil)
	})
}
