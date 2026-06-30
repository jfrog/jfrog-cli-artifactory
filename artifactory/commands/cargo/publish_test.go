package cargo

import "testing"

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
