package install

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/skills/commands/publish"
	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// scope is project (under project root) or global (agent home paths).
type scope string

const (
	scopeProject scope = "project"
	scopeGlobal  scope = "global"
)

// agentSkillInstallDir pairs an agent (or path-mode sentinel) with the absolute skill install directory (includes slug).
type agentSkillInstallDir struct {
	Agent          common.AgentSpec
	DestinationDir string
	Scope          string
}

// InstallCommand installs a skill for configured agents or legacy --path (update).
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agents        []common.AgentSpec
	scope         scope
	projectDir    string
	// installPath is the base directory for jf skills install --path. The skill is installed at
	// <installPath>/<slug> and takes precedence over --agent / --project-dir / --skills-global.
	installPath string
	format      string
	quiet       bool
	// explicitTargets, when set, overrides resolveAgentTargetDirectories (used by skills update).
	explicitTargets []common.AgentTarget
	suppressSummary bool
}

// FetchExtractInvocationCount counts completed FetchAndExtractTo calls (for tests).
var FetchExtractInvocationCount int

// ResetFetchExtractInvocationCount resets FetchExtractInvocationCount (for tests).
func ResetFetchExtractInvocationCount() {
	FetchExtractInvocationCount = 0
}

func NewInstallCommand() *InstallCommand {
	return &InstallCommand{scope: scopeProject}
}

func (ic *InstallCommand) SetServerDetails(details *config.ServerDetails) *InstallCommand {
	ic.serverDetails = details
	return ic
}

func (ic *InstallCommand) SetRepoKey(repoKey string) *InstallCommand {
	ic.repoKey = repoKey
	return ic
}

func (ic *InstallCommand) SetSlug(slug string) *InstallCommand {
	ic.slug = slug
	return ic
}

func (ic *InstallCommand) SetVersion(version string) *InstallCommand {
	ic.version = version
	return ic
}

func (ic *InstallCommand) SetAgents(agents []common.AgentSpec) *InstallCommand {
	ic.agents = agents
	return ic
}

// SetGlobal sets global vs project scope.
func (ic *InstallCommand) SetGlobal(isGlobal bool) *InstallCommand {
	if isGlobal {
		ic.scope = scopeGlobal
	} else {
		ic.scope = scopeProject
	}
	return ic
}

// SetProjectDir sets absolute project root for project scope.
func (ic *InstallCommand) SetProjectDir(projectRoot string) *InstallCommand {
	ic.projectDir = projectRoot
	return ic
}

func (ic *InstallCommand) SetQuiet(quiet bool) *InstallCommand {
	ic.quiet = quiet
	return ic
}

// SetFormat sets summary output: "table" (default) or "json".
func (ic *InstallCommand) SetFormat(format string) *InstallCommand {
	ic.format = format
	return ic
}

// SetInstallPath sets direct install base (same as skills update --path): skill at <base>/<slug>.
func (ic *InstallCommand) SetInstallPath(installPath string) *InstallCommand {
	ic.installPath = installPath
	return ic
}

// SetTargets overrides target resolution. Used by skills update for a filtered subset.
func (ic *InstallCommand) SetTargets(targets []common.AgentTarget) *InstallCommand {
	ic.explicitTargets = targets
	return ic
}

// SetSuppressSummary skips PrintSummary at the end of Run (caller prints a merged summary).
func (ic *InstallCommand) SetSuppressSummary(suppress bool) *InstallCommand {
	ic.suppressSummary = suppress
	return ic
}

func (ic *InstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return ic.serverDetails, nil
}

func (ic *InstallCommand) CommandName() string {
	return "skills_install"
}

func (ic *InstallCommand) Run() error {
	if ic.installPath == "" && len(ic.agents) == 0 && len(ic.explicitTargets) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	installTargets, err := ic.resolveAgentTargetDirectories()
	if err != nil {
		return err
	}

	resolvedVersion, err := common.ResolveSkillVersion(ic.serverDetails, ic.repoKey, ic.slug, ic.version, ic.quiet)
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if ic.installPath != "" {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' to %s", ic.slug, ic.version, installTargets[0].DestinationDir))
	} else {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' for %d agent(s)", ic.slug, ic.version, len(installTargets)))
	}

	tmpDir, err := os.MkdirTemp("", "skill-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	unzipDir, err := ic.FetchAndExtractTo(tmpDir)
	if err != nil {
		return err
	}

	results := ic.copyExtractedToTargets(unzipDir, installTargets)

	if !ic.suppressSummary {
		if err := PrintSummary(ic.slug, ic.version, results, ic.format); err != nil {
			return err
		}
	}

	for _, result := range results {
		if result.Status != SummaryStatusOK {
			if ic.suppressSummary {
				return fmt.Errorf("installation failed for one or more targets")
			}
			return fmt.Errorf("installation failed for one or more agents (see summary above)")
		}
	}
	return nil
}

// FetchAndExtractTo downloads the skill zip into tmpDir, extracts it, and runs evidence checks.
// The returned unzipDir is under tmpDir; callers must keep tmpDir until copies finish.
func (ic *InstallCommand) FetchAndExtractTo(tmpDir string) (unzipDir string, err error) {
	FetchExtractInvocationCount++

	zipPath, err := ic.downloadZip(tmpDir)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return "", ic.diagnoseDownloadForbidden(err)
		}
		return "", fmt.Errorf("download failed: %w", err)
	}

	unzipDir = filepath.Join(tmpDir, "contents")
	if err := unzipFile(zipPath, unzipDir); err != nil {
		return "", fmt.Errorf("unzip failed: %w", err)
	}

	if err := ic.handleEvidenceVerification(); err != nil {
		return "", err
	}
	return unzipDir, nil
}

