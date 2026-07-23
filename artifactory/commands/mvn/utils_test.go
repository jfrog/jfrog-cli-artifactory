package mvn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMavenExecutable(t *testing.T) {
	wrapperName := "mvnw"
	if coreutils.IsWindows() {
		wrapperName = "mvnw.cmd"
	}

	tests := []struct {
		name          string
		preferWrapper bool
		// setupCwd creates the fixture and returns the directory to chdir into, plus the
		// directory the wrapper was actually placed in ("" if no wrapper was created).
		setupCwd    func(t *testing.T) (cwd string, wrapperRoot string)
		expectedExe string
		expectErr   bool
	}{
		{
			name:          "jf mvn without wrapper present - falls back to PATH mvn",
			preferWrapper: false,
			setupCwd: func(t *testing.T) (string, string) {
				return t.TempDir(), ""
			},
			expectedExe: "mvn",
		},
		{
			name:          "jf mvn with wrapper present - still uses PATH mvn (opt-in only via mvnw)",
			preferWrapper: false,
			setupCwd: func(t *testing.T) (string, string) {
				root := t.TempDir()
				createWrapperFixture(t, root, wrapperName)
				return root, root
			},
			expectedExe: "mvn",
		},
		{
			name:          "jf mvnw with wrapper in cwd - uses wrapper",
			preferWrapper: true,
			setupCwd: func(t *testing.T) (string, string) {
				root := t.TempDir()
				createWrapperFixture(t, root, wrapperName)
				return root, root
			},
		},
		{
			name:          "jf mvnw with wrapper only in parent dir - upward search finds it",
			preferWrapper: true,
			setupCwd: func(t *testing.T) (string, string) {
				root := t.TempDir()
				createWrapperFixture(t, root, wrapperName)
				subDir := filepath.Join(root, "module-a")
				require.NoError(t, os.MkdirAll(subDir, 0755))
				return subDir, root
			},
		},
		{
			name:          "jf mvnw without wrapper anywhere - errors, no fallback",
			preferWrapper: true,
			setupCwd: func(t *testing.T) (string, string) {
				return t.TempDir(), ""
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origWd, err := os.Getwd()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.Chdir(origWd))
			}()

			cwd, wrapperRoot := tt.setupCwd(t)
			require.NoError(t, os.Chdir(cwd))

			exe, err := resolveMavenExecutable(tt.preferWrapper)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.expectedExe != "" {
				assert.Equal(t, tt.expectedExe, exe)
				return
			}
			resolvedRoot, err := filepath.EvalSymlinks(wrapperRoot)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(resolvedRoot, wrapperName), exe)
		})
	}
}

// createWrapperFixture creates a minimal Maven Wrapper marker (.mvn dir + wrapper script) at root.
func createWrapperFixture(t *testing.T, root, wrapperName string) {
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mvn", "wrapper"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, wrapperName), []byte("#!/bin/sh\n"), 0755))
}
