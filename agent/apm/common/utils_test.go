package apmcommon

import (
	"testing"

	"github.com/jfrog/gofrog/version"
	"github.com/stretchr/testify/assert"
)

// TestParseApmVersion is a regression test: GetApmVersion used to feed apm's entire
// descriptive `--version` output straight into version.NewVersion, which made
// ValidateApmPrerequisites's min-version check pass or fail by accident of string shape
// rather than by actually comparing semantic versions (verified against the real installed
// apm binary during review). This confirms parseApmVersion extracts just the dotted version
// number token.
func TestParseApmVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "real apm --version output",
			output: "Agent Package Manager (APM) CLI version 0.23.1 (d1d926d)\n",
			want:   "0.23.1",
		},
		{
			name:   "bare version",
			output: "0.1.0\n",
			want:   "0.1.0",
		},
		{
			name:   "two-segment version",
			output: "apm version 1.2\n",
			want:   "1.2",
		},
		{
			name:   "no version present",
			output: "apm: command not found\n",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseApmVersion(tt.output))
		})
	}
}

func TestIsDottedVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "three segments", in: "0.23.1", want: true},
		{name: "two segments", in: "1.2", want: true},
		{name: "one segment", in: "1", want: false},
		{name: "four segments", in: "1.2.3.4", want: false},
		{name: "trailing dot with empty segment", in: "1.2.", want: false},
		{name: "non-numeric segment", in: "1.2a", want: false},
		{name: "v-prefixed", in: "v1.2.3", want: false},
		{name: "empty string", in: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDottedVersion(tt.in))
		})
	}
}

func TestValidateApmPrerequisites_VersionComparisonDirection(t *testing.T) {
	// Exercises the exact version.AtLeast call ValidateApmPrerequisites makes, using a
	// correctly-extracted version string (the fix for the bug above): a version at or above
	// the minimum must not trigger the "needs update" error path, and one below it must.
	tests := []struct {
		name      string
		rawOutput string
		wantError bool
	}{
		{
			name:      "modern installed version satisfies the minimum",
			rawOutput: "Agent Package Manager (APM) CLI version 0.23.1 (d1d926d)",
			wantError: false,
		},
		{
			name:      "installed version below the minimum",
			rawOutput: "Agent Package Manager (APM) CLI version 0.0.5 (abc1234)",
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed := parseApmVersion(tt.rawOutput)
			require := assert.New(t)
			require.NotEmpty(installed)

			ver := version.NewVersion(installed)
			gotError := !ver.AtLeast(minSupportedApmVersion)
			assert.Equal(t, tt.wantError, gotError)
		})
	}
}
