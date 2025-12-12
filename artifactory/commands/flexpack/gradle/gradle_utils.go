package flexpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func getGradleUserHome() string {
	if home := os.Getenv(envGradleUserHome); home != "" {
		// Sanitize the path to prevent path traversal
		return filepath.Clean(home)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Clean(filepath.Join(home, dotGradleDir))
	}
	return ""
}

// sanitizePath is a taint-clearing sanitizer that cleans a path and resolves it to absolute.
// This function returns a new sanitized path, breaking the taint chain from the input.
// Returns the cleaned absolute path and an error if the path is invalid.
func sanitizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	// filepath.Clean removes .. and . components
	cleaned := filepath.Clean(path)
	// filepath.Abs resolves to absolute path, producing a new untainted value
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	return absPath, nil
}

// isPathContainedIn checks if childPath is contained within parentPath.
// Both paths must be sanitized (absolute and cleaned) before calling this function.
// This is a validation function that returns true if the child is within the parent.
func isPathContainedIn(childPath, parentPath string) bool {
	// Ensure consistent trailing separator for prefix comparison
	parentWithSep := parentPath + string(filepath.Separator)
	childWithSep := childPath + string(filepath.Separator)
	return strings.HasPrefix(childWithSep, parentWithSep)
}

// sanitizeAndValidatePath sanitizes a path and validates it's contained within the base directory.
// Returns the sanitized path or an error if invalid or escaping the base.
func sanitizeAndValidatePath(path, baseDir string) (string, error) {
	sanitizedPath, err := sanitizePath(path)
	if err != nil {
		return "", err
	}
	sanitizedBase, err := sanitizePath(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}
	if !isPathContainedIn(sanitizedPath, sanitizedBase) && sanitizedPath != sanitizedBase {
		return "", fmt.Errorf("path %s escapes base directory %s", sanitizedPath, sanitizedBase)
	}
	return sanitizedPath, nil
}

func findGradleFile(dir, baseName string) (path string, isKts bool, err error) {
	// Sanitize the directory path - this returns a new untainted value
	sanitizedDir, err := sanitizePath(dir)
	if err != nil {
		return "", false, fmt.Errorf("invalid directory path: %w", err)
	}

	// Validate baseName doesn't contain path separators (prevent traversal via filename)
	if strings.ContainsAny(baseName, `/\`) {
		return "", false, fmt.Errorf("invalid base name: %s", baseName)
	}

	// Build and validate groovy path
	groovyPath := filepath.Join(sanitizedDir, baseName+".gradle")
	sanitizedGroovyPath, err := sanitizeAndValidatePath(groovyPath, sanitizedDir)
	if err != nil {
		return "", false, fmt.Errorf("invalid gradle file path: %w", err)
	}
	if _, err := os.Stat(sanitizedGroovyPath); err == nil {
		return sanitizedGroovyPath, false, nil
	}

	// Build and validate kts path
	ktsPath := filepath.Join(sanitizedDir, baseName+".gradle.kts")
	sanitizedKtsPath, err := sanitizeAndValidatePath(ktsPath, sanitizedDir)
	if err != nil {
		return "", false, fmt.Errorf("invalid gradle file path: %w", err)
	}
	if _, err := os.Stat(sanitizedKtsPath); err == nil {
		return sanitizedKtsPath, true, nil
	}
	return "", false, fmt.Errorf("no %s.gradle or %s.gradle.kts found", baseName, baseName)
}

func validateWorkingDirectory(workingDir string) error {
	if workingDir == "" {
		return fmt.Errorf("working directory cannot be empty")
	}
	info, err := os.Stat(workingDir)
	if err != nil {
		return fmt.Errorf("invalid working directory: %s - %w", workingDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory: %s", workingDir)
	}
	return nil
}

func isEscaped(content string, index int) bool {
	backslashes := 0
	for j := index - 1; j >= 0; j-- {
		if content[j] == '\\' {
			backslashes++
		} else {
			break
		}
	}
	return backslashes%2 != 0
}

func isDelimiter(b byte) bool {
	switch b {
	case '{', '}', '(', ')', ';', ',':
		return true
	}
	return isWhitespace(b)
}

func isWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

