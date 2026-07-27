package publish

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithPackageFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "bare positional package spec gets promoted",
			args: []string{"jfrog/proj3"},
			want: []string{"--package", "jfrog/proj3"},
		},
		{
			name: "positional spec mixed with flags",
			args: []string{"--dry-run", "jfrog/proj3"},
			want: []string{"--package", "jfrog/proj3", "--dry-run"},
		},
		{
			name: "already has --package flag - untouched",
			args: []string{"--package", "jfrog/proj3"},
			want: []string{"--package", "jfrog/proj3"},
		},
		{
			name: "already has --package= form - untouched",
			args: []string{"--package=jfrog/proj3"},
			want: []string{"--package=jfrog/proj3"},
		},
		{
			name: "no positional args - untouched",
			args: []string{"--dry-run"},
			want: []string{"--dry-run"},
		},
		{
			name: "empty args",
			args: []string{},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, withPackageFlag(tt.args))
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
