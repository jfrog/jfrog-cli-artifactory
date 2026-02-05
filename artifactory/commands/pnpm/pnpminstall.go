package pnpm

import "github.com/jfrog/jfrog-client-go/utils/log"

type pnpmInstall struct {
	*PnpmCommand
}

func (pi *pnpmInstall) PrepareInstallPrerequisites(repo string) error {
	log.Debug("Skipping pnpm install preparation on repository: ", repo)
	return nil
}

func (pi *pnpmInstall) Run() (err error) {
	if err = pi.prepareBuildInfoModule(); err != nil {
		return
	}
	err = pi.collectDependencies()
	return
}

func (pi *pnpmInstall) RestoreNpmrc() error {
	// No need to restore the npmrc file, since we are using user's npmrc
	return nil
}
