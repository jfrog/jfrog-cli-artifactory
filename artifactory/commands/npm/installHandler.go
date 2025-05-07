package npm

type Installer interface {
	PrepareInstallPrerequisites(repo string) error
	Run() error
	RestoreNpmrcFunc() error
}

// Get npm implementation
func NpmInstallStrategy(shouldUseNpmRc bool, npmCommand *NpmCommand) Installer {

	if shouldUseNpmRc {
		return &npmInstall{npmCommand}
	}

	return &npmRtInstall{npmCommand}
}
