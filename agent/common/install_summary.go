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

// InstallFailureRow builds a failed install/update summary row for one target.
func InstallFailureRow(agentName, scope, destinationDir string, err error) SummaryRow {
	return SummaryRow{
		Agent:  agentName,
		Scope:  scope,
		Path:   destinationDir,
		Status: SummaryStatusFailed,
		Detail: err.Error(),
	}
}

// UpdateAllSummaryRow is one row in a combined install/update summary for update --all.
// Table columns (struct field order): Agent, Name, Scope, Path, Status, Detail.
type UpdateAllSummaryRow struct {
	Agent   string `json:"agent" col-name:"Agent"`
	Name    string `json:"name" col-name:"Name"`
	Scope   string `json:"scope" col-name:"Scope"`
	Path    string `json:"path" col-name:"Path"`
	Status  string `json:"status" col-name:"Status"`
	Detail  string `json:"detail" col-name:"Detail"`
	Version string `json:"version,omitempty"`
}

// AppendUpdateAllSummaryRows copies per-target summary rows into a combined --all summary.
func AppendUpdateAllSummaryRows(dest []UpdateAllSummaryRow, name, version string, rows []SummaryRow) []UpdateAllSummaryRow {
	for _, row := range rows {
		dest = append(dest, UpdateAllSummaryRow{
			Agent:   row.Agent,
			Name:    name,
			Scope:   row.Scope,
			Path:    row.Path,
			Status:  row.Status,
			Detail:  row.Detail,
			Version: version,
		})
	}
	return dest
}

type updateAllSummaryJSON struct {
	Results []UpdateAllSummaryRow `json:"results"`
}

// PrintUpdateAllSummary renders one table or JSON blob for update --all across many packages.
func PrintUpdateAllSummary(entityLabel string, results []UpdateAllSummaryRow, format string) error {
	if len(results) == 0 {
		return nil
	}
	if strings.EqualFold(format, "json") {
		payload := updateAllSummaryJSON{Results: results}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal update-all summary: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	log.Info(entityLabel + " update summary (--all):")
	emptyMsg := "No " + strings.ToLower(entityLabel) + "s updated"
	if err := coreutils.PrintTable(results, "Updated", emptyMsg, false); err != nil {
		log.Warn("Failed to render update-all summary: " + err.Error())
	}
	return nil
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
