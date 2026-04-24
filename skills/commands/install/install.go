package install

import (
	"archive/zip"
	"fmt"
	"io"
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

const defaultInstallBase = "."

type InstallCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	slug          string
	version       string
	installPath   string
	quiet         bool
}

func NewInstallCommand() *InstallCommand {
	return &InstallCommand{}
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

func (ic *InstallCommand) SetInstallPath(path string) *InstallCommand {
	ic.installPath = path
	return ic
}

func (ic *InstallCommand) SetQuiet(quiet bool) *InstallCommand {
	ic.quiet = quiet
	return ic
}

func (ic *InstallCommand) ServerDetails() (*config.ServerDetails, error) {
	return ic.serverDetails, nil
}

func (ic *InstallCommand) CommandName() string {
	return "skills_install"
}

func (ic *InstallCommand) Run() error {
	version, err := ic.resolveVersion()
	if err != nil {
		return err
	}
	ic.version = version

	log.Info(fmt.Sprintf("Installing skill '%s' version '%s'", ic.slug, ic.version))

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

	destDir := ic.getDestDir()
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("failed to remove existing skill directory: %w", err)
	}
	if err := copyDir(unzipDir, destDir); err != nil {
		return fmt.Errorf("failed to copy skill files: %w", err)
	}

	log.Info(fmt.Sprintf("Skill '%s' version '%s' installed to %s", ic.slug, ic.version, destDir))
	return nil
}

func (ic *InstallCommand) resolveVersion() (string, error) {
	if ic.version == "latest" || ic.version == "" {
		versions, err := common.ListVersions(ic.serverDetails, ic.repoKey, ic.slug)
		if err != nil {
			if strings.Contains(err.Error(), "404 Not Found") {
				return "", fmt.Errorf("skill '%s' not found in repository '%s'", ic.slug, ic.repoKey)
			}
			if ic.version == "" {
				return "", fmt.Errorf("failed to list versions (provide --version explicitly): %w", err)
			}
			return "", fmt.Errorf("failed to list versions: %w", err)
		}

		versionStrs := make([]string, len(versions))
		for i, v := range versions {
			versionStrs[i] = v.Version
		}

		if ic.version == "latest" {
			return common.LatestVersion(versionStrs)
		}

		if ic.quiet || common.IsNonInteractive() {
			return "", fmt.Errorf("--version is required in non-interactive mode (use semver or \"latest\")")
		}

		latest, err := common.LatestVersion(versionStrs)
		if err != nil {
			return "", err
		}
		log.Info("Available versions:", versionStrs)
		log.Info("Using latest version:", latest)
		return latest, nil
	}

	return ic.version, nil
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

func (ic *InstallCommand) getDestDir() string {
	base := ic.installPath
	if base == "" {
		base = defaultInstallBase
	}
	return filepath.Join(base, ic.slug)
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
		return fmt.Errorf("usage: jf skills install <slug> [--repo <repo>] [options]")
	}

	slug := c.GetArgumentAt(0)

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	quiet := common.IsQuiet(c)
	repoKey, err := common.ResolveRepo(serverDetails, c.GetStringFlagValue("repo"), quiet)
	if err != nil {
		return err
	}

	cmd := NewInstallCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSlug(slug).
		SetVersion(c.GetStringFlagValue("version")).
		SetInstallPath(c.GetStringFlagValue("path")).
		SetQuiet(quiet)

	return cmd.Run()
}
