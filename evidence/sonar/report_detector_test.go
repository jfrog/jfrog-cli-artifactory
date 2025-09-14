package sonar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectReportTaskPath(t *testing.T) {
	tests := []struct {
		name         string
		setupFiles   []string
		expectedPath string
		cleanupFiles []string
	}{
		{
			name:         "Maven report task found",
			setupFiles:   []string{"target/sonar/report-task.txt"},
			expectedPath: "target/sonar/report-task.txt",
		},
		{
			name:         "Gradle report task found",
			setupFiles:   []string{"build/sonar/report-task.txt"},
			expectedPath: "build/sonar/report-task.txt",
		},
		{
			name:         "CLI report task found",
			setupFiles:   []string{".scannerwork/report-task.txt"},
			expectedPath: ".scannerwork/report-task.txt",
		},
		{
			name:         "MSBuild report task found",
			setupFiles:   []string{".sonarqube/out/.sonar/report-task.txt"},
			expectedPath: ".sonarqube/out/.sonar/report-task.txt",
		},
		{
			name:         "Multiple files exist - should return first found (maven)",
			setupFiles:   []string{"target/sonar/report-task.txt", "build/sonar/report-task.txt", ".scannerwork/report-task.txt"},
			expectedPath: "target/sonar/report-task.txt",
		},
		{
			name:         "No files exist - should return empty string",
			setupFiles:   []string{},
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test files
			for _, file := range tt.setupFiles {
				dir := filepath.Dir(file)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory %s: %v", dir, err)
				}
				if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
					t.Fatalf("Failed to create test file %s: %v", file, err)
				}
			}

			// Run the function
			result := detectReportTaskPath("")

			// Verify result
			if result != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, result)
			}

			// Cleanup test files
			for _, file := range tt.setupFiles {
				if err := os.RemoveAll(filepath.Dir(file)); err != nil {
					t.Logf("Warning: failed to cleanup %s: %v", filepath.Dir(file), err)
				}
			}
		})
	}
}

func TestDetectReportTaskPathPriority(t *testing.T) {
	// Test that the priority order is correct (maven > gradle > cli > msbuild)
	setupFiles := []string{
		"build/sonar/report-task.txt",           // gradle
		".scannerwork/report-task.txt",          // cli
		".sonarqube/out/.sonar/report-task.txt", // msbuild
	}

	// Create all files
	for _, file := range setupFiles {
		dir := filepath.Dir(file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
	}

	// Now add maven file (should be found first)
	mavenFile := "target/sonar/report-task.txt"
	dir := filepath.Dir(mavenFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(mavenFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", mavenFile, err)
	}

	// Run the function
	result := detectReportTaskPath("")

	// Should return maven file (first in priority order)
	expectedPath := "target/sonar/report-task.txt"
	if result != expectedPath {
		t.Errorf("Expected maven path %s, got %s", expectedPath, result)
	}

	// Cleanup
	for _, file := range append(setupFiles, mavenFile) {
		if err := os.RemoveAll(filepath.Dir(file)); err != nil {
			t.Logf("Warning: failed to cleanup %s: %v", filepath.Dir(file), err)
		}
	}
}

func TestDetectReportTaskPathWithRealContent(t *testing.T) {
	// Test with actual report-task.txt content
	testContent := `ceTaskUrl=https://sonarcloud.io/api/ce/task?id=AX123456789
ceTaskId=AX123456789
analysisId=AX987654321
projectKey=test-project
serverUrl=https://sonarcloud.io`

	// Create test file
	testFile := "target/sonar/report-task.txt"
	dir := filepath.Dir(testFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", testFile, err)
	}

	// Run the function
	result := detectReportTaskPath("")

	// Should return the maven file
	expectedPath := "target/sonar/report-task.txt"
	if result != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, result)
	}

	// Verify the file is readable
	if _, err := os.Stat(result); os.IsNotExist(err) {
		t.Errorf("Detected file %s does not exist", result)
	}

	// Cleanup
	if err := os.RemoveAll(dir); err != nil {
		t.Logf("Warning: failed to cleanup %s: %v", dir, err)
	}
}

