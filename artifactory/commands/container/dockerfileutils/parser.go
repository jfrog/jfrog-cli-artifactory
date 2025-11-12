package dockerfileutils

import (
	"bufio"
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/log"
)

// ParseDockerfileBaseImages extracts all base image references from FROM instructions in a Dockerfile
// Handles FROM instructions with flags like --platform and AS clauses
// Ignores FROM clauses that reference previous build stages in multi-stage builds
// Examples:
//   - FROM ubuntu:20.04
//   - FROM ubuntu:20.04 AS builder
//   - FROM --platform=linux/amd64 ubuntu:20.04
//   - FROM --platform=linux/amd64 ubuntu:20.04 AS builder
//   - FROM builder (skipped - references previous stage)
func ParseDockerfileBaseImages(dockerfilePath string) ([]string, error) {
	file, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Warn("Error closing file: " + cerr.Error())
		}
	}()

	var baseImages []string
	var stageNames = make(map[string]bool) // Track stage names from AS clauses
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		upperLine := strings.ToUpper(line)
		if strings.HasPrefix(upperLine, "FROM ") {
			baseImage, stageName := extractBaseImageAndStageFromLine(line)
			if baseImage == "" {
				log.Debug("Could not extract base image from FROM instruction: " + line)
				continue
			}

			// Track stage name if AS clause is present
			if stageName != "" {
				stageNames[stageName] = true
				log.Debug("Found build stage: " + stageName)
			}

			// Skip if this FROM references a previous build stage
			if stageNames[baseImage] {
				log.Debug("Skipping FROM clause referencing previous stage: " + baseImage)
				continue
			}

			// Skip scratch image as it has no layers
			if baseImage == "scratch" {
				log.Debug("Skipping scratch image (no layers to track)")
				continue
			}

			baseImages = append(baseImages, baseImage)
		}
	}

	return baseImages, scanner.Err()
}

// extractBaseImageAndStageFromLine extracts the base image name and stage name from a FROM instruction line
// Returns: (baseImage, stageName)
// Handles flags like --platform and AS clauses
func extractBaseImageAndStageFromLine(line string) (string, string) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", ""
	}

	var imageName string
	var stageName string

	// Find the image name by skipping flags (starting with --) and stopping at AS
	for i := 1; i < len(parts); i++ {
		part := parts[i]

		// Check for AS clause (case-insensitive)
		if strings.ToUpper(part) == "AS" {
			// Next part after AS is the stage name
			if i+1 < len(parts) {
				stageName = parts[i+1]
			}
			break
		}

		// Skip flags (starting with --)
		if strings.HasPrefix(part, "--") {
			continue
		}

		// This should be the image name
		if imageName == "" {
			imageName = part
			// Don't break here - continue to check for AS clause
		}
	}

	return imageName, stageName
}
