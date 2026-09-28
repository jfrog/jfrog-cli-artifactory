package setup

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jfrog/jfrog-cli-core/v2/common/project"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// StatusSchemaVersion is the version of the JSON document `jf setup <pm> --status` prints.
// Fields are only ever added to that document; a breaking change bumps this number.
const StatusSchemaVersion = 1

// ConfigState says where a package manager resolves from, according to the user-level
// configuration `jf setup` writes for it.
type ConfigState string

const (
	// StateConfigured means the configuration points at the given Artifactory server.
	StateConfigured ConfigState = "configured"
	// StateOtherHost means the setting `jf setup` owns holds a value that is neither the
	// package manager's public default nor the given server.
	StateOtherHost ConfigState = "other-host"
	// StateNotConfigured means the setting is unset, or set to the public default.
	StateNotConfigured ConfigState = "not-configured"
	// StateUnsupported means status cannot inspect this package manager yet.
	StateUnsupported ConfigState = "unsupported"
)

// CredentialsState reports whether the package manager's own configuration stores
// credentials for the server. It is independent of ConfigState, because anonymous setup
// is legitimate.
type CredentialsState string

const (
	CredentialsPresent       CredentialsState = "present"
	CredentialsAbsent        CredentialsState = "absent"
	CredentialsNotApplicable CredentialsState = "not-applicable"
	// CredentialsUnknown is reported when the package manager keeps credentials in a
	// store status cannot inspect (for example uv's credential store).
	CredentialsUnknown CredentialsState = "unknown"
)

// ProbeResult is a three-valued answer: the deep probe cannot always decide.
type ProbeResult string

const (
	ProbeTrue    ProbeResult = "true"
	ProbeFalse   ProbeResult = "false"
	ProbeUnknown ProbeResult = "unknown"
)

