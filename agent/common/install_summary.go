package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// Summary row statuses for install/update tables and JSON output.
const (
	SummaryStatusOK        = "ok"
	SummaryStatusFailed    = "failed"
	SummaryStatusSkipped   = "skipped"
	SummaryDetailOKInstall = "Executed successfully with no issues."
)

// SummaryRow is one row in the install/update summary table.
type SummaryRow struct {
	Agent  string `json:"agent" col-name:"Agent"`
	Scope  string `json:"scope" col-name:"Scope"`
	Path   string `json:"path" col-name:"Path"`
	Status string `json:"status" col-name:"Status"`
	Detail string `json:"detail" col-name:"Detail"`
}

type summaryJSON struct {
	Slug    string       `json:"slug"`
	Version string       `json:"version"`
	Results []SummaryRow `json:"results"`
}

// PrintInstallSummary renders a table or JSON summary of an install/update run.
// entityLabel is used in the table heading (e.g. "Skill" or "Plugin").
func PrintInstallSummary(entityLabel, slug, version string, results []SummaryRow, format string) error {
	if len(results) == 0 {
		return nil
	}
	if strings.EqualFold(format, "json") {
		payload := summaryJSON{Slug: slug, Version: version, Results: results}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal install summary: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	log.Info(entityLabel + " installation summary for '" + slug + "' v" + version + ":")
	if err := coreutils.PrintTable(results, "Installed", "No "+strings.ToLower(entityLabel)+"s installed", false); err != nil {
		log.Warn("Failed to render install summary: " + err.Error())
	}
	return nil
}
