package dockerfileutils

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExtractBaseImageStageAndPlatformFromLine(t *testing.T) {
	defaultOS := runtime.GOOS
	if defaultOS == "darwin" {
		defaultOS = "linux"
	}
	defaultArch := runtime.GOARCH

	tests := []struct {
		name          string
		line          string
		expectedImage string
		expectedStage string
		expectedOS    string
		expectedArch  string
	}{
		{
			name:          "Simple FROM",
			line:          "FROM ubuntu:20.04",
			expectedImage: "ubuntu:20.04",
			expectedStage: "",
			expectedOS:    defaultOS,
			expectedArch:  defaultArch,
		},
		{
			name:          "FROM with AS",
			line:          "FROM ubuntu:20.04 AS builder",
			expectedImage: "ubuntu:20.04",
			expectedStage: "builder",
			expectedOS:    defaultOS,
			expectedArch:  defaultArch,
		},
		{
			name:          "FROM with --platform flag",
			line:          "FROM --platform=linux/amd64 ubuntu:20.04",
			expectedImage: "ubuntu:20.04",
			expectedStage: "",
			expectedOS:    "linux",
			expectedArch:  "amd64",
		},
		{
			name:          "FROM with --platform and AS",
			line:          "FROM --platform=linux/amd64 ubuntu:20.04 AS builder",
			expectedImage: "ubuntu:20.04",
			expectedStage: "builder",
			expectedOS:    "linux",
			expectedArch:  "amd64",
		},
		{
			name:          "FROM with registry and --platform",
			line:          "FROM --platform=linux/amd64 ecosysjfrog.jfrog.io/docker-remote/nginx:latest",
			expectedImage: "ecosysjfrog.jfrog.io/docker-remote/nginx:latest",
			expectedStage: "",
			expectedOS:    "linux",
			expectedArch:  "amd64",
		},
		{
			name:          "FROM scratch",
			line:          "FROM scratch",
			expectedImage: "scratch",
			expectedStage: "",
			expectedOS:    defaultOS,
			expectedArch:  defaultArch,
		},
		{
			name:          "FROM scratch with AS",
			line:          "FROM scratch AS base",
			expectedImage: "scratch",
			expectedStage: "base",
			expectedOS:    defaultOS,
			expectedArch:  defaultArch,
		},
		{
			name:          "FROM with multiple flags",
			line:          "FROM --platform=linux/amd64 --some-flag=value alpine:3.18",
			expectedImage: "alpine:3.18",
			expectedStage: "",
			expectedOS:    "linux",
			expectedArch:  "amd64",
		},
		{
			name:          "FROM with --platform=linux/arm64",
			line:          "FROM --platform=linux/arm64 alpine:3.18",
			expectedImage: "alpine:3.18",
			expectedStage: "",
			expectedOS:    "linux",
			expectedArch:  "arm64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, stage, os, arch := extractBaseImageStageAndPlatformFromLine(tt.line, defaultOS, defaultArch)
			if image != tt.expectedImage {
				t.Errorf("extractBaseImageStageAndPlatformFromLine(%q) image = %q, want %q", tt.line, image, tt.expectedImage)
			}
			if stage != tt.expectedStage {
				t.Errorf("extractBaseImageStageAndPlatformFromLine(%q) stage = %q, want %q", tt.line, stage, tt.expectedStage)
			}
			if os != tt.expectedOS {
				t.Errorf("extractBaseImageStageAndPlatformFromLine(%q) os = %q, want %q", tt.line, os, tt.expectedOS)
			}
			if arch != tt.expectedArch {
				t.Errorf("extractBaseImageStageAndPlatformFromLine(%q) arch = %q, want %q", tt.line, arch, tt.expectedArch)
			}
		})
	}
}

