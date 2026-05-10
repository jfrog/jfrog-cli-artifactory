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

// agentTarget is one agent plus absolute destination dir (includes slug).
type agentTarget struct {
	Agent   common.AgentSpec
	DestDir string // absolute; ends with /<slug>
}

// InstallCommand installs a skill for configured agents or legacy --path (update).
type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	agents        []common.AgentSpec
	scope         scope
	projectDir    string // absolute project root for project scope
	// installPath: legacy base dir (e.g. skills update --path); wins over agents.
	installPath string
	quiet       bool
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
func (ic *InstallCommand) SetGlobal(useGlobal bool) *InstallCommand {
	if useGlobal {
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

// SetInstallPath sets direct install base (same as skills update --path): skill at <base>/<slug>.
func (ic *InstallCommand) SetInstallPath(installPath string) *InstallCommand {
	ic.installPath = installPath
	return ic
}

func (ic *InstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return ic.serverDetails, nil
}

func (ic *InstallCommand) CommandName() string {
	return "skills_install"
}

func (ic *InstallCommand) Run() error {
	if ic.installPath == "" && len(ic.agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	targets, err := ic.resolveTargets()
	if err != nil {
		return err
	}

	resolvedVersion, err := ic.resolveVersion()
	if err != nil {
		return err
	}
	ic.version = resolvedVersion

	if ic.installPath != "" {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' to %s", ic.slug, ic.version, targets[0].DestDir))
	} else {
		log.Info(fmt.Sprintf("Installing skill '%s' version '%s' for %d agent(s)", ic.slug, ic.version, len(targets)))
	}

	tmpDir, err := os.MkdirTemp("", "skill-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	zipPath, err := ic.downloadZip(tmpDir)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return ic.diagnoseDownloadForbidden(err)
		}
		return fmt.Errorf("download failed: %w", err)
	}

	unzipDir := filepath.Join(tmpDir, "contents")
	if err := unzipFile(zipPath, unzipDir); err != nil {
		return fmt.Errorf("unzip failed: %w", err)
	}

	if err := ic.verifyEvidence(); err != nil {
		if ic.quiet || common.IsNonInteractive() {
			if common.ShouldFailOnMissingEvidence() {
				return fmt.Errorf("evidence verification failed for skill '%s': %s. Set JFROG_SKILLS_DISABLE_QUIET_FAILURE=true to proceed without evidence", ic.slug, err.Error())
			}
			log.Warn(fmt.Sprintf("Evidence verification failed for skill '%s': %s. Proceeding with installation.", ic.slug, err.Error()))
		} else {
			log.Warn("Evidence verification failed:", err.Error())
			if !coreutils.AskYesNo("The skill is unattested. Continue with installation?", false) {
				return fmt.Errorf("installation aborted by user")
			}
		}
	}

	results := make([]installResult, 0, len(targets))
	for _, target := range targets {
		if err := ensureDestDir(target.DestDir); err != nil {
			results = append(results, installResult{Agent: target.Agent.Name, Scope: string(ic.scope), Path: target.DestDir, Status: skillInstallStatusFailed, Detail: err.Error()})
			continue
		}
		if err := copyDir(unzipDir, target.DestDir); err != nil {
			results = append(results, installResult{Agent: target.Agent.Name, Scope: string(ic.scope), Path: target.DestDir, Status: skillInstallStatusFailed, Detail: err.Error()})
			continue
		}
		results = append(results, installResult{Agent: target.Agent.Name, Scope: string(ic.scope), Path: target.DestDir, Status: skillInstallStatusOK, Detail: skillInstallDetailOK})
	}

	printSummary(ic.slug, ic.version, results)

	for _, result := range results {
		if result.Status != skillInstallStatusOK {
			return fmt.Errorf("installation failed for one or more agents (see summary above)")
		}
	}
	return nil
}

// resolveTargets builds per-agent dest dirs, or one direct target if installPath is set (install/update --path).
func (ic *InstallCommand) resolveTargets() ([]agentTarget, error) {
	if ic.installPath != "" {
		base, err := filepath.Abs(ic.installPath)
		if err != nil {
			return nil, fmt.Errorf("invalid install path %q: %w", ic.installPath, err)
		}
		return []agentTarget{{
			Agent:   common.AgentSpec{Name: "(path)"},
			DestDir: filepath.Join(base, ic.slug),
		}}, nil
	}
	if ic.scope == scopeProject && ic.projectDir == "" {
		return nil, fmt.Errorf("project directory is required for project-scoped install")
	}
	targets := make([]agentTarget, 0, len(ic.agents))
	for _, agentSpec := range ic.agents {
		base, err := common.ResolveAgentInstallDir(agentSpec, ic.projectDir, ic.scope == scopeGlobal)
		if err != nil {
			return nil, err
		}
		targets = append(targets, agentTarget{
			Agent:   agentSpec,
			DestDir: filepath.Join(base, ic.slug),
		})
	}
	return targets, nil
}

func (ic *InstallCommand) resolveVersion() (string, error) {
	return common.ResolveSkillVersion(ic.serverDetails, ic.repoKey, ic.slug, ic.version, ic.quiet)
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

// ensureDestDir mkdirs if missing, errors if path exists and is not a dir.
func ensureDestDir(dest string) error {
	info, err := os.Stat(dest)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("install destination %q exists and is not a directory", dest)
	case err == nil:
		return nil
	case errors.Is(err, fs.ErrNotExist):
		// #nosec G301 -- skill files need to be readable across the user's tools.
		if mkErr := os.MkdirAll(dest, 0750); mkErr != nil {
			return fmt.Errorf("failed to create install destination %q: %w", dest, mkErr)
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
	// Reject paths containing traversal sequences as defense-in-depth
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

// RunInstall is the CLI action for `jf skills install`.
func RunInstall(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills install <slug> (--agent <name[,name...]> [--global] [--project-dir <dir>]] | --path <dir>) [--repo <repo>] [--version <ver>]")
	}

	slug := c.GetArgumentAt(0)

	directBase := strings.TrimSpace(c.GetStringFlagValue("path"))
	rawAgents := strings.TrimSpace(c.GetStringFlagValue("agent"))
	useGlobal := c.GetBoolFlagValue("global")
	projectDirFlag := strings.TrimSpace(c.GetStringFlagValue("project-dir"))

	var pathInstallAbs string
	var specs []common.AgentSpec
	var projectDirAbs string

	if directBase != "" {
		if rawAgents != "" {
			return fmt.Errorf("--path cannot be combined with --agent")
		}
		if useGlobal {
			return fmt.Errorf("--path cannot be combined with --global")
		}
		if projectDirFlag != "" {
			return fmt.Errorf("--path cannot be combined with --project-dir")
		}
		if err := common.ValidateExistingDir(directBase); err != nil {
			return fmt.Errorf("--path: %w", err)
		}
		absBase, err := filepath.Abs(directBase)
		if err != nil {
			return fmt.Errorf("invalid --path %q: %w", directBase, err)
		}
		pathInstallAbs = absBase
	} else {
		registry, err := common.LoadAgentRegistry()
		if err != nil {
			return err
		}
		if rawAgents == "" {
			return fmt.Errorf("--agent is required unless --path is set. Supported agents: %s", common.AgentNames(registry))
		}

		agentNames, err := common.ParseAgentList(rawAgents)
		if err != nil {
			return err
		}

		specs = make([]common.AgentSpec, 0, len(agentNames))
		for _, name := range agentNames {
			spec, err := common.ResolveAgent(registry, name)
			if err != nil {
				return err
			}
			specs = append(specs, spec)
		}

		if useGlobal && projectDirFlag != "" {
			return fmt.Errorf("--global and --project-dir are mutually exclusive")
		}

		if !useGlobal {
			dir := projectDirFlag
			if dir == "" {
				dir = "."
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("invalid --project-dir %q: %w", dir, err)
			}
			info, statErr := os.Stat(abs)
			if statErr != nil || !info.IsDir() {
				return fmt.Errorf("--project-dir %q is not an existing directory", dir)
			}
			projectDirAbs = abs
		}
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
	if pathInstallAbs != "" {
		return NewInstallCommand().
			SetServerDetails(serverDetails).
			SetRepoKey(repoKey).
			SetSlug(slug).
			SetVersion(version).
			SetInstallPath(pathInstallAbs).
			SetQuiet(quiet).
			Run()
	}

	return NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(version).
		SetAgents(specs).
		SetGlobal(useGlobal).
		SetProjectDir(projectDirAbs).
		SetQuiet(quiet).
		Run()
}
