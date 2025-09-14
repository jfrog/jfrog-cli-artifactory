package npm

import "github.com/jfrog/jfrog-client-go/utils/log"

type npmInstall struct {
	*NpmCommand
}

func (ni *npmInstall) PrepareInstallPrerequisites(repo string) error {
	log.Debug("Skipping npm install preparation on repository: ", repo)
	return nil
}

func (ni *npmInstall) Run() (err error) {
	if err = ni.prepareBuildInfoModule(); err != nil {
		return
	}
	err = ni.collectDependencies()
	return
}

func (ni *npmInstall) RestoreNpmrc() error {
	// No need to restore the npmrc file, since we are using user's npmrc
	return nil
}
