package install

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

const (
	skillInstallStatusOK     = "ok"
	skillInstallStatusFailed = "failed"
	skillInstallDetailOK     = "Executed successfully with no issues."
)

type installAttemptResult struct {
	Agent  string `json:"agent" col-name:"Agent"`
	Scope  string `json:"scope" col-name:"Scope"`
	Path   string `json:"path" col-name:"Path"`
	Status string `json:"status" col-name:"Status"`
	Detail string `json:"detail" col-name:"Detail"`
}

type installSummaryJSON struct {
	Slug    string                 `json:"slug"`
	Version string                 `json:"version"`
	Results []installAttemptResult `json:"results"`
}

func printSummary(slug, version string, results []installAttemptResult, format string) error {
	if len(results) == 0 {
		return nil
	}
	if strings.EqualFold(format, "json") {
		payload := installSummaryJSON{Slug: slug, Version: version, Results: results}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal install summary: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	log.Info("Skill installation summary for '" + slug + "' v" + version + ":")
	if err := coreutils.PrintTable(results, "Installed", "No skills installed", false); err != nil {
		log.Warn("Failed to render install summary: " + err.Error())
	}
	return nil
}
