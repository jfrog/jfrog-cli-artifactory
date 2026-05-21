package common

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEvidenceLicenseError(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		isLicenseErr bool
	}{
		{
			name:         "403 Forbidden with Enterprise+ message",
			errMsg:       `upload failed: server response: 403 Forbidden\n{"errors":[{"message":"evidence deployment requires an Enterprise+ license"}]}`,
			isLicenseErr: true,
		},
		{
			name:         "Enterprise+ only",
			errMsg:       "evidence deployment requires an Enterprise+ license",
			isLicenseErr: true,
		},
		{
			name:         "403 Forbidden only",
			errMsg:       "server response: 403 Forbidden",
			isLicenseErr: false,
		},
		{
			name:         "network error",
			errMsg:       "connection refused",
			isLicenseErr: false,
		},
		{
			name:         "401 unauthorized",
			errMsg:       "server response: 401 Unauthorized",
			isLicenseErr: false,
		},
		{
			name:         "signing key error",
			errMsg:       "failed to read signing key: no such file",
			isLicenseErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errMsg)
			assert.Equal(t, tt.isLicenseErr, IsEvidenceLicenseError(err), "for error: %s", tt.errMsg)
		})
	}
}
