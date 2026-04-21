package ocicontainer

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func getImageSizeMB(t *testing.T, imageName string) uint64 {
	cmd := exec.Command("docker", "inspect", "--format", "{{.Size}}", imageName)
	out, err := cmd.Output()
	require.NoError(t, err)
	sizeBytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	require.NoError(t, err)
	return sizeBytes / 1024 / 1024
}

// Verifies that Id() does not buffer the entire image in memory (OOM fix for #3382).
func TestDockerClientId_MemoryUsage(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker daemon not available")
	}

	cmd := exec.Command("docker", "pull", "busybox:latest")
	if err := cmd.Run(); err != nil {
		t.Skip("Cannot pull Docker images (no Docker engine or network access)")
	}

	imageSizeMB := getImageSizeMB(t, "busybox:latest")
	t.Logf("Image size: %d MB", imageSizeMB)

	ref, err := name.ParseReference("busybox:latest")
	require.NoError(t, err)

	// --- Buffered (default, the bug) ---
	runtime.GC()
	var memBeforeBuffered runtime.MemStats
	runtime.ReadMemStats(&memBeforeBuffered)

	bufferedImg, err := daemon.Image(ref)
	require.NoError(t, err)
	_, err = bufferedImg.Manifest()
	require.NoError(t, err)

	var memAfterBuffered runtime.MemStats
	runtime.ReadMemStats(&memAfterBuffered)
	bufferedAllocMB := (memAfterBuffered.TotalAlloc - memBeforeBuffered.TotalAlloc) / 1024 / 1024
	t.Logf("[BUFFERED]   Memory allocated: %d MB (default — causes OOM on large images)", bufferedAllocMB)

	// --- Unbuffered (the fix) ---
	runtime.GC()
	var memBeforeUnbuffered runtime.MemStats
	runtime.ReadMemStats(&memBeforeUnbuffered)

	cm := &containerManager{Type: DockerClient}
	image := NewImage("busybox:latest")
	_, err = cm.Id(image, Push)
	require.NoError(t, err)

	var memAfterUnbuffered runtime.MemStats
	runtime.ReadMemStats(&memAfterUnbuffered)
	unbufferedAllocMB := (memAfterUnbuffered.TotalAlloc - memBeforeUnbuffered.TotalAlloc) / 1024 / 1024
	t.Logf("[UNBUFFERED] Memory allocated: %d MB (fix — streams without buffering)", unbufferedAllocMB)

	t.Logf("Savings: %d MB", bufferedAllocMB-unbufferedAllocMB)

	// Unbuffered should allocate less than half the image size
	threshold := imageSizeMB / 2
	if threshold < 10 {
		threshold = 10
	}
	assert.Less(t, unbufferedAllocMB, threshold,
		"Id() allocated %d MB for a %d MB image — may be buffering entire image in memory (OOM risk for large images)", unbufferedAllocMB, imageSizeMB)
}

// Push + skip env=true => short-circuit, no daemon call, empty id.
func TestDockerClientId_PushWithSkipEnvTrue_ShortCircuits(t *testing.T) {
	t.Setenv(SkipDockerImageIdVerificationEnv, "true")

	cm := &containerManager{Type: DockerClient}
	id, err := cm.Id(NewImage("any-image-that-does-not-exist:latest"), Push)
	require.NoError(t, err, "Id() must not error when the daemon is not consulted")
	assert.Empty(t, id, "Id() must return an empty string when skip is opted-in on a push")
}

// Push + skip env unset => daemon path is taken. Prove by passing a
// deliberately invalid image name: we expect name.ParseReference to reject it.
func TestDockerClientId_PushWithoutSkipEnv_GoesToDaemonPath(t *testing.T) {
	t.Setenv(SkipDockerImageIdVerificationEnv, "")

	cm := &containerManager{Type: DockerClient}
	_, err := cm.Id(NewImage("INVALID NAME WITH SPACE"), Push)
	require.Error(t, err, "Id() must reach name.ParseReference on the default path")
}

// Push + skip env=false (any non-truthy value) => default verify path.
func TestDockerClientId_PushWithSkipEnvFalse_GoesToDaemonPath(t *testing.T) {
	t.Setenv(SkipDockerImageIdVerificationEnv, "false")

	cm := &containerManager{Type: DockerClient}
	_, err := cm.Id(NewImage("INVALID NAME WITH SPACE"), Push)
	require.Error(t, err, "Id() must reach name.ParseReference when skip is explicitly false")
}

// Pull must IGNORE the skip env and always take the verify path. This is the
// reviewer-requested contract: the OOM/skipping opt-out only applies to push.
func TestDockerClientId_PullWithSkipEnvTrue_StillVerifies(t *testing.T) {
	t.Setenv(SkipDockerImageIdVerificationEnv, "true")

	cm := &containerManager{Type: DockerClient}
	_, err := cm.Id(NewImage("INVALID NAME WITH SPACE"), Pull)
	require.Error(t, err, "Pull must always verify; name.ParseReference should reject the malformed name")
}

// Non-truthy env values must all be treated as "do not skip" (strconv.ParseBool
// semantics: everything except 1/t/T/TRUE/true/True returns false or an error).
func TestDockerClientId_PushWithNonBoolSkipEnv_GoesToDaemonPath(t *testing.T) {
	t.Setenv(SkipDockerImageIdVerificationEnv, "anything-other-than-true")

	cm := &containerManager{Type: DockerClient}
	_, err := cm.Id(NewImage("INVALID NAME WITH SPACE"), Push)
	require.Error(t, err, "strconv.ParseBool returns an error for non-bool values; must fall through to daemon path")
}
