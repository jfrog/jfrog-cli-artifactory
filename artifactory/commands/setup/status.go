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
	"github.com/jfrog/jfrog-client-go/http/httpclient"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var verifyTimeout = 5 * time.Second

// SetupStatusCommand implements `jf setup <pm> --status`: a read-only report of whether
// the configuration `jf setup` writes for a package manager points at the given server.
type SetupStatusCommand struct {
	packageManager project.ProjectType
	serverDetails  *config.ServerDetails
	verify         bool
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

// SetVerify makes the command check the configured repository with the stored credentials.
// This is the only mode in which the command contacts the network.
func (ssc *SetupStatusCommand) SetVerify(verify bool) *SetupStatusCommand {
	ssc.verify = verify
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
// refreshes its token: plain --status does not contact the server at all, and --verify
// sends only its single request.
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
	if ssc.verify {
		verify := ssc.verifyRepository(found)
		found.status.Verify = &verify
	}
	ssc.result = found.status
	return ssc.print()
}

// verifyRepository checks the configured repository, or says why there is none to check.
func (ssc *SetupStatusCommand) verifyRepository(found inspection) VerifyStatus {
	notVerified := func(reason string) VerifyStatus {
		return VerifyStatus{AuthOk: ProbeUnknown, Error: "not verified: " + reason}
	}
	switch {
	case found.status.State == StateUnsupported:
		return notVerified("status does not support " + ssc.packageManager.String())
	case found.status.State != StateConfigured:
		return notVerified("the configuration does not point at this server")
	case ssc.packageManager == project.Docker || ssc.packageManager == project.Podman || ssc.packageManager == project.Helm:
		return notVerified(ssc.packageManager.String() + " logs in to the registry, not to a repository, so there is no repository to check")
	case found.status.RepoKey == "":
		return notVerified("no repository key could be read from the configured URL")
	}
	return probeRepository(ssc.serverDetails, found.status.RepoKey, found.credentials, found.status.Credentials)
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
	if status.Verify != nil {
		rows = append(rows,
			statusTableRow{"Repository reachable", fmt.Sprint(status.Verify.RepoReachable)},
			statusTableRow{"Authentication OK", string(status.Verify.AuthOk)})
		appendIfSet("Verification error", status.Verify.Error)
	}
	return rows
}

// probeRepository asks Artifactory for the repository's configuration, authenticating with
// the credentials stored in the package manager's own configuration - never the server's,
// because those are what the package manager will send, and they can expire while the
// server's keep being refreshed. One endpoint serves every repository type. The storage
// API is avoided: on a virtual repository it aggregates every member and can take far
// longer than the probe timeout. Redirects are not followed, so the credentials never
// reach another host. The TLS settings are the server's, as for every other jf request.
// credentialsState is what the inspection reported: credentials it saw but could not read
// (a credential store) make the probe anonymous, not "absent".
func probeRepository(serverDetails *config.ServerDetails, repoKey string, creds storedCredentials, credentialsState CredentialsState) VerifyStatus {
	endpoint := strings.TrimSuffix(serverDetails.GetArtifactoryUrl(), "/") + "/api/repositories/" + url.PathEscape(repoKey)
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return VerifyStatus{AuthOk: ProbeUnknown, Error: err.Error()}
	}
	switch {
	case creds.token != "":
		request.Header.Set("Authorization", "Bearer "+creds.token)
	case creds.basicAuth != "":
		request.Header.Set("Authorization", "Basic "+creds.basicAuth)
	case creds.password != "":
		request.SetBasicAuth(creds.user, creds.password)
	}
	client, err := verifyHTTPClient(serverDetails)
	if err != nil {
		return VerifyStatus{AuthOk: ProbeUnknown, Error: err.Error()}
	}
	response, err := client.Do(request)
	if err != nil {
		message := probeErrorMessage(err)
		log.Debug("Setup status verification failed:", message)
		return VerifyStatus{AuthOk: ProbeUnknown, Error: message}
	}
	defer func() { _ = response.Body.Close() }()

	authenticated := !creds.isEmpty()
	switch code := response.StatusCode; code {
	case http.StatusOK:
		if authenticated {
			return VerifyStatus{RepoReachable: true, AuthOk: ProbeTrue}
		}
		return VerifyStatus{RepoReachable: true, AuthOk: ProbeUnknown}
	case http.StatusUnauthorized, http.StatusForbidden:
		if authenticated && code == http.StatusForbidden {
			// Recognised but not permitted: the package manager cannot read the repository either.
			return VerifyStatus{AuthOk: ProbeFalse, Error: fmt.Sprintf("the stored credentials are not allowed to read repository %s (HTTP %d)", repoKey, code)}
		}
		if authenticated {
			return VerifyStatus{AuthOk: ProbeFalse, Error: fmt.Sprintf("the stored credentials were rejected (HTTP %d)", code)}
		}
		if credentialsState == CredentialsAbsent {
			return VerifyStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("the server requires credentials and none are stored (HTTP %d)", code)}
		}
		return VerifyStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("the server requires credentials, and status cannot read the ones the package manager uses (HTTP %d)", code)}
	case http.StatusBadRequest, http.StatusNotFound:
		// Artifactory answers 400 for an unknown repository key.
		return VerifyStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("repository %s was not found (HTTP %d)", repoKey, code)}
	default:
		return VerifyStatus{AuthOk: ProbeUnknown, Error: fmt.Sprintf("unexpected response (HTTP %d)", code)}
	}
}

// verifyHTTPClient builds a client with the server's TLS settings - the certificates jf
// trusts, InsecureTls and the client certificate - that times out and never follows a
// redirect.
func verifyHTTPClient(serverDetails *config.ServerDetails) (*http.Client, error) {
	certsPath, err := coreutils.GetJfrogCertsDir()
	if err != nil {
		return nil, err
	}
	httpClient, err := httpclient.ClientBuilder().
		SetCertificatesPath(certsPath).
		SetInsecureTls(serverDetails.InsecureTls).
		SetClientCertPath(serverDetails.GetClientCertPath()).
		SetClientCertKeyPath(serverDetails.GetClientCertKeyPath()).
		SetOverallRequestTimeout(verifyTimeout).
		Build()
	if err != nil {
		return nil, err
	}
	client := httpClient.GetClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, nil
}

// probeErrorMessage describes a failed request without its URL, which the table already shows as Host.
func probeErrorMessage(err error) string {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err.Error()
	}
	if urlErr.Timeout() {
		return fmt.Sprintf("no response within %s", verifyTimeout)
	}
	return urlErr.Err.Error()
}
