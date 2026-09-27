package setup

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/ioutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var chocoCommandRunner = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

var chocoPlatformChecker = coreutils.IsWindows

var askChocoRepositoryType = func() string {
	return ioutils.AskFromList("", "Select NuGet repository type to configure:", false,
		ioutils.ConvertToSuggests([]string{rtutils.Virtual.String(), rtutils.Local.String(), rtutils.Remote.String()}),
		rtutils.Virtual.String())
}

var chocoRepoClassResolver = func(serverDetails *config.ServerDetails, repoName string) (string, error) {
	servicesManager, err := rtutils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return "", fmt.Errorf("create services manager: %w", err)
	}
	var repoDetails services.RepositoryDetails
	if err = servicesManager.GetRepository(repoName, &repoDetails); err != nil {
		return "", fmt.Errorf("get repository %q: %w", repoName, err)
	}
	if repoDetails.PackageType != repository.Nuget {
		return "", errorutils.CheckErrorf("repository %q is package type %q; Chocolatey requires a NuGet repository", repoName, repoDetails.PackageType)
	}
	return repoDetails.GetRepoType(), nil
}

func chocoSourceDetails(serverDetails *config.ServerDetails, repoName string) (sourceURL, user, password string, err error) {
	if serverDetails == nil {
		return "", "", "", errorutils.CheckErrorf("server details are required to configure Chocolatey")
	}
	if repoName == "" {
		return "", "", "", errorutils.CheckErrorf("a repository name is required to configure Chocolatey")
	}
	sourceURL, user, password, err = dotnet.GetSourceDetails(serverDetails, repoName, true)
	if err != nil {
		return "", "", "", fmt.Errorf("get Chocolatey source details: %w", err)
	}
	if password == "" {
		return "", "", "", errorutils.CheckErrorf("credentials are required to configure Chocolatey authentication")
	}
	// Chocolatey talks to a NuGet feed over HTTP basic authentication for both reads and pushes, so
	// it needs a username as well as a secret. A username and password, a username and API key, and
	// a JWT access token (whose subject is the username) all satisfy that. A reference token does
	// not: it carries no subject, and jfrog-client-go's own guidance for that case is to supply a
	// username. Failing here beats adding a credential-less source that only breaks later, as an
	// unexplained 401 from the first "choco install".
	if user == "" {
		return "", "", "", errorutils.CheckErrorf("a username is required to configure Chocolatey. Chocolatey authenticates to Artifactory with basic authentication, and the configured access token carries no username. Re-run 'jf c add' with a username, or use a JWT access token")
	}
	return sourceURL, user, password, nil
}

// warnOnPlaintextChocoSource notes that the credentials about to be stored on the source will be
// sent in the clear. The source is still configured: "jf setup nuget" and "jf setup dotnet" both
// accept plain HTTP sources, and refusing here would put Chocolatey back in the outlier position
// this command was aligned away from. A loopback host stays quiet - nothing leaves the machine.
func warnOnPlaintextChocoSource(sourceURL string) {
	parsedURL, err := url.Parse(sourceURL)
	if err != nil || !strings.EqualFold(parsedURL.Scheme, "http") {
		return
	}
	if hostname := parsedURL.Hostname(); hostname == "localhost" || net.ParseIP(hostname).IsLoopback() {
		return
	}
	log.Warn("The Artifactory URL uses plain HTTP, so the credentials stored on the Chocolatey source and its API key will be transmitted unencrypted. Use HTTPS instead.")
}

func chocoSourceName(serverDetails *config.ServerDetails, repoName string) (string, error) {
	if serverDetails == nil {
		return "", errorutils.CheckErrorf("server details are required to configure Chocolatey")
	}
	parsedURL, err := url.Parse(serverDetails.ArtifactoryUrl)
	if err != nil || parsedURL.Hostname() == "" {
		return "", errorutils.CheckErrorf("a valid Artifactory URL is required to configure Chocolatey")
	}
	hostname := sanitizeChocoSourceComponent(parsedURL.Hostname())
	repositoryName := sanitizeChocoSourceComponent(repoName)
	if hostname == "" || repositoryName == "" {
		return "", errorutils.CheckErrorf("a valid Artifactory hostname and repository name are required to configure Chocolatey")
	}
	return "jfrt-" + hostname + "-" + repositoryName, nil
}

func sanitizeChocoSourceComponent(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			builder.WriteRune(character)
			continue
		}
		builder.WriteByte('-')
	}
	return strings.Trim(builder.String(), "-")
}

func normalizeChocoRepositoryType(repositoryType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(repositoryType)) {
	case rtutils.Virtual.String():
		return rtutils.Virtual.String(), nil
	case rtutils.Local.String():
		return rtutils.Local.String(), nil
	case rtutils.Remote.String():
		return rtutils.Remote.String(), nil
	default:
		return "", errorutils.CheckErrorf("Chocolatey repository type must be virtual, local, or remote")
	}
}

func (sc *SetupCommand) promptUserToSelectChocoRepository() error {
	repositoryType, err := normalizeChocoRepositoryType(askChocoRepositoryType())
	if err != nil {
		return err
	}
	return sc.promptUserToSelectRepositoryFiltered(repositoryType)
}

func ValidateChocoPlatform() error {
	if !chocoPlatformChecker() {
		return errorutils.CheckErrorf("'jf setup choco' requires Chocolatey, which runs on Windows only. Detected OS: %s", runtime.GOOS)
	}
	return nil
}

func (sc *SetupCommand) configureChoco() error {
	if err := ValidateChocoPlatform(); err != nil {
		return err
	}

	sourceURL, user, password, err := chocoSourceDetails(sc.serverDetails, sc.repoName)
	if err != nil {
		return err
	}
	sourceName, err := chocoSourceName(sc.serverDetails, sc.repoName)
	if err != nil {
		return err
	}
	repoClass, err := chocoRepoClassResolver(sc.serverDetails, sc.repoName)
	if err != nil {
		return err
	}

	warnOnPlaintextChocoSource(sourceURL)

	addArgs := []string{"source", "add", "-n=" + sourceName, "-s=" + sourceURL}
	// Chocolatey keeps two separate credential stores: the source's own -u/-p authenticates reads
	// (install, list, outdated), while the API key below is used only for pushes. Setting just one
	// of them is what made push succeed while every install returned HTTP 401.
	addArgs = append(addArgs, "-u="+user, "-p="+password)
	switch repoClass {
	case services.VirtualRepositoryRepoType, services.RemoteRepositoryRepoType:
		addArgs = append(addArgs, "--priority=1")
	case services.LocalRepositoryRepoType:
	default:
		return errorutils.CheckErrorf("repository %q must be a NuGet virtual, local, or remote repository; got %q", sc.repoName, repoClass)
	}

	// "source add" is an upsert keyed by name: Chocolatey updates the existing source's URL and
	// priority in place when the name matches, rather than requiring an add after removing the old
	// one. Removing first would leave every user on this machine without a working source for the
	// span between the two commands, and permanently so if the add or apikey step then failed.
	if err = chocoCommandRunner("choco", addArgs...); err != nil {
		return errorutils.CheckErrorf("failed to add the Artifactory source to Chocolatey. Ensure choco is installed and that this shell is elevated (Administrator)")
	}
	if err = chocoCommandRunner("choco", "apikey", "add", "-s="+sourceURL, "-k="+user+":"+password); err != nil {
		return errorutils.CheckErrorf("failed to store the Artifactory API key in Chocolatey for source %q. Ensure this shell is elevated (Administrator)", sourceName)
	}
	log.Output(fmt.Sprintf("Chocolatey source name: %s", sourceName))
	return nil
}
