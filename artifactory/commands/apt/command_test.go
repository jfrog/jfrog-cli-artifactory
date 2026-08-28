package apt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── needsUpdate ───────────────────────────────────────────────────────────────

func TestNeedsUpdate_Install(t *testing.T) {
	assert.True(t, needsUpdate([]string{"install", "curl"}))
}

func TestNeedsUpdate_Upgrade(t *testing.T) {
	assert.True(t, needsUpdate([]string{"upgrade"}))
}

func TestNeedsUpdate_DistUpgrade(t *testing.T) {
	assert.True(t, needsUpdate([]string{"dist-upgrade"}))
}

func TestNeedsUpdate_FullUpgrade(t *testing.T) {
	assert.True(t, needsUpdate([]string{"full-upgrade"}))
}

func TestNeedsUpdate_Satisfy(t *testing.T) {
	assert.True(t, needsUpdate([]string{"satisfy", "curl (>= 7.0)"}))
}

func TestNeedsUpdate_Remove(t *testing.T) {
	assert.False(t, needsUpdate([]string{"remove", "curl"}))
}

func TestNeedsUpdate_Purge(t *testing.T) {
	assert.False(t, needsUpdate([]string{"purge", "curl"}))
}

func TestNeedsUpdate_Show(t *testing.T) {
	assert.False(t, needsUpdate([]string{"show", "curl"}))
}

func TestNeedsUpdate_List(t *testing.T) {
	assert.False(t, needsUpdate([]string{"list", "--installed"}))
}

func TestNeedsUpdate_Autoremove(t *testing.T) {
	assert.False(t, needsUpdate([]string{"autoremove"}))
}

func TestNeedsUpdate_LeadingFlags(t *testing.T) {
	// flags before subcommand should be skipped
	assert.True(t, needsUpdate([]string{"-y", "--quiet", "install", "curl"}))
}

func TestNeedsUpdate_EmptyArgs(t *testing.T) {
	assert.False(t, needsUpdate([]string{}))
}

func TestNeedsUpdate_OnlyFlags(t *testing.T) {
	assert.False(t, needsUpdate([]string{"-y", "--quiet"}))
}

func TestNeedsUpdate_TwoTokenValueFlag(t *testing.T) {
	// `-o` takes its value as a separate argv token; the value must not be
	// mistaken for the subcommand, so `install` following it is still detected.
	assert.True(t, needsUpdate([]string{"-o", "Debug::pkgProblemResolver=1", "install", "curl"}))
	assert.True(t, needsUpdate([]string{"-t", "noble-backports", "install", "curl"}))
}

func TestNeedsUpdate_TwoTokenValueFlagBeforeNonUpdate(t *testing.T) {
	assert.False(t, needsUpdate([]string{"-o", "Foo=bar", "remove", "curl"}))
}

// ── AptCommand setters/defaults ───────────────────────────────────────────────

func TestSetComponent_DefaultsToMain(t *testing.T) {
	cmd := NewAptCommand().SetComponent("")
	assert.Equal(t, "main", cmd.component)
}

func TestSetComponent_CustomValue(t *testing.T) {
	cmd := NewAptCommand().SetComponent("contrib")
	assert.Equal(t, "contrib", cmd.component)
}

func TestRun_NoArgs(t *testing.T) {
	cmd := NewAptCommand().SetArgs([]string{})
	err := cmd.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no apt arguments")
}
