package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// Summary row statuses for install tables and JSON output.
const (
	SummaryStatusOK        = "ok"
	SummaryStatusFailed    = "failed"
	SummaryStatusSkipped   = "skipped"
	SummaryDetailOKInstall = "Executed successfully with no issues."
)

// SummaryRow is one row in the install summary table.
type SummaryRow struct {
	Agent  string `json:"agent" col-name:"Agent"`
	Scope  string `json:"scope" col-name:"Scope"`
	Path   string `json:"path" col-name:"Path"`
	Status string `json:"status" col-name:"Status"`
	Detail string `json:"detail" col-name:"Detail"`
}

type summaryJSONPayload struct {
	Slug    string       `json:"slug"`
	Version string       `json:"version"`
	Results []SummaryRow `json:"results"`
}

// PrintInstallSummary renders a table or JSON summary of an install run.
// label is the human-readable package noun used in headings (e.g. "skill", "plugin").
func PrintInstallSummary(label, slug, version string, results []SummaryRow, format string) error {
	if len(results) == 0 {
		return nil
	}
	if strings.EqualFold(format, "json") {
		payload := summaryJSONPayload{Slug: slug, Version: version, Results: results}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal install summary: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	log.Info(fmt.Sprintf("%s installation summary for '%s' v%s:", capitalizeFirst(label), slug, version))
	emptyMessage := fmt.Sprintf("No %ss installed", label)
	if err := coreutils.PrintTable(results, "Installed", emptyMessage, false); err != nil {
		log.Warn("Failed to render install summary: " + err.Error())
	}
	return nil
}

func capitalizeFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
