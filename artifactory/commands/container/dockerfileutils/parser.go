package dockerfileutils

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ocicontainer"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// dockerfileParser holds state for parsing a Dockerfile
type dockerfileParser struct {
	// Track stage names from AS clauses
	stageNames map[string]bool
	// Track already added images to avoid duplicates
	seenImages  map[string]bool
	defaultOS   string
	defaultArch string
}

// newDockerfileParser creates a new parser with default platform settings
func newDockerfileParser() *dockerfileParser {
	defaultOS := runtime.GOOS
	if defaultOS == "darwin" {
		defaultOS = "linux"
	}
	return &dockerfileParser{
		stageNames:  make(map[string]bool),
		seenImages:  make(map[string]bool),
		defaultOS:   defaultOS,
		defaultArch: runtime.GOARCH,
	}
}

// ParseDockerfileBaseImages extracts all base image references from FROM instructions in a Dockerfile.
// Handles:
//   - FROM instructions with flags like --platform and AS clauses
//   - Multi-line instructions with backslash continuation
//   - Ignores FROM clauses that reference previous build stages
//   - Deduplicates identical base images
//
// Examples:
//   - FROM ubuntu:20.04
//   - FROM ubuntu:20.04 AS builder
//   - FROM --platform=linux/amd64 ubuntu:20.04
//   - FROM --platform=linux/amd64 ubuntu:20.04 AS builder
//   - FROM builder (skipped - references previous stage)
func ParseDockerfileBaseImages(dockerfilePath string) ([]ocicontainer.DockerImage, error) {
	file, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeError := file.Close(); closeError != nil {
			log.Warn("Error closing file: " + closeError.Error())
		}
	}()

	parser := newDockerfileParser()
	lines, err := readDockerfileLines(file)
	if err != nil {
		return nil, err
	}

	return parser.extractBaseImages(lines), nil
}

// readDockerfileLines reads a Dockerfile and returns complete logical lines,
// handling backslash line continuation.
func readDockerfileLines(file *os.File) ([]string, error) {
	var lines []string
	var pendingLine strings.Builder
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		trimmedLine := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines (only if not continuing a previous line)
		if pendingLine.Len() == 0 && (strings.HasPrefix(trimmedLine, "#") || trimmedLine == "") {
			continue
		}

		// Handle line continuation (backslash at end)
		if strings.HasSuffix(trimmedLine, "\\") {
			pendingLine.WriteString(strings.TrimSuffix(trimmedLine, "\\"))
			pendingLine.WriteString(" ")
			continue
		}

		// Complete the line
		if pendingLine.Len() > 0 {
			pendingLine.WriteString(trimmedLine)
			lines = append(lines, strings.TrimSpace(pendingLine.String()))
			pendingLine.Reset()
		} else {
			lines = append(lines, trimmedLine)
		}
	}

	return lines, scanner.Err()
}

// extractBaseImages processes all lines and extracts base images from FROM instructions
func (p *dockerfileParser) extractBaseImages(lines []string) []ocicontainer.DockerImage {
	var baseImages []ocicontainer.DockerImage

	for _, line := range lines {
		if !isFromInstruction(line) {
			continue
		}

		fromInfo := p.parseFromInstruction(line)
		if fromInfo.image == "" {
			log.Debug("Could not extract base image from FROM instruction: " + line)
			continue
		}

		// Track stage name for multi-stage build references
		if fromInfo.stageName != "" {
			p.stageNames[fromInfo.stageName] = true
			log.Debug("Found build stage: " + fromInfo.stageName)
		}

		// Apply skip rules
		if reason := p.shouldSkipImage(fromInfo.image); reason != "" {
			log.Debug(reason)
			continue
		}

		p.seenImages[fromInfo.image] = true
		baseImages = append(baseImages, ocicontainer.DockerImage{
			Image:        fromInfo.image,
			OS:           fromInfo.os,
			Architecture: fromInfo.arch,
		})
	}

	return baseImages
}

// fromInstruction holds parsed data from a FROM instruction
type fromInstruction struct {
	image     string
	stageName string
	os        string
	arch      string
}

// isFromInstruction checks if a line is a FROM instruction
func isFromInstruction(line string) bool {
	return strings.HasPrefix(strings.ToUpper(line), "FROM ")
}

// parseFromInstruction extracts image, stage name, and platform from a FROM line
// Examples:
//   - "FROM ubuntu:20.04" -> {image: "ubuntu:20.04"}
//   - "FROM ubuntu:20.04 AS builder" -> {image: "ubuntu:20.04", stageName: "builder"}
//   - "FROM --platform=linux/amd64 ubuntu:20.04" -> {image: "ubuntu:20.04", os: "linux", arch: "amd64"}
func (p *dockerfileParser) parseFromInstruction(line string) fromInstruction {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return fromInstruction{os: p.defaultOS, arch: p.defaultArch}
	}

	result := fromInstruction{
		os:   p.defaultOS,
		arch: p.defaultArch,
	}

	for i := 1; i < len(parts); i++ {
		part := parts[i]

		// Check for AS clause (case-insensitive)
		if strings.EqualFold(part, "AS") {
			if i+1 < len(parts) {
				result.stageName = parts[i+1]
			}
			break
		}

		// Check for --platform flag
		if strings.HasPrefix(part, "--platform=") {
			result.os, result.arch = parsePlatformFlag(part, p.defaultOS, p.defaultArch)
			continue
		}

		// Skip other flags
		if strings.HasPrefix(part, "--") {
			continue
		}

		// First non-flag token is the image name
		if result.image == "" {
			result.image = part
		}
	}

	return result
}

// parsePlatformFlag extracts OS and architecture from --platform=os/arch flag
func parsePlatformFlag(flag, defaultOS, defaultArch string) (string, string) {
	value := strings.TrimPrefix(flag, "--platform=")
	parts := strings.Split(value, "/")

	if len(parts) == 2 {
		log.Debug("Found platform flag: " + value)
		return parts[0], parts[1]
	}

	log.Debug("Invalid platform format in --platform flag: " + value)
	return defaultOS, defaultArch
}

// shouldSkipImage returns a reason string if the image should be skipped, or empty string if it should be included
func (p *dockerfileParser) shouldSkipImage(image string) string {
	// Skip if this FROM references a previous build stage
	if p.stageNames[image] {
		return "Skipping FROM clause referencing previous stage: " + image
	}

	// Skip scratch image as it has no layers
	if image == "scratch" {
		return "Skipping scratch image (no layers to track)"
	}

	// Skip if already seen (deduplication)
	if p.seenImages[image] {
		return "Skipping duplicate base image: " + image
	}

	return ""
}
