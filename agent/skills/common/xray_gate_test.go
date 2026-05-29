package common

import (
	"testing"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/stretchr/testify/assert"
)

func TestCheckXrayGate_SkipScan(t *testing.T) {
	err := CheckXrayGate(XrayGateParams{SkipScan: true})
	assert.NoError(t, err)
}

func TestCheckXrayGate_SkipViaEnv(t *testing.T) {
	t.Setenv(envSkipSkillsScan, "true")
	err := CheckXrayGate(XrayGateParams{})
	assert.NoError(t, err)
}

func TestResolveTimeout_Default(t *testing.T) {
	t.Setenv(envScanTimeout, "")
	d := resolveTimeout()
	assert.Equal(t, defaultXrayGateTimeout, d)
}

func TestResolveTimeout_Custom(t *testing.T) {
	t.Setenv(envScanTimeout, "10m")
	d := resolveTimeout()
	assert.Equal(t, 10*time.Minute, d)
}

func TestResolveTimeout_Invalid(t *testing.T) {
	t.Setenv(envScanTimeout, "not-a-duration")
	d := resolveTimeout()
	assert.Equal(t, defaultXrayGateTimeout, d, "invalid value should fall back to default")
}

func TestHandleBlocked_NoAutoDelete(t *testing.T) {
	params := XrayGateParams{
		Slug:                "test-skill",
		Version:             "1.0.0",
		AutoDeleteOnFailure: false,
	}
	err := handleBlocked(nil, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked by Xray security scan")
}

func TestXrayStatusConstants(t *testing.T) {
	// Verify constants match expected values from the API spec.
	assert.Equal(t, "NOT_IN_ENTITLEMENT", services.SkillXrayStatusNotInEntitlement)
	assert.Equal(t, "XRAY_DISABLED_FOR_REPO", services.SkillXrayStatusDisabledForRepo)
	assert.Equal(t, "SCAN_IN_PROGRESS", services.SkillXrayStatusScanInProgress)
	assert.Equal(t, "BLOCKED", services.SkillXrayStatusBlocked)
	assert.Equal(t, "APPROVED", services.SkillXrayStatusApproved)
}