func TestDetectReportTaskPathWithConfiguredPath(t *testing.T) {
	// Test with configured path
	configuredPath := "custom/path/report-task.txt"

	// Create the configured file
	dir := filepath.Dir(configuredPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(configuredPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", configuredPath, err)
	}

	// Also create a standard candidate file to ensure configured path takes priority
	standardPath := "target/sonar/report-task.txt"
	standardDir := filepath.Dir(standardPath)
	if err := os.MkdirAll(standardDir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", standardDir, err)
	}
	if err := os.WriteFile(standardPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", standardPath, err)
	}

	// Run the function with configured path
	result := detectReportTaskPath(configuredPath)

	// Should return the configured path (priority over standard candidates)
	if result != configuredPath {
		t.Errorf("Expected configured path %s, got %s", configuredPath, result)
	}

	// Cleanup
	if err := os.RemoveAll(dir); err != nil {
		t.Logf("Warning: failed to cleanup %s: %v", dir, err)
	}
	if err := os.RemoveAll(standardDir); err != nil {
		t.Logf("Warning: failed to cleanup %s: %v", standardDir, err)
	}
}

func TestDetectReportTaskPathWithInvalidConfiguredPath(t *testing.T) {
	// Test with invalid configured path
	invalidConfiguredPath := "non/existent/path/report-task.txt"

	// Create a standard candidate file
	standardPath := "target/sonar/report-task.txt"
	standardDir := filepath.Dir(standardPath)
	if err := os.MkdirAll(standardDir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", standardDir, err)
	}
	if err := os.WriteFile(standardPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", standardPath, err)
	}

	// Run the function with invalid configured path
	result := detectReportTaskPath(invalidConfiguredPath)

	// Should fall back to standard candidate
	if result != standardPath {
		t.Errorf("Expected fallback to standard path %s, got %s", standardPath, result)
	}

	// Cleanup
	if err := os.RemoveAll(standardDir); err != nil {
		t.Logf("Warning: failed to cleanup %s: %v", standardDir, err)
	}
}

func TestReportTaskCandidates(t *testing.T) {
	// Test that the candidates array is correctly defined
	expectedCandidates := []string{
		"target/sonar/report-task.txt",          // maven
		"build/sonar/report-task.txt",           // gradle
		".scannerwork/report-task.txt",          // cli
		".sonarqube/out/.sonar/report-task.txt", // msbuild
	}

	if len(reportTaskCandidates) != len(expectedCandidates) {
		t.Errorf("Expected %d candidates, got %d", len(expectedCandidates), len(reportTaskCandidates))
	}

	for i, expected := range expectedCandidates {
		if reportTaskCandidates[i] != expected {
			t.Errorf("Expected candidate[%d] to be '%s', got '%s'", i, expected, reportTaskCandidates[i])
		}
	}
}

func TestGetReportTaskPath(t *testing.T) {
	tests := []struct {
		name         string
		setupFiles   []string
		expectedPath string
		cleanupFiles []string
	}{
		{
			name:         "Auto-detected file found",
			setupFiles:   []string{"target/sonar/report-task.txt"},
			expectedPath: "target/sonar/report-task.txt",
		},
		{
			name:         "No files exist - should return empty string",
			setupFiles:   []string{},
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test files
			for _, file := range tt.setupFiles {
				dir := filepath.Dir(file)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory %s: %v", dir, err)
				}
				if err := os.WriteFile(file, []byte("test content"), 0644); err != nil {
					t.Fatalf("Failed to create test file %s: %v", file, err)
				}
			}

			// Run the function
			result := GetReportTaskPath()

			// Verify result
			if result != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, result)
			}

			// Cleanup test files
			for _, file := range tt.setupFiles {
				if err := os.RemoveAll(filepath.Dir(file)); err != nil {
					t.Logf("Warning: failed to cleanup %s: %v", filepath.Dir(file), err)
				}
			}
		})
	}
}
