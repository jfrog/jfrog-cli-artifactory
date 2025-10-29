package python

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"gopkg.in/ini.v1"
)

// ConfigurePypirc creates or updates the .pypirc file for Twine
// The function creates a .pypirc file in the user's home directory with the following structure:
//
// [distutils]
// index-servers =
//
//	pypi
//
// [pypi]
// repository = https://<your-artifactory-url>/artifactory/api/pypi/<repo-name>/
// username = <user>
// password = <token-or-password>
//
// Using the name "pypi" as the repository section makes it the default for Twine,
// allowing users to run `twine upload` without specifying a repository.
func ConfigurePypirc(repoURL, repoName, username, password string) error {
	pypircPath, err := getPypircPath()
	if err != nil {
		return err
	}

	pypirc, err := loadOrCreatePypirc(pypircPath)
	if err != nil {
		return err
	}

	// Configure the .pypirc file content
	configurePypiDistutils(pypirc)
	configurePypiRepository(pypirc, repoURL, username, password)

	// Save the file with appropriate permissions
	if err := pypirc.SaveTo(pypircPath); err != nil {
		return fmt.Errorf("failed to save .pypirc file: %w", err)
	}

	return os.Chmod(pypircPath, 0600)
}

// getPypircPath returns the path to the .pypirc file
// Twine exclusively uses this file for configuration,
// The .pypirc file is located in the user's home directory by convention.
func getPypircPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("couldn't find user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".pypirc"), nil
}

// loadOrCreatePypirc loads an existing .pypirc file or creates a new one
func loadOrCreatePypirc(pypircPath string) (*ini.File, error) {
	exists, err := fileutils.IsFileExists(pypircPath, false)
	if err != nil {
		return nil, err
	}

	var pypirc *ini.File
	if exists {
		// Load ini file with necessary options for cross-platform compatibility
		pypirc, err = ini.LoadSources(ini.LoadOptions{
			Loose:                      true, // Required for handling non-standard formatting
			Insensitive:                true, // Required for case-insensitive keys
			IgnoreInlineComment:        true, // Required for values containing special chars
			AllowPythonMultilineValues: true, // Required for Python-style multi-line values
		}, pypircPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load .pypirc file: %w", err)
		}
	} else {
		pypirc = ini.Empty()
		// Create parent directory if it doesn't exist (needed on Windows)
		if err = os.MkdirAll(filepath.Dir(pypircPath), 0700); err != nil {
			return nil, fmt.Errorf("failed to create directory for .pypirc file: %w", err)
		}
	}

	return pypirc, nil
}

// configurePypiDistutils configures the distutils section of .pypirc
func configurePypiDistutils(pypirc *ini.File) {
	distutils := pypirc.Section("distutils")
	indexServers := distutils.Key("index-servers")

	// Get current list of servers
	servers := []string{}
	if indexServers.String() != "" {
		for _, server := range strings.Split(indexServers.String(), "\n") {
			server = strings.TrimSpace(server)
			if server != "" && server != "pypi" {
				servers = append(servers, server)
			}
		}
	}

	// Use "pypi" as the server name to make it the default repository
	const defaultSectionName = "pypi"
	servers = append([]string{defaultSectionName}, servers...)
	indexServers.SetValue(strings.Join(servers, " "))
}

// configurePypiRepository configures the pypi repository section in .pypirc
func configurePypiRepository(pypirc *ini.File, repoURL, username, password string) {
	// Configure the pypi section which will be the default for Twine
	const defaultSectionName = "pypi"
	pypiSection := pypirc.Section(defaultSectionName)
	pypiSection.Key("repository").SetValue(repoURL)
	pypiSection.Key("username").SetValue(username)
	pypiSection.Key("password").SetValue(password)
}
