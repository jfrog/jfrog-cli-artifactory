package pnpm

import "github.com/jfrog/jfrog-client-go/utils/log"

type Installer interface {
	PrepareInstallPrerequisites(repo string) error
	Run() error
	RestoreNpmrc() error
}

type PnpmInstallStrategy struct {
	strategy     Installer
	strategyName string
}

// Get pnpm implementation
func NewPnpmInstallStrategy(useNativeClient bool, pnpmCommand *PnpmCommand) *PnpmInstallStrategy {
	ppi := PnpmInstallStrategy{}
	if useNativeClient {
		ppi.strategy = &pnpmInstall{pnpmCommand}
		ppi.strategyName = "native"
	} else {
		ppi.strategy = &pnpmRtInstall{pnpmCommand}
		ppi.strategyName = "artifactory"
	}
	return &ppi
}

func (ppi *PnpmInstallStrategy) PrepareInstallPrerequisites(repo string) error {
	log.Debug("Using strategy for preparing install prerequisites: ", ppi.strategyName)
	return ppi.strategy.PrepareInstallPrerequisites(repo)
}

func (ppi *PnpmInstallStrategy) Install() error {
	log.Debug("Using strategy for pnpm install: ", ppi.strategyName)
	return ppi.strategy.Run()
}

func (ppi *PnpmInstallStrategy) RestoreNpmrc() error {
	return ppi.strategy.RestoreNpmrc()
}
