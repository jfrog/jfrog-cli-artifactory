package publish

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequirePackageFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "--package with space form present", args: []string{"--package", "jfrog/proj3"}, wantErr: false},
		{name: "--package= form present", args: []string{"--package=jfrog/proj3"}, wantErr: false},
		{name: "--package present alongside other flags", args: []string{"--dry-run", "--package", "jfrog/proj3"}, wantErr: false},
		{name: "missing --package entirely", args: []string{"--dry-run"}, wantErr: true},
		{name: "empty args", args: []string{}, wantErr: true},
		{
			name:    "bare positional package spec is no longer auto-promoted - requires an explicit error",
			args:    []string{"jfrog/proj3"},
			wantErr: true,
		},
		{
			name:    "a value-taking flag's value is never mistaken for --package - still requires it explicitly",
			args:    []string{"--zip", "foo.zip", "jfrog/proj3"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requirePackageFlag(tt.args)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOwnerFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "--package with space form", args: []string{"--package", "jfrog/proj3"}, want: "jfrog"},
		{name: "--package= form", args: []string{"--package=acme/skills-pack"}, want: "acme"},
		{name: "no --package flag", args: []string{"--dry-run"}, want: ""},
		{name: "--package without a slash", args: []string{"--package", "standalone-name"}, want: ""},
		{name: "--package as last arg with no value", args: []string{"--package"}, want: ""},
		{name: "empty args", args: []string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ownerFromArgs(tt.args))
		})
	}
}