// CopyExtractedToAgentTargets copies an unpacked skill tree to the given resolved targets.
func (ic *InstallCommand) CopyExtractedToAgentTargets(unzipDir string, targets []common.AgentTarget) []SummaryRow {
	return ic.copyExtractedToTargets(unzipDir, agentDirsFromTargets(targets))
}

func (ic *InstallCommand) copyExtractedToTargets(unzipDir string, installTargets []agentSkillInstallDir) []SummaryRow {
	results := make([]SummaryRow, 0, len(installTargets))
	for _, target := range installTargets {
		if err := ensureDestinationDir(target.DestinationDir); err != nil {
			results = append(results, SummaryRow{
				Agent:  target.Agent.Name,
				Scope:  target.Scope,
				Path:   target.DestinationDir,
				Status: SummaryStatusFailed,
				Detail: err.Error(),
			})
			continue
		}
		if err := copyDir(unzipDir, target.DestinationDir); err != nil {
			results = append(results, SummaryRow{
				Agent:  target.Agent.Name,
				Scope:  target.Scope,
				Path:   target.DestinationDir,
				Status: SummaryStatusFailed,
				Detail: err.Error(),
			})
			continue
		}
		results = append(results, SummaryRow{
			Agent:  target.Agent.Name,
			Scope:  target.Scope,
			Path:   target.DestinationDir,
			Status: SummaryStatusOK,
			Detail: SummaryDetailOKInstall,
		})
	}
	return results
}

func (ic *InstallCommand) handleEvidenceVerification() error {
	err := ic.verifyEvidence()
	if err == nil {
		return nil
	}
	if ic.quiet || common.IsNonInteractive() {
		if common.ShouldFailOnMissingEvidence() {
			return fmt.Errorf("evidence verification failed for skill '%s': %s. Set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true to proceed without evidence", ic.slug, err.Error())
		}
		log.Warn(fmt.Sprintf("Evidence verification failed for skill '%s': %s. Proceeding with installation.", ic.slug, err.Error()))
		return nil
	}
	log.Warn("Evidence verification failed:", err.Error())
	if !coreutils.AskYesNo("The skill is unattested. Continue with installation?", false) {
		return fmt.Errorf("installation aborted by user")
	}
	return nil
}

// resolveAgentTargetDirectories builds per-agent dest dirs, or one direct target if installPath is set (install/update --path).
func (ic *InstallCommand) resolveAgentTargetDirectories() ([]agentSkillInstallDir, error) {
	if len(ic.explicitTargets) > 0 {
		return agentDirsFromTargets(ic.explicitTargets), nil
	}
	if ic.installPath != "" {
		targets, err := common.ResolveAgentTargets(ic.slug, ic.installPath, nil, "", false)
		if err != nil {
			return nil, err
		}
		return agentDirsFromTargets(targets), nil
	}
	if ic.scope == scopeProject && ic.projectDir == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}
	isGlobal := ic.scope == scopeGlobal
	targets, err := common.ResolveAgentTargets(ic.slug, "", ic.agents, ic.projectDir, isGlobal)
	if err != nil {
		return nil, err
	}
	return agentDirsFromTargets(targets), nil
}

func agentDirsFromTargets(targets []common.AgentTarget) []agentSkillInstallDir {
	out := make([]agentSkillInstallDir, len(targets))
	for i, t := range targets {
		out[i] = agentSkillInstallDir{
			Agent:          t.Agent,
			DestinationDir: t.DestinationDir,
			Scope:          string(t.Scope),
		}
	}
	return out
}

func (ic *InstallCommand) downloadZip(tmpDir string) (string, error) {
	serviceManager, err := utils.CreateDownloadServiceManager(ic.serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return "", err
	}

	pattern := fmt.Sprintf("%s/%s/%s/%s-%s.zip", ic.repoKey, ic.slug, ic.version, ic.slug, ic.version)

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = pattern
	downloadParams.Target = tmpDir + "/"
	downloadParams.Flat = true

	totalDownloaded, totalFailed, err := serviceManager.DownloadFiles(downloadParams)
	if err != nil {
		return "", err
	}
	if totalFailed > 0 {
		return "", fmt.Errorf("download failed for %s", pattern)
	}
	if totalDownloaded == 0 {
		return "", fmt.Errorf("skill '%s' version '%s' not found in repository '%s'", ic.slug, ic.version, ic.repoKey)
	}

	zipName := fmt.Sprintf("%s-%s.zip", ic.slug, ic.version)
	zipPath := filepath.Join(tmpDir, zipName)
	return zipPath, nil
}

