package cargo

import (
	"reflect"
	"testing"
)

func TestCommandBucket(t *testing.T) {
	cases := map[string]string{
		"build": "deps", "install": "deps", "update": "deps", "add": "deps", "fetch": "deps",
		"package": "artifacts", "publish": "publish",
		"metadata": "none", "tree": "none", "search": "none", "--version": "none",
	}
	for cmd, want := range cases {
		if got := commandBucket(cmd); got != want {
			t.Errorf("commandBucket(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestCargoRegistryEnvKey(t *testing.T) {
	if got := cargoRegistryEnvKey("my-crates"); got != "CARGO_REGISTRIES_MY_CRATES_TOKEN" {
		t.Errorf("got %q", got)
	}
}

func TestBuildAuthEnv(t *testing.T) {
	got := buildAuthEnv("my-crates", "abc")
	want := []string{`CARGO_REGISTRIES_MY_CRATES_TOKEN=Bearer abc`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(buildAuthEnv("", "abc")) != 0 || len(buildAuthEnv("my-crates", "")) != 0 {
		t.Error("expected empty env when registry or token missing")
	}
}
