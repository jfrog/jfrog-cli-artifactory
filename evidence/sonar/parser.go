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
	ProjectKey string
	ServerURL  string
}

func parseReportTask(path string) (ReportTask, error) {
	rt := ReportTask{}

	props, err := readTaskReport(path)
	if err != nil {
		return rt, err
	}

	rt.CeTaskURL = props["ceTaskUrl"]
	rt.CeTaskID = props["ceTaskId"]
	rt.ProjectKey = props["projectKey"]
	rt.ServerURL = props["serverUrl"]

	if rt.CeTaskID == "" && rt.CeTaskURL != "" {
		if idx := strings.LastIndex(rt.CeTaskURL, "?id="); idx != -1 {
			rt.CeTaskID = rt.CeTaskURL[idx+4:]
		}
	}

	return rt, nil
}

func readTaskReport(path string) (map[string]string, error) {
	props := make(map[string]string)

	f, err := os.Open(path)
	if err != nil {
		return nil, errorutils.CheckErrorf("failed to open report task file '%s': %v", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			props[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errorutils.CheckErrorf("failed reading report task file '%s': %v", path, err)
	}

	return props, nil
}
