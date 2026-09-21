package setup

import (
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/dotnet"
	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/repository"
	rtutils "github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// psresourceModuleCheckScript is run through pwsh/powershell.exe to determine whether the
// Microsoft.PowerShell.PSResourceGet module is installed. Get-Module -ListAvailable never throws
// for a missing module, so the exit code is the only signal this needs to hand back.
const psresourceModuleCheckScript = "if (Get-Module -ListAvailable -Name Microsoft.PowerShell.PSResourceGet) { exit 0 } else { exit 1 }"

// psresourceCommandRunner shells out to the resolved PowerShell executable. It is a var, like
// chocoCommandRunner, so tests can replace it with a stub instead of invoking pwsh for real.
// Stderr is captured (not discarded) and folded into the returned error, so a real
// Register-PSResourceRepository failure (bad URL, name collision, permissions) is diagnosable
// instead of always looking like a missing pwsh/module installation.
var psresourceCommandRunner = func(name string, args ...string) error {
	var stderr strings.Builder
	cmd := exec.Command(name, args...) // #nosec G204 -- name/args are this package's own constructed pwsh invocation, never external input
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

// psresourceShellResolver reports the PowerShell executable to use for PSResourceGet cmdlets: pwsh
// (PowerShell 7+) if it is on PATH, otherwise powershell.exe on Windows (Windows PowerShell 5.1,
// which also carries the PSResourceGet module on recent Windows builds). PSResourceGet through pwsh
// is cross-platform, which is exactly why - unlike Chocolatey - platform support here is never
// gated on runtime.GOOS; the Windows-only fallback exists only because powershell.exe itself never
// existed anywhere else.
//
// It is a var so tests can stub it without touching the real PATH.
var psresourceShellResolver = func() (shell string, found bool) {
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "pwsh", true
	}
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell.exe", true
		}
	}
	return "", false
}

// psresourcePlatformChecker reports whether this machine can run 'jf Install-PSResource' /
// 'jf Publish-PSResource': a PowerShell executable on PATH (see psresourceShellResolver) AND the
// Microsoft.PowerShell.PSResourceGet module available to it. It is a var, like
// chocoPlatformChecker, so tests can stub it directly instead of driving the two lower-level vars.
var psresourcePlatformChecker = func() bool {
	shell, found := psresourceShellResolver()
	if !found {
		return false
	}
	return psresourceCommandRunner(shell, "-NoProfile", "-Command", psresourceModuleCheckScript) == nil
}

// psresourcePlatformErr is the single error message used whenever no usable PowerShell +
// PSResourceGet installation can be found, whether the caller asked via ValidatePSResourcePlatform
// or PSResourceShell.
func psresourcePlatformErr() error {
	return errorutils.CheckErrorf("'jf Install-PSResource'/'jf Publish-PSResource' requires PowerShell 7+ (pwsh) with the Microsoft.PowerShell.PSResourceGet module installed")
}

// ValidatePSResourcePlatform returns a clear error when this machine cannot run PSResourceGet
// commands. Call it before doing anything else, so a missing pwsh/module costs nothing.
func ValidatePSResourcePlatform() error {
	if !psresourcePlatformChecker() {
		return psresourcePlatformErr()
	}
	return nil
}

// PSResourceShell returns the PowerShell executable this environment uses to run PSResourceGet
// cmdlets - the exact same binary psresourcePlatformChecker validated. Callers outside this
// package (the psresource FlexPack command) use this instead of re-detecting pwsh vs
// powershell.exe on their own, so the two layers can never disagree about which shell is in use.
// Call ValidatePSResourcePlatform first; this does not re-check that the module is installed.
func PSResourceShell() (string, error) {
	shell, found := psresourceShellResolver()
	if !found {
		return "", psresourcePlatformErr()
	}
	return shell, nil
}

// psresourceRepoClassResolver mirrors chocoRepoClassResolver: Artifactory has no distinct
// PowerShell package type, so a PSResourceGet feed is backed by a NuGet repository - virtual,
// local, or remote are all acceptable. It is a var for testability.
var psresourceRepoClassResolver = func(serverDetails *config.ServerDetails, repoName string) (string, error) {
	servicesManager, err := rtutils.CreateServiceManager(serverDetails, -1, 0, false)
	if err != nil {
		return "", fmt.Errorf("create services manager: %w", err)
	}
	var repoDetails services.RepositoryDetails
	if err = servicesManager.GetRepository(repoName, &repoDetails); err != nil {
		return "", fmt.Errorf("get repository %q: %w", repoName, err)
	}
	if repoDetails.PackageType != repository.Nuget {
		return "", errorutils.CheckErrorf("repository %q is package type %q; PSResourceGet requires a NuGet repository", repoName, repoDetails.PackageType)
	}
	return repoDetails.GetRepoType(), nil
}