func TestParseDockerfileBaseImages(t *testing.T) {
	// Create a temporary Dockerfile with various FROM instructions
	dockerfileContent := `# Multi-stage build example
FROM --platform=linux/amd64 ecosysjfrog.jfrog.io/docker-remote/nginx:latest AS base

FROM ubuntu:20.04 AS builder
RUN echo "building"

FROM alpine:3.18
COPY --from=builder /app /app

FROM scratch AS final
`

	// Create temporary file
	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test Dockerfile: %v", err)
	}

	// Parse the Dockerfile
	baseImages, err := ParseDockerfileBaseImages(dockerfilePath)
	if err != nil {
		t.Fatalf("ParseDockerfileBaseImages failed: %v", err)
	}

	// Verify results
	expected := []struct {
		image string
		os    string
		arch  string
	}{
		{"ecosysjfrog.jfrog.io/docker-remote/nginx:latest", "linux", "amd64"},
		{"ubuntu:20.04", runtime.GOOS, runtime.GOARCH},
		{"alpine:3.18", runtime.GOOS, runtime.GOARCH},
		// scratch should be skipped
	}
	if runtime.GOOS == "darwin" {
		expected[1].os = "linux"
		expected[2].os = "linux"
	}

	if len(baseImages) != len(expected) {
		t.Errorf("Expected %d base images, got %d: %v", len(expected), len(baseImages), baseImages)
	}

	for i, exp := range expected {
		if i < len(baseImages) {
			if baseImages[i].Image != exp.image {
				t.Errorf("Base image[%d].Image = %q, want %q", i, baseImages[i].Image, exp.image)
			}
			if baseImages[i].OS != exp.os {
				t.Errorf("Base image[%d].OS = %q, want %q", i, baseImages[i].OS, exp.os)
			}
			if baseImages[i].Architecture != exp.arch {
				t.Errorf("Base image[%d].Architecture = %q, want %q", i, baseImages[i].Architecture, exp.arch)
			}
		}
	}
}

func TestParseDockerfileBaseImages_MultiStageReferences(t *testing.T) {
	// Test Dockerfile where FROM references previous stages
	dockerfileContent := `# Multi-stage build with stage references
FROM golang:1.18 AS builder
WORKDIR /app
COPY . .
RUN go build -o app

FROM builder AS test
RUN ./app test

FROM alpine:3.18 AS runtime
COPY --from=builder /app/app /usr/local/bin/app

FROM runtime AS final
RUN echo "Final stage"
`

	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test Dockerfile: %v", err)
	}

	baseImages, err := ParseDockerfileBaseImages(dockerfilePath)
	if err != nil {
		t.Fatalf("ParseDockerfileBaseImages failed: %v", err)
	}

	// Should only include actual base images, not stage references
	expectedImages := []string{
		"golang:1.18",
		"alpine:3.18",
		// "builder" and "runtime" should be skipped as they reference previous stages
	}

	if len(baseImages) != len(expectedImages) {
		t.Errorf("Expected %d base images, got %d: %v", len(expectedImages), len(baseImages), baseImages)
	}

	for i, expectedImage := range expectedImages {
		if i < len(baseImages) && baseImages[i].Image != expectedImage {
			t.Errorf("Base image[%d].Image = %q, want %q", i, baseImages[i].Image, expectedImage)
		}
	}
}

func TestParseDockerfileBaseImages_StageNameCollision(t *testing.T) {
	// Test edge case: stage name that matches an image name
	dockerfileContent := `# Stage name matches image name
FROM ubuntu:20.04 AS ubuntu
FROM ubuntu AS final
`

	tmpDir := t.TempDir()
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test Dockerfile: %v", err)
	}

	baseImages, err := ParseDockerfileBaseImages(dockerfilePath)
	if err != nil {
		t.Fatalf("ParseDockerfileBaseImages failed: %v", err)
	}

	// Should only include the first ubuntu:20.04, not the second FROM ubuntu
	expectedImage := "ubuntu:20.04"

	if len(baseImages) != 1 {
		t.Errorf("Expected 1 base image, got %d: %v", len(baseImages), baseImages)
	}

	if len(baseImages) > 0 && baseImages[0].Image != expectedImage {
		t.Errorf("Base image[0].Image = %q, want %q", baseImages[0].Image, expectedImage)
	}
}
