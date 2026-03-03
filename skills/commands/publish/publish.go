package publish

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jfrog/jfrog-cli-artifactory/skills/common"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/plugins/components"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	serviceutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

var zipExcludes = map[string]bool{
	".git":         true,
	"__pycache__":  true,
	"node_modules": true,
	".DS_Store":    true,
}

type PublishCommand struct {
	serverDetails *config.ServerDetails
	repoKey       string
	skillDir      string
	version       string
	quiet         bool
}

func NewPublishCommand() *PublishCommand {
	return &PublishCommand{}
}

func (pc *PublishCommand) SetServerDetails(details *config.ServerDetails) *PublishCommand {
	pc.serverDetails = details
	return pc
}

func (pc *PublishCommand) SetRepoKey(repoKey string) *PublishCommand {
	pc.repoKey = repoKey
	return pc
}

func (pc *PublishCommand) SetSkillDir(dir string) *PublishCommand {
	pc.skillDir = dir
	return pc
}

func (pc *PublishCommand) SetVersion(version string) *PublishCommand {
	pc.version = version
	return pc
}

func (pc *PublishCommand) SetQuiet(quiet bool) *PublishCommand {
	pc.quiet = quiet
	return pc
}

func (pc *PublishCommand) ServerDetails() (*config.ServerDetails, error) {
	return pc.serverDetails, nil
}

func (pc *PublishCommand) CommandName() string {
	return "skills_publish"
}

func (pc *PublishCommand) Run() error {
	meta, err := ParseSkillMeta(pc.skillDir)
	if err != nil {
		return err
	}

	slug := meta.Name
	if err := ValidateSlug(slug); err != nil {
		return err
	}

	version := pc.version
	if version == "" {
		version = meta.Version
	}
	if version == "" {
		return fmt.Errorf("no version specified. Provide --version flag or set 'version' in SKILL.md frontmatter")
	}

	log.Info(fmt.Sprintf("Publishing skill '%s' version '%s'", slug, version))

	exists, err := common.VersionExists(pc.serverDetails, pc.repoKey, slug, version)
	if err != nil {
		log.Debug("Could not check version existence:", err.Error())
	}
	if exists && !pc.quiet {
		log.Warn(fmt.Sprintf("Version %s of skill '%s' already exists and will be overridden.", version, slug))
	}

	zipPath, err := pc.resolveZip(slug, version)
	if err != nil {
		return err
	}
	defer func() {
		if !isPrebuiltZip(pc.skillDir, slug, version) {
			_ = os.Remove(zipPath)
		}
	}()

	sha256Hex, err := computeSHA256(zipPath)
	if err != nil {
		return fmt.Errorf("failed to compute SHA256: %w", err)
	}

	targetProps := fmt.Sprintf("skill.name=%s;skill.description=%s;skill.version=%s",
		slug, escapePropertyValue(meta.Description), version)

	target := fmt.Sprintf("%s/%s/%s/", pc.repoKey, slug, version)
	if err := pc.upload(zipPath, target, targetProps); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	log.Info("Upload complete. Attaching evidence...")
	pc.attachEvidence(slug, version, sha256Hex)

	log.Info(fmt.Sprintf("Skill '%s' version '%s' published successfully.", slug, version))
	return nil
}

func (pc *PublishCommand) resolveZip(slug, version string) (string, error) {
	prebuilt := filepath.Join(pc.skillDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version))
	if _, err := os.Stat(prebuilt); err == nil {
		log.Info("Using pre-built zip:", prebuilt)
		return prebuilt, nil
	}

	return zipSkillFolder(pc.skillDir, slug, version)
}

func isPrebuiltZip(skillDir, slug, version string) bool {
	prebuilt := filepath.Join(skillDir, "zip", fmt.Sprintf("%s_%s.zip", slug, version))
	_, err := os.Stat(prebuilt)
	return err == nil
}

func zipSkillFolder(skillDir, slug, version string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "skill-publish-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s.zip", slug, version))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer func() {
		_ = zipFile.Close()
	}()

	w := zip.NewWriter(zipFile)
	defer func() {
		_ = w.Close()
	}()

	err = filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(skillDir, path)
		if err != nil {
			return err
		}

		if shouldExclude(relPath, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		// #nosec G304 -- Path is constructed from user-provided skill directory
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			_ = file.Close()
		}()

		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		return "", fmt.Errorf("failed to zip skill folder: %w", err)
	}

	return zipPath, nil
}