// psresourceSourceName mirrors chocoSourceName's naming scheme so the two package managers'
// registered source names follow one convention across the CLI.
func psresourceSourceName(serverDetails *config.ServerDetails, repoName string) (string, error) {
	if serverDetails == nil {
		return "", errorutils.CheckErrorf("server details are required to configure PSResourceGet")
	}
	parsedURL, err := url.Parse(serverDetails.ArtifactoryUrl)
	if err != nil || parsedURL.Hostname() == "" {
		return "", errorutils.CheckErrorf("a valid Artifactory URL is required to configure PSResourceGet")
	}
	hostname := sanitizePSResourceSourceComponent(parsedURL.Hostname())
	repositoryName := sanitizePSResourceSourceComponent(repoName)
	if hostname == "" || repositoryName == "" {
		return "", errorutils.CheckErrorf("a valid Artifactory hostname and repository name are required to configure PSResourceGet")
	}
	return "jfrt-" + hostname + "-" + repositoryName, nil
}

// sanitizePSResourceSourceComponent restricts a source-name component to characters
// Register-PSResourceRepository accepts unquoted-safe: lowercase letters, digits, '.', '_', '-'.
// Everything else becomes '-', and leading/trailing '-' are trimmed.
func sanitizePSResourceSourceComponent(value string) string {
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

// QuotePSLiteral renders value as a single-quoted PowerShell string literal, doubling any embedded
// single quote the way PowerShell itself escapes one. This is the single implementation of that
// rule shared by this package and artifactory/commands/psresource (which already imports this
// package for ValidatePSResourcePlatform/PSResourceShell, so it calls this rather than the reverse -
// that direction would create an import cycle).
func QuotePSLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// configurePSResource registers the target Artifactory repository as a trusted PSResourceGet
// source. It deliberately does not persist credentials at registration time - see the comment
// above the Register-PSResourceRepository call.
func (sc *SetupCommand) configurePSResource() error {
	// Re-checked here (Run() already checks it before dispatch) so configurePSResource stays safe
	// to call on its own - e.g. in a unit test that exercises this function directly.
	if err := ValidatePSResourcePlatform(); err != nil {
		return err
	}
	if _, err := psresourceRepoClassResolver(sc.serverDetails, sc.repoName); err != nil {
		return err
	}
	// useNugetV2=false: PSResourceGet needs the NuGet v3 feed (".../api/nuget/v3/<repo>/index.json"),
	// unlike Chocolatey, which needs v2. dotnet.GetSourceDetails already produces exactly that shape.
	sourceURL, _, _, err := dotnet.GetSourceDetails(sc.serverDetails, sc.repoName, false)
	if err != nil {
		return fmt.Errorf("get PSResource source details: %w", err)
	}
	sourceName, err := psresourceSourceName(sc.serverDetails, sc.repoName)
	if err != nil {
		return err
	}
	shell, err := PSResourceShell()
	if err != nil {
		return err
	}

	// Unregister any previous registration under this name first. -ErrorAction SilentlyContinue
	// makes PSResourceGet's non-terminating "repository not found" error a no-op instead of a
	// failure, so a first-time `jf setup psresource` (nothing registered yet) never fails here -
	// there is no separate "not found" exit code to special-case, unlike choco's source remove.
	unregisterScript := fmt.Sprintf("Unregister-PSResourceRepository -Name %s -ErrorAction SilentlyContinue",
		QuotePSLiteral(sourceName))
	if unregisterErr := psresourceCommandRunner(shell, "-NoProfile", "-Command", unregisterScript); unregisterErr != nil {
		log.Debug(fmt.Sprintf("Unregister-PSResourceRepository for %q returned an error (ignored): %s", sourceName, unregisterErr.Error()))
	}

	// NOTE: this deliberately does NOT persist credentials on the registered source - no
	// -CredentialInfo, no SecretManagement-backed vault entry. This project's design is for
	// 'jf Install-PSResource' / 'jf Save-PSResource' / 'jf Update-PSResource' / 'jf Publish-PSResource'
	// (artifactory/commands/psresource) to inject -Credential on every native invocation instead.
	// Do not "fix" this later by adding credential persistence here; it was left out on purpose.
	registerScript := fmt.Sprintf("Register-PSResourceRepository -Name %s -Uri %s -Trusted",
		QuotePSLiteral(sourceName), QuotePSLiteral(sourceURL))
	if err = psresourceCommandRunner(shell, "-NoProfile", "-Command", registerScript); err != nil {
		return errorutils.CheckErrorf("failed to register the Artifactory source with PSResourceGet: %s", err.Error())
	}

	log.Output(fmt.Sprintf("PSResource source name: %s", sourceName))
	return nil
}
