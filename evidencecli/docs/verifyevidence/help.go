package verifyevidence

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Verify an evidence."
}

func GetArguments() []components.Argument {
	return []components.Argument{{Name: "evidence PUK key", Description: "PUK key path."}, {Name: "evidence repo path", Description: "Evidence path as a key path or url."}}
}
