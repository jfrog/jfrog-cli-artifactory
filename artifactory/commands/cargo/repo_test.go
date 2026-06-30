package cargo

import "testing"

func TestExtractRepoNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://acme.jfrog.io/artifactory/api/cargo/cargo-local":         "cargo-local",
		"https://acme.jfrog.io/artifactory/api/cargo/cargo-local/index/":  "cargo-local",
		"https://acme.jfrog.io/artifactory/cargo-virtual":                 "cargo-virtual",
		"sparse+https://acme.jfrog.io/artifactory/api/cargo/cargo-v/":     "cargo-v",
		"cargo/foo":  "foo",
	}
	for url, want := range cases {
		if got := extractRepoNameFromURL(url); got != want {
			t.Errorf("extractRepoNameFromURL(%q) = %q, want %q", url, got, want)
		}
	}
}

type fakeRepoGetter struct {
	rclass    string
	defaultDR string
	err       error
}

func (f fakeRepoGetter) GetRepository(_ string, target interface{}) error {
	if f.err != nil {
		return f.err
	}
	p := target.(*servicesVirtualParams)
	p.Rclass = f.rclass
	p.DefaultDeploymentRepo = f.defaultDR
	return nil
}

func TestResolveDeploymentRepo(t *testing.T) {
	// non-virtual passes through
	if got := resolveDeploymentRepo("local-repo", fakeRepoGetter{rclass: "local"}); got != "local-repo" {
		t.Errorf("local: got %q", got)
	}
	// virtual resolves to default deployment repo
	if got := resolveDeploymentRepo("virt", fakeRepoGetter{rclass: "virtual", defaultDR: "local-deploy"}); got != "local-deploy" {
		t.Errorf("virtual: got %q, want local-deploy", got)
	}
	// virtual without default -> ""
	if got := resolveDeploymentRepo("virt", fakeRepoGetter{rclass: "virtual"}); got != "" {
		t.Errorf("virtual no-default: got %q, want empty", got)
	}
}
