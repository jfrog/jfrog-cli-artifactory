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

func TestZipPathFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "--zip with space form", args: []string{"--zip", "./build/my-package-1.0.0.zip"}, want: "./build/my-package-1.0.0.zip"},
		{name: "--zip= form", args: []string{"--zip=./build/custom.zip"}, want: "./build/custom.zip"},
		{name: "no --zip flag", args: []string{"--package", "acme/skills-pack"}, want: ""},
		{name: "--zip as last arg with no value", args: []string{"--zip"}, want: ""},
		{name: "empty args", args: []string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, zipPathFromArgs(tt.args))
		})
	}
}
