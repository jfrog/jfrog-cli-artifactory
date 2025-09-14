package npm

import "github.com/jfrog/jfrog-client-go/utils/log"

type npmRtInstall struct {
	*NpmCommand
}

func (nri *npmRtInstall) PrepareInstallPrerequisites(repo string) (err error) {
	log.Debug("Executing npm install command using jfrog RT on repository: ", repo)
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

func (nri *npmRtInstall) RestoreNpmrc() (err error) {
	// Restore the npmrc file, since we are using our own npmrc
	return nri.restoreNpmrcFunc()
}