func shouldExclude(relPath string, info os.FileInfo) bool {
	name := info.Name()

	if zipExcludes[name] {
		return true
	}
	if strings.HasSuffix(name, ".pyc") {
		return true
	}
	if relPath == "." {
		return false
	}
	return false
}

func computeSHA256(path string) (string, error) {
	// #nosec G304 -- Path is the zip we just created
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (pc *PublishCommand) upload(zipPath, target, targetProps string) error {
	serviceManager, err := utils.CreateUploadServiceManager(pc.serverDetails, 1, 3, 0, false, nil)
	if err != nil {
		return err
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = zipPath
	uploadParams.Target = target
	uploadParams.Flat = true
	props := serviceutils.NewProperties()
	for _, prop := range strings.Split(targetProps, ";") {
		parts := strings.SplitN(prop, "=", 2)
		if len(parts) == 2 {
			props.AddProperty(parts[0], parts[1])
		}
	}
	uploadParams.TargetProps = props

	_, _, err = serviceManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	return err
}

func (pc *PublishCommand) attachEvidence(slug, version, sha256Hex string) {
	keyPath := os.Getenv("EVD_SIGNING_KEY_PATH")
	if keyPath == "" {
		keyPath = os.Getenv("JFROG_CLI_SIGNING_KEY")
	}
	keyAlias := os.Getenv("EVD_KEY_ALIAS")

	if keyPath == "" {
		log.Info("No signing key configured (EVD_SIGNING_KEY_PATH or JFROG_CLI_SIGNING_KEY). Skipping evidence creation.")
		return
	}

	tmpDir, err := os.MkdirTemp("", "skill-evidence-*")
	if err != nil {
		log.Warn("Failed to create temp dir for evidence:", err.Error())
		return
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	predicatePath, err := GeneratePredicateFile(tmpDir, slug, version)
	if err != nil {
		log.Warn("Failed to generate predicate:", err.Error())
		return
	}

	markdownPath, err := GenerateMarkdownFile(tmpDir, slug, version)
	if err != nil {
		log.Warn("Failed to generate attestation markdown:", err.Error())
		return
	}

	subjectRepoPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", pc.repoKey, slug, version, slug, version)

	err = common.CreateEvidence(pc.serverDetails, common.CreateEvidenceOpts{
		SubjectRepoPath: subjectRepoPath,
		SubjectSHA256:   sha256Hex,
		PredicatePath:   predicatePath,
		PredicateType:   predicateTypePublishAttestation,
		MarkdownPath:    markdownPath,
		KeyPath:         keyPath,
		KeyAlias:        keyAlias,
	})
	if err != nil {
		log.Warn("Evidence creation failed (skill upload succeeded):", err.Error())
		return
	}

	log.Info("Evidence successfully attached.")
}

func escapePropertyValue(val string) string {
	val = strings.ReplaceAll(val, ";", "\\;")
	val = strings.ReplaceAll(val, "=", "\\=")
	return val
}

// RunPublish is the CLI action for `jf skills publish`.
func RunPublish(c *components.Context) error {
	if c.GetNumberOfArgs() < 1 {
		return fmt.Errorf("usage: jf skills publish <path-to-skill-folder> [options]")
	}

	skillDir := c.GetArgumentAt(0)
	absDir, err := filepath.Abs(skillDir)
	if err != nil {
		return fmt.Errorf("invalid skill path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("skill path '%s' is not a valid directory", skillDir)
	}

	serverDetails, err := common.GetServerDetails(c)
	if err != nil {
		return err
	}

	repoKey, err := common.ResolveRepo(c, serverDetails)
	if err != nil {
		return err
	}

	cmd := NewPublishCommand().
		SetServerDetails(serverDetails).
		SetRepoKey(repoKey).
		SetSkillDir(absDir).
		SetVersion(c.GetStringFlagValue("version")).
		SetQuiet(common.IsQuiet(c))

	return cmd.Run()
}
