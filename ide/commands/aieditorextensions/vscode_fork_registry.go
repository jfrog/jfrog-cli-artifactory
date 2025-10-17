package aieditorextensions

import "runtime"

// VSCodeForkConfig contains configuration for a VSCode-based IDE
type VSCodeForkConfig struct {
	Name         string              // Internal name (e.g., "vscode", "cursor")
	DisplayName  string              // User-facing name (e.g., "Visual Studio Code", "Cursor")
	InstallPaths map[string][]string // OS -> possible installation paths
	ProductJson  string              // Relative path to product.json (usually just "product.json")
	SettingsDir  string              // Settings directory name (e.g., "Code", "Cursor")
}

// GetDefaultInstallPath returns the most common install path for the current OS
func (c *VSCodeForkConfig) GetDefaultInstallPath() string {
	paths := c.InstallPaths[runtime.GOOS]
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// GetAllInstallPaths returns all possible install paths for the current OS
func (c *VSCodeForkConfig) GetAllInstallPaths() []string {
	return c.InstallPaths[runtime.GOOS]
}

// VSCodeForks is the registry of all supported VSCode-based IDEs
var VSCodeForks = map[string]*VSCodeForkConfig{
	"vscode": {
		Name:        "vscode",
		DisplayName: "Visual Studio Code",
		InstallPaths: map[string][]string{
			"darwin": {
				"/Applications/Visual Studio Code.app/Contents/Resources/app",
				"~/Applications/Visual Studio Code.app/Contents/Resources/app",
			},
			"windows": {
				`C:\Program Files\Microsoft VS Code\resources\app`,
				`%LOCALAPPDATA%\Programs\Microsoft VS Code\resources\app`,
			},
			"linux": {
				"/usr/share/code/resources/app",
				"/opt/visual-studio-code/resources/app",
				"~/.local/share/code/resources/app",
			},
		},
		ProductJson: "product.json",
		SettingsDir: "Code",
	},
	"cursor": {
		Name:        "cursor",
		DisplayName: "Cursor",
		InstallPaths: map[string][]string{
			"darwin": {
				"/Applications/Cursor.app/Contents/Resources/app",
				"~/Applications/Cursor.app/Contents/Resources/app",
			},
			"windows": {
				`%LOCALAPPDATA%\Programs\Cursor\resources\app`,
				`C:\Program Files\Cursor\resources\app`,
			},
			"linux": {
				"/usr/share/cursor/resources/app",
				"~/.local/share/cursor/resources/app",
			},
		},
		ProductJson: "product.json",
		SettingsDir: "Cursor",
	},
	"windsurf": {
		Name:        "windsurf",
		DisplayName: "Windsurf",
		InstallPaths: map[string][]string{
			"darwin": {
				"/Applications/Windsurf.app/Contents/Resources/app",
				"~/Applications/Windsurf.app/Contents/Resources/app",
			},
			"windows": {
				`%LOCALAPPDATA%\Programs\Windsurf\resources\app`,
				`C:\Program Files\Windsurf\resources\app`,
			},
			"linux": {
				"/usr/share/windsurf/resources/app",
				"~/.local/share/windsurf/resources/app",
			},
		},
		ProductJson: "product.json",
		SettingsDir: "Windsurf",
	},
}

// GetVSCodeFork retrieves a VSCode fork configuration by name
func GetVSCodeFork(name string) (*VSCodeForkConfig, bool) {
	config, exists := VSCodeForks[name]
	return config, exists
}

// GetSupportedForks returns a list of all supported VSCode fork names
func GetSupportedForks() []string {
	forks := make([]string, 0, len(VSCodeForks))
	for name := range VSCodeForks {
		forks = append(forks, name)
	}
	return forks
}
