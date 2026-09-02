package npm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jfrog/jfrog-client-go/xray/services"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/zerotouchremediation"
)

type countingZTRClient struct {
	calls int
}

type stubInstaller struct {
	runErr error
}

func (s stubInstaller) PrepareInstallPrerequisites(string) error { return nil }
func (s stubInstaller) Run() error                               { return s.runErr }
func (s stubInstaller) RestoreNpmrc() error                      { return nil }

func (c *countingZTRClient) GetVersion() (string, error) {
	return zerotouchremediation.ZeroTouchRemediationMinXrayVersion, nil
}

func (c *countingZTRClient) ZeroTouchRemediation(_ services.ComponentResolutionRequest) (*services.ComponentResolutionResponse, bool, error) {
	c.calls++
	return &services.ComponentResolutionResponse{
		Changes: []services.Change{{Package: "lodash"}},
	}, false, nil
}

func TestApplyZeroTouchRemediation_DisabledByDefault(t *testing.T) {
	t.Setenv(zerotouchremediation.ZtrComponentsEnabledEnvVar, "")
	nc := &NpmCommand{cmdName: "install", workingDirectory: t.TempDir()}
	nc.SetRepo("npm-virtual").SetServerDetails(nil)
	assert.NoError(t, nc.applyZeroTouchRemediation())
	assert.False(t, nc.remediatedLockfile)
	assert.Nil(t, nc.restoreResolution)
}

func TestInstallWithLockfileRestore_RestoresOnlyOnFailure(t *testing.T) {
	installErr := errors.New("install failed")
	restoreErr := errors.New("restore failed")
	restoreCalls := 0
	nc := &NpmCommand{
		installHandler: &NpmInstallStrategy{strategy: stubInstaller{runErr: installErr}},
		restoreResolution: func() error {
			restoreCalls++
			return restoreErr
		},
	}

	err := nc.installWithLockfileRestore()

	assert.ErrorIs(t, err, installErr)
	assert.ErrorIs(t, err, restoreErr)
	assert.Equal(t, 1, restoreCalls)

	nc.installHandler = &NpmInstallStrategy{strategy: stubInstaller{}}
	require.NoError(t, nc.installWithLockfileRestore())
	assert.Equal(t, 1, restoreCalls)
}

func TestResolverRepoForResolution_PrefersCliRegistryOverNpmConfig(t *testing.T) {
	nc := &NpmCommand{
		cmdName:        "install",
		executablePath: "/bin/false",
	}
	got, err := nc.resolverRepoForResolution("https://acme.jfrog.io/artifactory/api/npm/libs-npm/")
	require.NoError(t, err)
	assert.Equal(t, "libs-npm", got)
}

func TestResolverRepoForResolution_CliRegistryOverridesRepo(t *testing.T) {
	nc := &NpmCommand{cmdName: "install"}
	nc.SetRepo("from-config")
	got, err := nc.resolverRepoForResolution("https://acme.jfrog.io/artifactory/api/npm/libs-npm/")
	require.NoError(t, err)
	assert.Equal(t, "libs-npm", got)
}

func TestResolverRepoFromNpmConfig(t *testing.T) {
	const (
		npmjs       = "https://registry.npmjs.org/"
		libsNpm     = "https://acme.jfrog.io/artifactory/api/npm/libs-npm/"
		internalNpm = "https://acme.jfrog.io/artifactory/api/npm/npm-internal/"
	)
	tests := []struct {
		name       string
		cliReg     string
		config     string
		want       string
		wantErrSub string
	}{
		{
			name:   "scoped Artifactory repo when default is public npm",
			config: "registry = " + npmjs + "\n@company:registry = " + libsNpm + "\n",
			want:   "libs-npm",
		},
		{
			name:   "quoted scoped registry from npm c ls",
			config: "; userconfig\nregistry = \"" + npmjs + "\"\n@company:registry = \"" + libsNpm + "\"\n",
			want:   "libs-npm",
		},
		{
			name:   "single Artifactory default registry",
			config: "registry = " + libsNpm + "\n",
			want:   "libs-npm",
		},
		{
			name:   "scoped maps to the same Artifactory repo as default",
			config: "registry = " + libsNpm + "\n@company:registry = " + libsNpm + "\n",
			want:   "libs-npm",
		},
		{
			name:       "distinct scoped and default Artifactory repos",
			config:     "registry = " + libsNpm + "\n@company:registry = " + internalNpm + "\n",
			wantErrSub: "multiple Artifactory npm registries",
		},
		{
			name:   "CLI --registry overrides default but scoped Artifactory still counts",
			cliReg: npmjs,
			config: "registry = " + libsNpm + "\n@company:registry = " + internalNpm + "\n",
			want:   "npm-internal",
		},
		{
			name:   "CLI --registry is the only Artifactory URL",
			cliReg: libsNpm,
			config: "registry = " + npmjs + "\n",
			want:   "libs-npm",
		},
		{
			name:   "no Artifactory registry",
			config: "registry = " + npmjs + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolverRepoFromNpmConfig(tt.cliReg, []byte(tt.config))
			if tt.wantErrSub != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSub)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDependencyCollectionArgsCommandSelection(t *testing.T) {
	nc := &NpmCommand{cmdName: "install", remediatedLockfile: true}
	assert.Equal(t, []string{"ci"}, nc.dependencyCollectionArgs())

	nc.remediatedLockfile = false
	assert.Equal(t, []string{"install"}, nc.dependencyCollectionArgs())

	nc.cmdName = "ci"
	nc.remediatedLockfile = true
	assert.Equal(t, []string{"ci"}, nc.dependencyCollectionArgs())
}

func TestDependencyCollectionArgsAfterRemediation(t *testing.T) {
	nc := &NpmCommand{
		cmdName:            "install",
		remediatedLockfile: true,
	}
	nc.SetNpmArgs([]string{
		"--save", "-D", "--save-prefix", "~",
		"--package-lock-only", "--package-lock", "false", "--no-package-lock=false",
		"--include", "dev", "--omit=optional",
		"--workspaces", "-w", "app",
		"--registry=https://acme.jfrog.io/artifactory/api/npm/libs-npm/",
	})

	assert.Equal(t, []string{
		"ci",
		"--include", "dev",
		"--omit=optional",
		"--workspaces", "-w", "app",
		"--registry=https://acme.jfrog.io/artifactory/api/npm/libs-npm/",
	}, nc.dependencyCollectionArgs())
}

func TestRunIfEnabled_SkipsSymlinkedLockfile(t *testing.T) {
	t.Setenv(zerotouchremediation.ZtrComponentsEnabledEnvVar, "true")
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.lock")
	require.NoError(t, os.WriteFile(outside, []byte("secret-from-outside"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"app"}`), 0644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "package-lock.json")))

	client := &countingZTRClient{}
	_, remediated, err := zerotouchremediation.RunIfEnabled(context.Background(), client, "npm-virtual", NewBuildTool(), "install", dir, nil)
	require.NoError(t, err)
	assert.False(t, remediated)
	assert.Equal(t, 0, client.calls)
}
