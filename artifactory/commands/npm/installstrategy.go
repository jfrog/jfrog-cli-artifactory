package npm

import "github.com/jfrog/jfrog-client-go/utils/log"

type Installer interface {
	PrepareInstallPrerequisites(repo string) error
	Run() error
	RestoreNpmrc() error
}

type NpmInstallStrategy struct {
	strategy     Installer
	strategyName string
}

// Get npm implementation
func NewNpmInstallStrategy(useNativeClient bool, npmCommand *NpmCommand) *NpmInstallStrategy {
	npi := NpmInstallStrategy{}
	if useNativeClient {
		npi.strategy = &npmInstall{npmCommand}
		npi.strategyName = "native"
	} else {
		npi.strategy = &npmRtInstall{npmCommand}
		npi.strategyName = "artifactory"
	}
	return &npi
}

func (npi *NpmInstallStrategy) PrepareInstallPrerequisites(repo string) error {
	log.Debug("Using strategy for preparing install prerequisites: ", npi.strategyName)
	return npi.strategy.PrepareInstallPrerequisites(repo)
}

func (npi *NpmInstallStrategy) Install() error {
	log.Debug("Using strategy for npm install: ", npi.strategyName)
	return npi.strategy.Run()
}

func (npi *NpmInstallStrategy) RestoreNpmrc() error {
	return npi.strategy.RestoreNpmrc()
}
