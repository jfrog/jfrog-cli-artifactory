package pnpm

import "github.com/jfrog/jfrog-client-go/utils/log"

type pnpmRtInstall struct {
	*PnpmCommand
}

func (pri *pnpmRtInstall) PrepareInstallPrerequisites(repo string) (err error) {
	log.Debug("Executing pnpm install command using jfrog RT on repository: ", repo)
	if err = pri.setArtifactoryAuth(); err != nil {
		return err
	}

	if err = pri.setNpmAuthRegistry(repo); err != nil {
		return err
	}

	return pri.setRestoreNpmrcFunc()
}

func (pri *pnpmRtInstall) Run() (err error) {
	if err = pri.CreateTempNpmrc(); err != nil {
		return
	}
	if err = pri.prepareBuildInfoModule(); err != nil {
		return
	}
	err = pri.collectDependencies()
	return
}

func (pri *pnpmRtInstall) RestoreNpmrc() (err error) {
	// Restore the npmrc file, since we are using our own npmrc
	return pri.restoreNpmrcFunc()
}
