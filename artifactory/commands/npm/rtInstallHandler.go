package npm

import "github.com/jfrog/jfrog-client-go/utils/log"

type npmRtInstall struct {
	*NpmCommand
}

func (nri *npmRtInstall) PrepareInstallPrerequisites(repo string) (err error) {
	log.Debug("Preparing NPM installation for npm rt command, repo: ", repo)
	if err = nri.setArtifactoryAuth(); err != nil {
		return err
	}

	if err = nri.setNpmAuthRegistry(repo); err != nil {
		return err
	}

	return nri.setRestoreNpmrcFunc()
}

func (nri *npmRtInstall) Run() (err error) {
	if err = nri.CreateTempNpmrc(); err != nil {
		return
	}
	if err = nri.prepareBuildInfoModule(); err != nil {
		return
	}
	err = nri.collectDependencies()
	return
}

func (nri *npmRtInstall) RestoreNpmrcFunc() (err error) {
	// Restore the npmrc file, since we are using our own npmrc
	if err = nri.restoreNpmrcFunc(); err != nil {
		return err
	}
	return
}
