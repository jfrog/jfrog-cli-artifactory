package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// goEnvFileForStatus returns the file `go env -w` writes to. It asks go when it is
// installed, and otherwise applies go's own default (GOENV, then the per-user config
// dir) so the file is still read. An empty path means GOENV=off: there is no such file,
// and `go env GOENV` then prints an empty line.
func goEnvFileForStatus(goFound bool) (string, error) {
	switch {
	case os.Getenv("GOENV") == "off":
		return "", nil
	case goFound:
		return goEnvFilePath()
	case os.Getenv("GOENV") != "":
		return os.Getenv("GOENV"), nil
	default:
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", errorutils.CheckError(err)
		}
		return filepath.Join(configDir, "go", "env"), nil
	}
}

// readGoEnvValue returns the value of key in a Go env file, which holds KEY=VALUE lines.
func readGoEnvValue(content []byte, key string) string {
	value := ""
	for _, line := range strings.Split(string(content), "\n") {
		if name, rest, found := strings.Cut(strings.TrimSpace(line), "="); found && strings.TrimSpace(name) == key {
			value = strings.Trim(strings.TrimSpace(rest), `"'`)
		}
	}
	return value
}

func splitGoProxy(goProxy string) []string {
	return strings.FieldsFunc(goProxy, func(r rune) bool { return strings.ContainsRune(goProxySeparators, r) })
}

func inspectGo(serverDetails *config.ServerDetails) (inspection, error) {
	goFound := binaryFound("go")
	result := newInspection(goFound)
	path, err := goEnvFileForStatus(goFound)
	if err != nil {
		return inspection{}, err
	}
	if path != "" {
		result.status.Location = path
		content, _, readErr := readOptionalFile(path)
		if readErr != nil {
			return inspection{}, readErr
		}
		classifyGoProxy(&result, readGoEnvValue(content, "GOPROXY"), serverDetails)
	}
	if os.Getenv("GOPROXY") != "" {
		result.status.OverriddenBy = []ConfigOverride{{Source: "GOPROXY environment variable"}}
	}
	return result, nil
}

// classifyGoProxy decides from the first GOPROXY entry only: go does not try a later entry
// until the first one fails, so a server listed after another proxy (or after "direct")
// is not the one go resolves through.
func classifyGoProxy(result *inspection, goProxy string, serverDetails *config.ServerDetails) {
	entries := splitGoProxy(goProxy)
	if len(entries) == 0 {
		return
	}
	first := entries[0]
	goRepoKey := func(rest string) string { return repoKeyAfter(rest, "api", "go") }
	result.status.State, result.status.Host, result.status.RepoKey = classify(project.Go, first, serverDetails, goRepoKey)
	if result.status.State == StateConfigured {
		applyURLCredentials(result, first, CredentialsAbsent)
	}
}
