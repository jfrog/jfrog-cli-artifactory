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
	_, err = cm.Id(image)
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
