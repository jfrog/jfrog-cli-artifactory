package cli

//go:generate ${PROJECT_DIR}/scripts/mockgen.sh ${GOFILE}

import (
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
)

type EvidenceCommands interface {
	CreateEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error
	GetEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error
	VerifyEvidence(ctx *components.Context, serverDetails *config.ServerDetails) error
}