// diagnoseDownloadForbidden checks the Xray status API when a download returns 403.
// If the artifact is blocked by Xray, it returns a specific error message.
// Otherwise, it returns the original error.
func (ic *InstallCommand) diagnoseDownloadForbidden(originalErr error) error {
	artifactPath := fmt.Sprintf("%s/%s/%s-%s.zip", ic.slug, ic.version, ic.slug, ic.version)
	sm, err := utils.CreateServiceManager(ic.serverDetails, 3, 0, false)
	if err != nil {
		return fmt.Errorf("download blocked (403): %w", originalErr)
	}
	resp, err := sm.GetSkillXrayStatus(ic.repoKey, artifactPath)
	if err != nil {
		return fmt.Errorf("download blocked (403): %w", originalErr)
	}
	if resp.Status == services.SkillXrayStatusBlocked {
		return fmt.Errorf("skill '%s' v%s is blocked by Xray security scan. The artifact has security or license violations and cannot be downloaded", ic.slug, ic.version)
	}
	return fmt.Errorf("download failed: %w", originalErr)
}

func (ic *InstallCommand) verifyEvidence() error {
	if ic.repoKey == "" || ic.slug == "" || ic.version == "" {
		return fmt.Errorf("cannot verify evidence: repoKey, slug, and version must all be set")
	}

	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", ic.repoKey, ic.slug, ic.version, ic.slug, ic.version)

	return common.VerifyEvidence(ic.serverDetails, common.VerifyEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
	})
}

// RunInstall is the CLI action for `jf skills install`.
func RunInstall(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills install <slug> (--agent <name[,name...]> [--global] [--project-dir <dir>]] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := c.GetArgumentAt(0)
	if err := publish.ValidateSlug(slug); err != nil {
		return err
	}

	absoluteInstallBaseDir, specs, projectDirAbs, isGlobal, err := common.ValidateInstallFlags(c)
	if err != nil {
		return err
	}

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}
	quiet := common.IsQuiet(c)
	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	version := c.GetStringFlagValue("version")
	format := "table"
	if c.GetStringFlagValue("format") != "" {
		format = c.GetStringFlagValue("format")
	}
	if absoluteInstallBaseDir != "" {
		return NewInstallCommand().
			SetServerDetails(serverDetails).
			SetRepoKey(repoKey).
			SetSlug(slug).
			SetVersion(version).
			SetInstallPath(absoluteInstallBaseDir).
			SetFormat(format).
			SetQuiet(quiet).
			Run()
	}

	return NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(version).
		SetAgents(specs).
		SetGlobal(isGlobal).
		SetProjectDir(projectDirAbs).
		SetFormat(format).
		SetQuiet(quiet).
		Run()
}

// ensureDestinationDir mkdirs if missing, errors if path exists and is not a dir.
func ensureDestinationDir(dest string) error {
	info, err := os.Stat(dest)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("install destination %q exists and is not a directory", dest)
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// #nosec G301 -- skill files need to be readable across the user's tools.
		if mkErr := os.MkdirAll(dest, 0750); mkErr != nil {
			return fmt.Errorf(
				"failed to create install destination %q: %w. "+
					"Create the directory at that path (including parent folders if needed), then run the command again",
				dest, mkErr,
			)
		}
		return nil
	default:
		return fmt.Errorf("install destination %q is not accessible: %w", dest, err)
	}
}

func unzipFile(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = r.Close()
	}()

	if err := os.MkdirAll(dest, 0750); err != nil {
		return err
	}

	for _, f := range r.File {
		// #nosec G305 -- path traversal is checked immediately below
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		// #nosec G301 -- skill files need to be readable
		if err := os.MkdirAll(filepath.Dir(fpath), 0750); err != nil {
			return err
		}

		if err := extractFile(f, fpath); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, dest string) error {
	if strings.Contains(dest, "..") {
		return fmt.Errorf("illegal file path: %s", dest)
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	cleanDest := filepath.Clean(dest)
	// #nosec G304 -- dest is validated in unzipFile and above to be under the extraction directory
	outFile, err := os.OpenFile(cleanDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer func() {
		_ = outFile.Close()
	}()

	// #nosec G110 -- skill zip files are size-bounded by Artifactory upload limits
	_, err = io.Copy(outFile, rc)
	return err
}

func copyDir(src, dst string) error {
	// #nosec G301 -- skill files need to be readable
	if err := os.MkdirAll(dst, 0750); err != nil {
		return err
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	// #nosec G304 -- src comes from our own unzip temp directory
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()

	// #nosec G304 -- dst is constructed from validated unzip output path
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, in)
	return err
}
