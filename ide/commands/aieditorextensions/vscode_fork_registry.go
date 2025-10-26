package aieditorextensions

import "runtime"

type VSCodeForkConfig struct {
	Name         string
	DisplayName  string
	InstallPaths map[string][]string
	ProductJson  string
	SettingsDir  string
}

func (c *VSCodeForkConfig) GetDefaultInstallPath() string {
	paths := c.InstallPaths[runtime.GOOS]
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func (c *VSCodeForkConfig) GetAllInstallPaths() []string {
	return c.InstallPaths[runtime.GOOS]
}

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

func GetVSCodeFork(name string) (*VSCodeForkConfig, bool) {
	config, exists := VSCodeForks[name]
	return config, exists
}

func GetSupportedForks() []string {
	forks := make([]string, 0, len(VSCodeForks))
	for name := range VSCodeForks {
		forks = append(forks, name)
	}
	return forks
}
