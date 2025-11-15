package ideconsts

import "strings"

// IDE name constants
const (
	IDENameVSCode    = "vscode"
	IDENameCode      = "code"
	IDENameCursor    = "cursor"
	IDENameWindsurf  = "windsurf"
	IDENameKiro      = "kiro"
	IDENameJetBrains = "jetbrains"
	IDENameJB        = "jb"
)

// SupportedIDEsList contains the list of primary IDE names (used for display purposes)
var SupportedIDEsList = []string{
	IDENameVSCode,
	IDENameCursor,
	IDENameWindsurf,
	IDENameKiro,
	IDENameJetBrains,
}

// VSCodeBasedIDEs contains the list of VSCode-based IDEs (for flags and documentation)
var VSCodeBasedIDEs = []string{
	IDENameVSCode,
	IDENameCursor,
	IDENameWindsurf,
	IDENameKiro,
}

// GetSupportedIDEsString returns a comma-separated string of supported IDE names
func GetSupportedIDEsString() string {
	return strings.Join(SupportedIDEsList, ", ")
}

// GetVSCodeBasedIDEsString returns a slash-separated string of VSCode-based IDE names
func GetVSCodeBasedIDEsString() string {
	return strings.Join(VSCodeBasedIDEs, "/")
}
