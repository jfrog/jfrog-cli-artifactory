package helm

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/stretchr/testify/require"
)

type mockPackageServicesManager struct {
	artifactory.EmptyArtifactoryServicesManager
}

func TestHandlePackageCommand(t *testing.T) {
	t.Setenv("JFROG_CLI_HOME_DIR", t.TempDir())

	tests := []struct {
		name string
		args func(chartPath, destination string) []string
	}{
		{
			name: "chart path only",
			args: func(chartPath, _ string) []string {
				return []string{chartPath}
			},
		},
		{
			name: "dependency update flag",
			args: func(chartPath, _ string) []string {
				return []string{"-u", chartPath}
			},
		},
		{
			name: "sign key and keyring flags",
			args: func(chartPath, _ string) []string {
				return []string{"--sign", chartPath, "--key", "mykey", "--keyring", "~/.gnupg/secring.gpg"}
			},
		},
		{
			name: "chart path with destination flag",
			args: func(chartPath, destination string) []string {
				return []string{chartPath, "--destination", destination}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath := createHelmChartWithDependencies(t)
			destination := t.TempDir()
			buildInfo := entities.New()
			err := handlePackageCommand(
				buildInfo,
				tt.args(chartPath, destination),
				&mockPackageServicesManager{},
				"helm-package-test",
				"1",
				"",
			)
			require.NoError(t, err)
			requireBuildInfoDetails(t, buildInfo)
		})
	}
}

func createHelmChartWithDependencies(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	parentChart := filepath.Join(root, "mychart")
	subChartA := filepath.Join(root, "subchart-a")
	subChartB := filepath.Join(root, "subchart-b")

	createChartFiles(t, subChartA, "subchart-a", "0.1.0")
	createChartFiles(t, subChartB, "subchart-b", "0.2.0")

	createChartFiles(t, parentChart, "mychart", "1.0.0")
	parentChartYaml := `apiVersion: v2
name: mychart
version: 1.0.0
dependencies:
  - name: subchart-a
    version: 0.1.0
    repository: file://../subchart-a
  - name: subchart-b
    version: 0.2.0
    repository: file://../subchart-b
`
	writeFile(t, filepath.Join(parentChart, "Chart.yaml"), parentChartYaml)
	parentChartLock := `dependencies:
  - name: subchart-a
    version: 0.1.0
    repository: file://../subchart-a
  - name: subchart-b
    version: 0.2.0
    repository: file://../subchart-b
digest: sha256:8eb5f18ec9f7242805d50cef3f31ce0b2c60d36d4e65b6f56af7b7344f5ba99c
generated: "2026-07-07T00:00:00.000000000Z"
`
	writeFile(t, filepath.Join(parentChart, "Chart.lock"), parentChartLock)

	chartsDir := filepath.Join(parentChart, "charts")
	require.NoError(t, os.MkdirAll(chartsDir, 0o755))
	require.NoError(t, packageChartDir(t, subChartA, filepath.Join(chartsDir, "subchart-a-0.1.0.tgz")))
	require.NoError(t, packageChartDir(t, subChartB, filepath.Join(chartsDir, "subchart-b-0.2.0.tgz")))

	return parentChart
}

func createChartFiles(t *testing.T, chartDir, name, version string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755))
	chartYaml := "apiVersion: v2\nname: " + name + "\nversion: " + version + "\n"
	writeFile(t, filepath.Join(chartDir, "Chart.yaml"), chartYaml)
	writeFile(t, filepath.Join(chartDir, "values.yaml"), "replicaCount: 1\n")
	writeFile(t, filepath.Join(chartDir, "templates", "configmap.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: "+name+"\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func packageChartDir(t *testing.T, sourceDir, targetTgz string) error {
	t.Helper()

	file, err := os.Create(targetTgz)
	if err != nil {
		return err
	}
	defer func() {
		require.NoError(t, file.Close())
	}()

	gzWriter := gzip.NewWriter(file)
	defer func() {
		require.NoError(t, gzWriter.Close())
	}()

	tarWriter := tar.NewWriter(gzWriter)
	defer func() {
		require.NoError(t, tarWriter.Close())
	}()

	baseName := filepath.Base(sourceDir)
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		archivePath := filepath.ToSlash(filepath.Join(baseName, relPath))
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = archivePath
		if info.IsDir() {
			if !strings.HasSuffix(header.Name, "/") {
				header.Name += "/"
			}
			return tarWriter.WriteHeader(header)
		}

		if err = tarWriter.WriteHeader(header); err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tarWriter.Write(content)
		return err
	})
}

func requireBuildInfoDetails(t *testing.T, buildInfo *entities.BuildInfo) {
	t.Helper()

	require.Len(t, buildInfo.Modules, 1)
	module := buildInfo.Modules[0]
	require.Equal(t, "mychart:1.0.0", module.Id)
	require.NotEmpty(t, module.Dependencies)

	depIds := map[string]bool{}
	for _, dep := range module.Dependencies {
		depIds[dep.Id] = true
		require.NotEmpty(t, dep.Sha256)
	}
	require.True(t, depIds["subchart-a:0.1.0"])
	require.True(t, depIds["subchart-b:0.2.0"])
}