// ConfigOverride is a configuration that wins over the user-level one in the current
// environment, so the package manager may not resolve where State says.
type ConfigOverride struct {
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

// DeepStatus is the result of probing the configured repository with the stored credentials.
type DeepStatus struct {
	RepoReachable bool        `json:"repoReachable"`
	AuthOk        ProbeResult `json:"authOk"`
	// Error says why the probe did not confirm the repository. It is empty on success.
	Error string `json:"error,omitempty"`
}

// PackageManagerStatus is the result of `jf setup <pm> --status`.
type PackageManagerStatus struct {
	SchemaVersion  int              `json:"schemaVersion"`
	PackageManager string           `json:"packageManager"`
	State          ConfigState      `json:"state"`
	Host           string           `json:"host,omitempty"`
	RepoKey        string           `json:"repoKey,omitempty"`
	Location       string           `json:"location,omitempty"`
	Credentials    CredentialsState `json:"credentials,omitempty"`
	// BinaryFound is nil only for unsupported package managers, which are not inspected.
	BinaryFound  *bool            `json:"binaryFound,omitempty"`
	OverriddenBy []ConfigOverride `json:"overriddenBy,omitempty"`
	Deep         *DeepStatus      `json:"deep,omitempty"`
}

// storedCredentials are the credentials read from the package manager's own
// configuration. They are only used by the deep probe and are never serialized.
type storedCredentials struct {
	user     string
	password string
	token    string
	// basicAuth is an already base64-encoded "user:password" (npm's _auth).
	basicAuth string
}

func (c storedCredentials) isEmpty() bool {
	return c.password == "" && c.token == "" && c.basicAuth == ""
}

// inspection is what an inspector found: the public status plus the credentials the
// deep probe needs.
type inspection struct {
	status      PackageManagerStatus
	credentials storedCredentials
}

// configInspector reads, without modifying anything, the configuration `jf setup`
// writes for one package manager.
type configInspector interface {
	status(serverDetails *config.ServerDetails) (inspection, error)
}

type inspectorFunc func(serverDetails *config.ServerDetails) (inspection, error)

func (f inspectorFunc) status(serverDetails *config.ServerDetails) (inspection, error) {
	return f(serverDetails)
}

// inspectors holds a status reader per package manager. Every other package manager in
// packageManagerToRepositoryPackageType must be listed in unsupportedInspection;
// TestInspectors_CoverEverySupportedPackageManager asserts it.
var inspectors = map[project.ProjectType]configInspector{
	project.Npm:    inspectorFunc(inspectNpm),
	project.Pnpm:   inspectorFunc(inspectPnpm),
	project.Pip:    inspectorFunc(func(sd *config.ServerDetails) (inspection, error) { return inspectPip(project.Pip, sd) }),
	project.Pipenv: inspectorFunc(func(sd *config.ServerDetails) (inspection, error) { return inspectPip(project.Pipenv, sd) }),
	project.UV:     inspectorFunc(inspectUV),
	project.Go:     inspectorFunc(inspectGo),
	project.Maven:  inspectorFunc(inspectMaven),
	project.Gradle: inspectorFunc(inspectGradle),
	project.Nuget:  inspectorFunc(func(sd *config.ServerDetails) (inspection, error) { return inspectNuget(project.Nuget, sd) }),
	project.Dotnet: inspectorFunc(func(sd *config.ServerDetails) (inspection, error) { return inspectNuget(project.Dotnet, sd) }),
	project.Docker: inspectorFunc(inspectDocker),
	project.Podman: inspectorFunc(inspectPodman),
	project.Helm:   inspectorFunc(inspectHelm),
}

var unsupportedInspection = []project.ProjectType{
	project.Yarn, project.Poetry, project.Twine, project.Cargo, project.Ruby,
	project.Apt, project.Apk, project.Apm, project.Choco, project.PSResource,
}

// toolTimeout bounds each package manager query, so a hung tool cannot hang the command.
const toolTimeout = 15 * time.Second

// runTool runs a package manager binary to ask where it keeps its configuration, and
// returns its stdout. It runs outside the current project, so a go.mod or package.json
// there cannot make the tool switch versions, and with toolchain and Corepack downloads
// disabled, so the query never reaches the network. Tests replace it so they never depend
// on the binaries installed on the machine.
var runTool = func(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "COREPACK_ENABLE_NETWORK=0")
	return cmd.Output()
}

// lookPath reports where a binary is on PATH. Tests replace it together with runTool.
var lookPath = exec.LookPath

func binaryFound(names ...string) bool {
	for _, name := range names {
		if _, err := lookPath(name); err == nil {
			return true
		}
	}
	return false
}

// GetStatus reports whether the user-level configuration `jf setup` writes for
// packageManager points at the given server. It reads files and may run the package
// manager's binary or a container credential helper's list action, but never prompts
// and never contacts the network.
func GetStatus(packageManager project.ProjectType, serverDetails *config.ServerDetails) (PackageManagerStatus, error) {
	result, err := inspect(packageManager, serverDetails)
	return result.status, err
}

func inspect(packageManager project.ProjectType, serverDetails *config.ServerDetails) (inspection, error) {
	if !IsSupportedPackageManager(packageManager) {
		return inspection{}, errorutils.CheckErrorf("unsupported package manager: %s", packageManager)
	}
	if serverDetails == nil || serverDetails.GetArtifactoryUrl() == "" {
		return inspection{}, errorutils.CheckErrorf("no Artifactory URL is configured; provide --url or --server-id, or configure a default server with 'jf config add'")
	}
	inspector, ok := inspectors[packageManager]
	if !ok {
		return inspection{status: PackageManagerStatus{
			SchemaVersion:  StatusSchemaVersion,
			PackageManager: packageManager.String(),
			State:          StateUnsupported,
			Location:       packageManagerConfigs[packageManager].location,
		}}, nil
	}
	result, err := inspector.status(serverDetails)
	if err != nil {
		return inspection{}, fmt.Errorf("failed to read the %s configuration: %w", packageManager.String(), err)
	}
	result.status.SchemaVersion = StatusSchemaVersion
	result.status.PackageManager = packageManager.String()
	if result.status.Location == "" {
		result.status.Location = packageManagerConfigs[packageManager].location
	}
	if result.status.Credentials == "" {
		result.status.Credentials = CredentialsNotApplicable
	}
	return result, nil
}

