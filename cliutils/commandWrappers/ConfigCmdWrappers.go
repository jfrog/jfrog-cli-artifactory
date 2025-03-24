package commandWrappers

import (
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
)

func DeprecationConfigCmdWarningWrapper(cmdName, oldSubcommand string, confType project.ProjectType, c *components.Context,
	cmd func(c *components.Context, confType project.ProjectType) error) error {
	utils.LogCommandRemovalNotice(cmdName, oldSubcommand)
	return cmd(c, confType)
}
