package sonarqube

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectBuildToolAndReportFilePath(t *testing.T) {
	testCases := []struct {
		name         string
		prepare      func() string
		expectedPath string
		cleanup      func()
	}{
		{
			name: "maven report present",
			prepare: func() string {
				path := filepath.Join("target/sonar/report-task.txt")
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte("dummy"), 0644)
				return path
			},
			cleanup: func() {
				path := filepath.Join("target")
				os.RemoveAll(path)
			},
		},
		{
			name: "gradle report present",
			prepare: func() string {
				path := filepath.Join("build/sonar/report-task.txt")
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte("dummy"), 0644)
				return path
			},
			cleanup: func() {
				path := filepath.Join("build")
				os.RemoveAll(path)
			},
		},
		{
			name: "cli report present",
			prepare: func() string {
				path := filepath.Join(".scannerwork/report-task.txt")
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte("dummy"), 0644)
				return path
			},
			cleanup: func() {
				path := filepath.Join(".scannerwork")
				os.RemoveAll(path)
			},
		},
		{
			name: "msbuild report present",
			prepare: func() string {
				path := filepath.Join(".sonarqube/out/.sonar/report-task.txt")
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte("dummy"), 0644)
				return path
			},
			cleanup: func() {
				path := filepath.Join(".sonarqube")
				os.RemoveAll(path)
			}},
		{
			name: "no report present, fallback to cli",
			prepare: func() string {
				return filepath.Join(".scannerwork/report-task.txt")
			},
			cleanup: func() {
				path := filepath.Join(".scannerwork")
				os.RemoveAll(path)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedPath := tc.prepare()
			path := DetectBuildToolAndReportFilePath()
			assert.Equal(t, expectedPath, path)
			tc.cleanup()
		})
	}
}