func newInspection(found bool) inspection {
	return inspection{status: PackageManagerStatus{State: StateNotConfigured, BinaryFound: &found}}
}

// normalizedEndpoint is a URL reduced to what decides whether two URLs name the same
// Artifactory: the host in lower case, the port only when it is not the scheme default,
// and the path without a trailing slash. Scheme and credentials are ignored.
type normalizedEndpoint struct {
	host string
	path string
}

func normalizeEndpoint(rawURL string) (normalizedEndpoint, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return normalizedEndpoint{}, false
	}
	return normalizedEndpoint{
		host: normalizeHostPort(parsed.Scheme, parsed.Host),
		path: strings.TrimSuffix(parsed.EscapedPath(), "/"),
	}, true
}

func normalizeHostPort(scheme, hostPort string) string {
	hostPort = strings.ToLower(hostPort)
	switch {
	case strings.HasSuffix(hostPort, ":443") && (scheme == "" || strings.EqualFold(scheme, "https")),
		strings.HasSuffix(hostPort, ":80") && (scheme == "" || strings.EqualFold(scheme, "http")):
		return hostPort[:strings.LastIndex(hostPort, ":")]
	}
	return hostPort
}

// matchServerURL reports whether rawURL is under the server's Artifactory URL, and returns
// the remainder of the path (without a leading slash) for parsing the repository key.
// Matching on the full URL rather than the host keeps two Artifactory instances served
// from the same host under different paths apart.
func matchServerURL(rawURL string, serverDetails *config.ServerDetails) (matched bool, rest string) {
	server, ok := normalizeEndpoint(serverDetails.GetArtifactoryUrl())
	if !ok {
		return false, ""
	}
	candidate, ok := normalizeEndpoint(rawURL)
	if !ok || candidate.host != server.host {
		return false, ""
	}
	candidatePath, serverPath := strings.ToLower(candidate.path), strings.ToLower(server.path)
	if candidatePath == serverPath {
		return true, ""
	}
	if !strings.HasPrefix(candidatePath, serverPath+"/") {
		return false, ""
	}
	return true, candidate.path[len(serverPath)+1:]
}

// matchServerHost reports whether a registry host (as Docker, Podman and Helm key their
// credentials) is the server's registry host. Those tools key logins by host only.
func matchServerHost(registry string, serverDetails *config.ServerDetails) bool {
	serverHost, err := deriveContainerRegistryHost(serverDetails)
	if err != nil {
		return false
	}
	return registryHost(registry) == normalizeHostPort("https", serverHost)
}

// registryHost reduces a container credential key ("acme.jfrog.io",
// "https://acme.jfrog.io/v1/") to its host.
func registryHost(key string) string {
	key = strings.TrimSpace(key)
	scheme := "https"
	if i := strings.Index(key, "://"); i != -1 {
		scheme = key[:i]
		key = key[i+len("://"):]
	}
	if i := strings.Index(key, "/"); i != -1 {
		key = key[:i]
	}
	return normalizeHostPort(scheme, key)
}

// hostOf returns the host of rawURL for display, without credentials.
func hostOf(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Host
}

// publicDefaultHosts are the registries each package manager uses when nothing is set.
// Pointing at one of them reads as not-configured rather than other-host.
var publicDefaultHosts = map[project.ProjectType][]string{
	project.Npm:    {"registry.npmjs.org", "registry.npmjs.com", "registry.yarnpkg.com"},
	project.Pnpm:   {"registry.npmjs.org", "registry.npmjs.com", "registry.yarnpkg.com"},
	project.Pip:    {"pypi.org", "pypi.python.org"},
	project.Pipenv: {"pypi.org", "pypi.python.org"},
	project.UV:     {"pypi.org", "pypi.python.org"},
	project.Go:     {"proxy.golang.org"},
	project.Nuget:  {"api.nuget.org", "www.nuget.org", "nuget.org"},
	project.Dotnet: {"api.nuget.org", "www.nuget.org", "nuget.org"},
	project.Maven:  {"repo.maven.apache.org", "repo1.maven.org"},
}

