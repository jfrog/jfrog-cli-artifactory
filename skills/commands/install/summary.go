package install

import (
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	skillInstallStatusOK     = "ok"
	skillInstallStatusFailed = "failed"
	skillInstallDetailOK     = "Executed successfully with no issues."
)

type installResult struct {
	Agent  string `col-name:"Agent"`
	Scope  string `col-name:"Scope"`
	Path   string `col-name:"Path"`
	Status string `col-name:"Status"`
	Detail string `col-name:"Detail"`
}

func printSummary(slug, version string, results []installResult) {
	if len(results) == 0 {
		return
	}
	log.Info("Skill installation summary for '" + slug + "' v" + version + ":")
	if err := coreutils.PrintTable(results, "Installed", "No skills installed", false); err != nil {
		log.Warn("Failed to render install summary: " + err.Error())
	}
}
