package verifyevidence

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

func GetDescription() string {
	return "Verify an evidencecli."
}

func GetArguments() []components.Argument {
	return []components.Argument{{Name: "evidencecli PUK key", Description: "PUK key path."}, {Name: "evidencecli repo path", Description: "Evidence path as a key path or url."}}
}
