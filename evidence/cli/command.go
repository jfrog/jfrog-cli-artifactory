package cli

import (
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type EvidenceCommands interface {
	CreateEvidence(*coreConfig.ServerDetails) error
}
