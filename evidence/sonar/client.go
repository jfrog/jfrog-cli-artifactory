package sonar

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/jfrog/jfrog-client-go/http/jfroghttpclient"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/httputils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

type TaskDetails struct {
	Task struct {
		ID                 string      `json:"id"`
		Type               string      `json:"type"`
		ComponentID        string      `json:"componentId"`
		ComponentKey       string      `json:"componentKey"`
		ComponentName      string      `json:"componentName"`
		ComponentQualifier string      `json:"componentQualifier"`
		AnalysisID         string      `json:"analysisId"`
		Status             string      `json:"status"`
		SubmittedAt        string      `json:"submittedAt"`
		StartedAt          string      `json:"startedAt"`
		ExecutedAt         string      `json:"executedAt"`
		ExecutionTimeMs    int         `json:"executionTimeMs"`
		Logs               interface{} `json:"logs"`
		HasScannerContext  bool        `json:"hasScannerContext"`
		Organization       string      `json:"organization"`
	} `json:"task"`
}

type Client interface {
	GetTaskDetails(ceTaskID string) (*TaskDetails, error)
	GetSonarIntotoStatement(ceTaskID string) ([]byte, error)
}

type httpClient struct {
	baseURL string
	token   string
	client  *jfroghttpclient.JfrogHttpClient
}

func NewClient(sonarURL, token string) (Client, error) {
	base := strings.TrimRight(sonarURL, "/")
	cli, err := jfroghttpclient.JfrogClientBuilder().Build()
	if err != nil {
		return nil, errorutils.CheckError(err)
	}
	return &httpClient{baseURL: base, token: token, client: cli}, nil
}

func (c *httpClient) authHeader() string {
	if c.token != "" {
		return "Bearer " + c.token
	}
	return ""
}

func (c *httpClient) doGET(urlStr string) ([]byte, int, error) {
	details := httputils.HttpClientDetails{Headers: map[string]string{}}
	if h := c.authHeader(); h != "" {
		details.Headers["Authorization"] = h
	}
	resp, body, _, err := c.client.SendGet(urlStr, true, &details)
	if err != nil {
		log.Debug("HTTP GET error for", urlStr, "error:", err.Error())
		return nil, 0, err
	}
	log.Debug("HTTP GET response for", urlStr, "status:", resp.StatusCode, "body:", string(body))
	return body, resp.StatusCode, nil
}

func (c *httpClient) GetSonarIntotoStatement(ceTaskID string) ([]byte, error) {
	if ceTaskID == "" {
		return nil, errorutils.CheckError(fmt.Errorf("missing ce task id for enterprise endpoint"))
	}
	baseURL := c.baseURL
	u, _ := url.Parse(baseURL)
	hostname := u.Hostname()
	// Get sonar intoto statement has api prefix before the hostname
	// if hostname is localhost or an ip address or has api prefix, then don't add the api.
	if hostname != "localhost" && net.ParseIP(hostname) == nil && !strings.HasPrefix(hostname, "api.") {
		baseURL = strings.Replace(baseURL, "://", "://api.", 1)
	}
	enterpriseURL := fmt.Sprintf("%s/dop-translation/jfrog-evidence/%s", baseURL, url.QueryEscape(ceTaskID))
	body, statusCode, err := c.doGET(enterpriseURL)
	if err != nil {
		return nil, errorutils.CheckErrorf("enterprise endpoint failed with status %d and response %s %v", statusCode, string(body), err)
	}
	if statusCode != 200 {
		return nil, errorutils.CheckErrorf("enterprise endpoint returned status %d: %s", statusCode, string(body))
	}
	return body, nil
}

func (c *httpClient) GetTaskDetails(ceTaskID string) (*TaskDetails, error) {
	if ceTaskID == "" {
		return nil, nil
	}
	taskURL := fmt.Sprintf("%s/api/ce/task?id=%s", c.baseURL, url.QueryEscape(ceTaskID))
	body, statusCode, err := c.doGET(taskURL)
	if err != nil {
		return nil, err
	}
	if statusCode != 200 {
		return nil, errorutils.CheckErrorf("task endpoint returned status %d: %s", statusCode, string(body))
	}
	var response TaskDetails
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, errorutils.CheckErrorf("failed to parse task response: %v", err)
	}
	return &response, nil
}