func isPublicDefault(packageManager project.ProjectType, rawURL string) bool {
	value := strings.TrimSpace(rawURL)
	if packageManager == project.Go && (value == "direct" || value == "off") {
		return true
	}
	endpoint, ok := normalizeEndpoint(value)
	return ok && slices.Contains(publicDefaultHosts[packageManager], endpoint.host)
}

// classify decides the state of a URL-shaped setting. repoKeyFrom parses the repository
// key from the path under the server's Artifactory URL.
func classify(packageManager project.ProjectType, rawURL string, serverDetails *config.ServerDetails, repoKeyFrom func(rest string) string) (state ConfigState, host, repoKey string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return StateNotConfigured, "", ""
	}
	host = hostOf(rawURL)
	if isPublicDefault(packageManager, rawURL) {
		return StateNotConfigured, host, ""
	}
	if matched, rest := matchServerURL(rawURL, serverDetails); matched {
		if repoKeyFrom != nil {
			repoKey = repoKeyFrom(rest)
		}
		return StateConfigured, host, repoKey
	}
	return StateOtherHost, host, ""
}

// repoKeyAfter returns the path segment that follows the given marker segments, for
// example "npm-virtual" in "api/npm/npm-virtual/" with markers "api", "npm".
func repoKeyAfter(rest string, markers ...string) string {
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	for i := 0; i+len(markers) < len(segments); i++ {
		if slices.Equal(segments[i:i+len(markers)], markers) {
			repoKey, err := url.PathUnescape(segments[i+len(markers)])
			if err != nil {
				return segments[i+len(markers)]
			}
			return repoKey
		}
	}
	return ""
}

// firstSegment returns the first path segment of rest, for layouts that put the
// repository key directly under the Artifactory URL (Maven, Gradle).
func firstSegment(rest string) string {
	return repoKeyAfter(rest)
}

// credentialsFromURL returns the credentials embedded in rawURL's userinfo. A username
// without a password is not usable credentials.
func credentialsFromURL(rawURL string) (storedCredentials, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.User == nil {
		return storedCredentials{}, false
	}
	password, _ := parsed.User.Password()
	creds := storedCredentials{user: parsed.User.Username(), password: password}
	return creds, !creds.isEmpty()
}

var urlUserinfo = regexp.MustCompile(`//[^/@]*@`)

// redactURL masks the credentials of rawURL, keeping the scheme and host so the value
// still says where it points.
func redactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// An unparsable URL may still carry userinfo.
		return urlUserinfo.ReplaceAllString(rawURL, "//***:***@")
	}
	if parsed.User == nil {
		return rawURL
	}
	// Splice in a literal masked userinfo instead of going through url.UserPassword + String(),
	// which percent-encodes the mask ("*" -> "%2A") and prints noisy "%2A%2A%2A:%2A%2A%2A@host".
	parsed.User = nil
	return fmt.Sprintf("%s://***:***@%s", parsed.Scheme, strings.TrimPrefix(parsed.String(), parsed.Scheme+"://"))
}

// unmarshalXML decodes an XML config file whatever encoding its declaration names. The
// writers use etree, which accepts any declared encoding, so the reader must too.
func unmarshalXML(content []byte, v any) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	return decoder.Decode(v)
}

// readOptionalFile reads path, treating a missing file as empty.
func readOptionalFile(path string) (content []byte, exists bool, err error) {
	// #nosec G304 G703 -- path is the package manager's own config file, located the way the package manager does from env/home; status only reads it
	content, err = os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, errorutils.CheckError(err)
	}
	return content, true, nil
}

func boolPtr(value bool) *bool {
	return &value
}
