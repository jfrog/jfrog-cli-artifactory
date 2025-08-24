package sonar

import (
	"bufio"
	"os"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

type ReportTask struct {
	CeTaskURL  string
	CeTaskID   string
	AnalysisID string
	ProjectKey string
	ServerURL  string
}

func parseReportTask(path string) (ReportTask, error) {
	rt := ReportTask{}
	f, e := os.Open(path)
	if e != nil {
		return rt, errorutils.CheckErrorf("failed to open report task file '%s': %v", path, e)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ceTaskUrl=") {
			val := strings.TrimPrefix(line, "ceTaskUrl=")
			rt.CeTaskURL = val
			if idx := strings.LastIndex(val, "?id="); idx != -1 {
				rt.CeTaskID = val[idx+4:]
			}
		}
		if strings.HasPrefix(line, "ceTaskId=") {
			rt.CeTaskID = strings.TrimPrefix(line, "ceTaskId=")
		}
		if strings.HasPrefix(line, "analysisId=") {
			rt.AnalysisID = strings.TrimPrefix(line, "analysisId=")
		}
		if strings.HasPrefix(line, "projectKey=") {
			rt.ProjectKey = strings.TrimPrefix(line, "projectKey=")
		}
		if strings.HasPrefix(line, "serverUrl=") {
			rt.ServerURL = strings.TrimPrefix(line, "serverUrl=")
		}
	}
	if err := scanner.Err(); err != nil {
		return rt, errorutils.CheckErrorf("failed reading report task file '%s': %v", path, err)
	}
	return rt, nil
}
