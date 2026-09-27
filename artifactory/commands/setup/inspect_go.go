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
// dir) so the file is still read. An empty path means GOENV=off: there is no such file.
func goEnvFileForStatus(goFound bool) (string, error) {
	var path string
	switch {
	case goFound:
		resolved, err := goEnvFilePath()
		if err != nil {
			return "", err
		}
		path = resolved
	case os.Getenv("GOENV") != "":
		path = os.Getenv("GOENV")
	default:
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", errorutils.CheckError(err)
		}
		path = filepath.Join(configDir, "go", "env")
	}
	if path == "off" {
		return "", nil
	}
	return path, nil
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

// classifyGoProxy uses the first GOPROXY entry that points at the server. Without one,
// the first entry that is not a public default decides between other-host and
// not-configured.
func classifyGoProxy(result *inspection, goProxy string, serverDetails *config.ServerDetails) {
	entries := splitGoProxy(goProxy)
	goRepoKey := func(rest string) string { return repoKeyAfter(rest, "api", "go") }
	for _, entry := range entries {
		if matched, _ := matchServerURL(entry, serverDetails); matched {
			result.status.State, result.status.Host, result.status.RepoKey = classify(project.Go, entry, serverDetails, goRepoKey)
			applyURLCredentials(result, entry, CredentialsAbsent)
			return
		}
	}
	for _, entry := range entries {
		if state, host, _ := classify(project.Go, entry, serverDetails, nil); state == StateOtherHost {
			result.status.State, result.status.Host = state, host
			return
		}
	}
}
