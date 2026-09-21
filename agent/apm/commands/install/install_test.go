package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootDirFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "--root with space form", args: []string{"--root", "./out"}, want: "./out"},
		{name: "--root= form", args: []string{"--root=/tmp/build"}, want: "/tmp/build"},
		{name: "no --root flag", args: []string{"--dry-run"}, want: ""},
		{name: "--root as last arg with no value", args: []string{"--root"}, want: ""},
		{name: "empty args", args: []string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, rootDirFromArgs(tt.args))
		})
	}
}
