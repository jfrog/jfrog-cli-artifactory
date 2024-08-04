package cli

//go:generate ${PROJECT_DIR}/scripts/mockgen.sh ${GOFILE}

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	coreConfig "github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type EvidenceCommands interface {
	CreateEvidence(ctx *components.Context, serverDetails *coreConfig.ServerDetails) error
}
