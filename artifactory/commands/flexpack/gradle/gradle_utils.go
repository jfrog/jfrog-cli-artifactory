package flexpack

import (
	"fmt"
	"os"
	"path/filepath"
)

func getGradleUserHome() string {
	if home := os.Getenv(envGradleUserHome); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, dotGradleDir)
	}
	return ""
}

func findGradleFile(dir, baseName string) (path string, isKts bool, err error) {
	groovyPath := filepath.Join(dir, baseName+".gradle")
	if _, err := os.Stat(groovyPath); err == nil {
		return groovyPath, false, nil
	}

	ktsPath := filepath.Join(dir, baseName+".gradle.kts")
	if _, err := os.Stat(ktsPath); err == nil {
		return ktsPath, true, nil
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

