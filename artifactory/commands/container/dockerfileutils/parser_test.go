package dockerfileutils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBaseImageAndStageFromLine(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		expectedImage  string
		expectedStage  string
	}{
		{
			name:          "Simple FROM",
			line:          "FROM ubuntu:20.04",
			expectedImage: "ubuntu:20.04",
			expectedStage: "",
		},
		{
			name:          "FROM with AS",
			line:          "FROM ubuntu:20.04 AS builder",
			expectedImage: "ubuntu:20.04",
			expectedStage: "builder",
		},
		{
			name:          "FROM with --platform flag",
			line:          "FROM --platform=linux/amd64 ubuntu:20.04",
			expectedImage: "ubuntu:20.04",
			expectedStage: "",
		},
		{
			name:          "FROM with --platform and AS",
			line:          "FROM --platform=linux/amd64 ubuntu:20.04 AS builder",
			expectedImage: "ubuntu:20.04",
			expectedStage: "builder",
		},
		{
			name:          "FROM with registry and --platform",
			line:          "FROM --platform=linux/amd64 ecosysjfrog.jfrog.io/docker-remote/nginx:latest",
			expectedImage: "ecosysjfrog.jfrog.io/docker-remote/nginx:latest",
			expectedStage: "",
		},
		{
			name:          "FROM scratch",
			line:          "FROM scratch",
			expectedImage: "scratch",
			expectedStage: "",
		},
		{
			name:          "FROM scratch with AS",
			line:          "FROM scratch AS base",
			expectedImage: "scratch",
			expectedStage: "base",
		},
		{
			name:          "FROM with multiple flags",
			line:          "FROM --platform=linux/amd64 --some-flag=value alpine:3.18",
			expectedImage: "alpine:3.18",
			expectedStage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, stage := extractBaseImageAndStageFromLine(tt.line)
			if image != tt.expectedImage {
				t.Errorf("extractBaseImageAndStageFromLine(%q) image = %q, want %q", tt.line, image, tt.expectedImage)
			}
			if stage != tt.expectedStage {
				t.Errorf("extractBaseImageAndStageFromLine(%q) stage = %q, want %q", tt.line, stage, tt.expectedStage)
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
	expected := []string{
		"ecosysjfrog.jfrog.io/docker-remote/nginx:latest",
		"ubuntu:20.04",
		"alpine:3.18",
		// scratch should be skipped
	}

	if len(baseImages) != len(expected) {
		t.Errorf("Expected %d base images, got %d: %v", len(expected), len(baseImages), baseImages)
	}

	for i, expectedImage := range expected {
		if i < len(baseImages) && baseImages[i] != expectedImage {
			t.Errorf("Base image[%d] = %q, want %q", i, baseImages[i], expectedImage)
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
	expected := []string{
		"golang:1.18",
		"alpine:3.18",
		// "builder" and "runtime" should be skipped as they reference previous stages
	}

	if len(baseImages) != len(expected) {
		t.Errorf("Expected %d base images, got %d: %v", len(expected), len(baseImages), baseImages)
	}

	for i, expectedImage := range expected {
		if i < len(baseImages) && baseImages[i] != expectedImage {
			t.Errorf("Base image[%d] = %q, want %q", i, baseImages[i], expectedImage)
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
	expected := []string{
		"ubuntu:20.04",
	}

	if len(baseImages) != len(expected) {
		t.Errorf("Expected %d base images, got %d: %v", len(expected), len(baseImages), baseImages)
	}

	if len(baseImages) > 0 && baseImages[0] != expected[0] {
		t.Errorf("Base image[0] = %q, want %q", baseImages[0], expected[0])
	}
}

