package flexpack

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseArtifactModifiedTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(t *testing.T, result time.Time)
	}{
		{
			name:    "RFC3339 format",
			input:   "2024-01-15T10:30:00Z",
			wantErr: false,
			validate: func(t *testing.T, result time.Time) {
				assert.Equal(t, 2024, result.Year())
				assert.Equal(t, time.January, result.Month())
				assert.Equal(t, 15, result.Day())
			},
		},
		{
			name:    "RFC3339 with timezone offset",
			input:   "2024-01-15T10:30:00+05:30",
			wantErr: false,
		},
		{
			name:    "RFC3339Nano format",
			input:   "2024-01-15T10:30:00.123456789Z",
			wantErr: false,
		},
		{
			name:    "ISO8601 with milliseconds and Z",
			input:   "2024-01-15T10:30:00.123Z",
			wantErr: false,
		},
		{
			name:    "Build info format",
			input:   "2024-01-15T10:30:00.000-0700",
			wantErr: false,
		},
		{
			name:    "ISO8601 with milliseconds and timezone",
			input:   "2024-01-15T10:30:00.999-07:00",
			wantErr: false,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Invalid format - plain text",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "Invalid format - partial date",
			input:   "2024-01-15",
			wantErr: true,
		},
		{
			name:    "Invalid format - Unix timestamp",
			input:   "1705312200",
			wantErr: true,
		},
		{
			name:    "Invalid format - wrong separator",
			input:   "2024/01/15T10:30:00Z",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseArtifactModifiedTime(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, result.IsZero())
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}
