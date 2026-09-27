package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jfrog/jfrog-cli-core/v2/common/format"
	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var deepProbeTimeout = 5 * time.Second

// SetupStatusCommand implements `jf setup <pm> --status`: a read-only report of whether
// the configuration `jf setup` writes for a package manager points at the given server.
type SetupStatusCommand struct {
	packageManager project.ProjectType
	serverDetails  *config.ServerDetails
	deep           bool
	format         format.OutputFormat
	result         PackageManagerStatus
}

func NewSetupStatusCommand(packageManager project.ProjectType) *SetupStatusCommand {
	return &SetupStatusCommand{packageManager: packageManager, format: format.Table}
}

func (ssc *SetupStatusCommand) SetServerDetails(serverDetails *config.ServerDetails) *SetupStatusCommand {
	ssc.serverDetails = serverDetails
	return ssc
}

// SetDeep makes the command probe the configured repository with the stored credentials.
// This is the only mode in which the command contacts the network.
func (ssc *SetupStatusCommand) SetDeep(deep bool) *SetupStatusCommand {
	ssc.deep = deep
	return ssc
}

func (ssc *SetupStatusCommand) SetFormat(outputFormat format.OutputFormat) *SetupStatusCommand {
	ssc.format = outputFormat
	return ssc
}

func (ssc *SetupStatusCommand) CommandName() string {
	return "setup_status_" + ssc.packageManager.String()
}

// ServerDetails returns no server, so commands.Exec neither reports usage to it nor
// refreshes its token: plain --status does not contact the server at all, and --deep
// sends only its single probe.
func (ssc *SetupStatusCommand) ServerDetails() (*config.ServerDetails, error) {
	return nil, nil
}

// Result returns the status found by the last Run.
func (ssc *SetupStatusCommand) Result() PackageManagerStatus {
	return ssc.result
}

func (ssc *SetupStatusCommand) Run() error {
	found, err := inspect(ssc.packageManager, ssc.serverDetails)
	if err != nil {
		return err
	}
	if ssc.deep && found.status.State == StateConfigured && found.status.RepoKey != "" {
		deep := probeRepository(ssc.serverDetails, found.status.RepoKey, found.credentials, found.status.Credentials)
		found.status.Deep = &deep
	}
	ssc.result = found.status
	return ssc.print()
}

func (ssc *SetupStatusCommand) print() error {
	switch ssc.format {
	case format.Json:
		data, err := json.MarshalIndent(ssc.result, "", "  ")
		if err != nil {
			return errorutils.CheckError(err)
		}
		log.Output(string(data))
		return nil
	case format.Table, "":
		return coreutils.PrintTable(statusTableRows(ssc.result), "", "", false)
	default:
		return errorutils.CheckErrorf("unsupported format '%s' for setup status; use json or table", ssc.format)
	}
}

type statusTableRow struct {
	Field string `col-name:"Field"`
	Value string `col-name:"Value"`
}

func statusTableRows(status PackageManagerStatus) []statusTableRow {
	rows := []statusTableRow{
		{"Package manager", status.PackageManager},
		{"State", string(status.State)},
	}
	appendIfSet := func(field, value string) {
		if value != "" {
			rows = append(rows, statusTableRow{field, value})
		}
	}
	appendIfSet("Host", status.Host)
	appendIfSet("Repository", status.RepoKey)
	appendIfSet("Location", status.Location)
	appendIfSet("Credentials", string(status.Credentials))
	if status.BinaryFound != nil {
		rows = append(rows, statusTableRow{"Binary found", fmt.Sprint(*status.BinaryFound)})
	}
	for _, override := range status.OverriddenBy {
		value := override.Source
		if override.Path != "" {
			value += " (" + override.Path + ")"
		}
		rows = append(rows, statusTableRow{"Overridden by", value})
	}
	if status.Deep != nil {
		rows = append(rows,
			statusTableRow{"Repository reachable", fmt.Sprint(status.Deep.RepoReachable)},
			statusTableRow{"Authentication OK", string(status.Deep.AuthOk)})
		appendIfSet("Deep check error", status.Deep.Error)
	}
	return rows
}

// probeRepository asks Artifactory for the repository's configuration, authenticating with
// the credentials stored in the package manager's own configuration - never the server's,
// because those are what the package manager will send, and they can expire while the
// server's keep being refreshed. One endpoint serves every repository type. The storage
// API is avoided: on a virtual repository it aggregates every member and can take far
// longer than the probe timeout. Redirects are not followed, so the credentials never
// reach another host. credentialsState is what the inspection reported: credentials it
// saw but could not read (a credential store) make the probe anonymous, not "absent".
func probeRepository(serverDetails *config.ServerDetails, repoKey string, creds storedCredentials, credentialsState CredentialsState) DeepStatus {
	endpoint := strings.TrimSuffix(serverDetails.GetArtifactoryUrl(), "/") + "/api/repositories/" + url.PathEscape(repoKey)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return DeepStatus{AuthOk: ProbeUnknown, Error: err.Error()}
	}
	switch {
	case creds.token != "":
		request.Header.Set("Authorization", "Bearer "+creds.token)
	case creds.basicAuth != "":
		request.Header.Set("Authorization", "Basic "+creds.basicAuth)
	case creds.password != "":
		request.SetBasicAuth(creds.user, creds.password)
	}
	client := &http.Client{
		Timeout:       deepProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		log.Debug("Setup status deep probe failed:", err.Error())
		return DeepStatus{AuthOk: ProbeUnknown, Error: probeErrorMessage(err)}
	}
	defer func() { _ = response.Body.Close() }()

	authenticated := !creds.isEmpty()
	switch code := response.StatusCode; code {
	case http.StatusOK:
		if authenticated {
			return DeepStatus{RepoReachable: true, AuthOk: ProbeTrue}
		}
		return DeepStatus{RepoReachable: true, AuthOk: ProbeUnknown}
	case http.StatusUnauthorized, http.StatusForbidden:
		if authenticated {
			return DeepStatus{AuthOk: ProbeFalse, Error: fmt.Sprintf("the stored credentials were rejected (HTTP %d)", code)}
		}
		if credentialsState == CredentialsAbsent {
			return DeepStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("the server requires credentials and none are stored (HTTP %d)", code)}
		}
		return DeepStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("the server requires credentials, and status cannot read the ones the package manager uses (HTTP %d)", code)}
	case http.StatusBadRequest, http.StatusNotFound:
		// Artifactory answers 400 for an unknown repository key.
		return DeepStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("repository %s was not found (HTTP %d)", repoKey, code)}
	default:
		return DeepStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("unexpected response (HTTP %d)", code)}
	}
}

// probeErrorMessage describes a failed request without its URL, which the table already shows as Host.
func probeErrorMessage(err error) string {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err.Error()
	}
	if urlErr.Timeout() {
		return fmt.Sprintf("no response within %s", deepProbeTimeout)
	}
	return urlErr.Err.Error()
}
