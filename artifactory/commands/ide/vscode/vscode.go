package vscode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jfrog/jfrog-cli-artifactory/artifactory/commands/ide"
	"github.com/jfrog/jfrog-cli-core/v2/artifactory/utils"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// VscodeCommand represents the VSCode configuration command
type VscodeCommand struct {
	serviceURL    string
	productPath   string
	backupPath    string
	serverDetails *config.ServerDetails
	repoKey       string
	isDirectURL   bool // true if URL was provided directly, false if constructed from server-id + repo-key
}

// NewVscodeCommand creates a new VSCode configuration command
func NewVscodeCommand(repoKey, productPath, serviceURL string) *VscodeCommand {
	return &VscodeCommand{
		repoKey:     repoKey,
		productPath: productPath,
		serviceURL:  serviceURL,
		isDirectURL: false, // default to false, will be set explicitly
	}
}

func (vc *VscodeCommand) SetServerDetails(serverDetails *config.ServerDetails) *VscodeCommand {
	vc.serverDetails = serverDetails
	return vc
}

func (vc *VscodeCommand) ServerDetails() (*config.ServerDetails, error) {
	return vc.serverDetails, nil
}

func (vc *VscodeCommand) CommandName() string {
	return "rt_vscode_config"
}

// SetDirectURL marks this command as using a direct URL (skip validation)
func (vc *VscodeCommand) SetDirectURL(isDirect bool) *VscodeCommand {
	vc.isDirectURL = isDirect
	return vc
}

// Run executes the VSCode configuration command
func (vc *VscodeCommand) Run() error {
	log.Info("Configuring VSCode extensions repository...")

	// Only validate repository if we have server details and repo key AND it's not a direct URL
	// Skip validation when using direct service URL since no server-id is involved
	if vc.serverDetails != nil && vc.repoKey != "" && !vc.isDirectURL {
		if err := vc.validateRepository(); err != nil {
			return errorutils.CheckError(fmt.Errorf("repository validation failed: %w", err))
		}
	} else if vc.isDirectURL {
		log.Debug("Direct service URL provided, skipping repository validation")
	}

	if vc.productPath == "" {
		detectedPath, err := vc.detectVSCodeInstallation()
		if err != nil {
			return errorutils.CheckError(fmt.Errorf("failed to auto-detect VSCode installation: %w\n\nManual setup instructions:\n%s", err, vc.getManualSetupInstructions(vc.serviceURL)))
		}
		vc.productPath = detectedPath
		log.Info("Detected VSCode at:", vc.productPath)
	}

	if err := vc.modifyProductJson(vc.serviceURL); err != nil {
		if restoreErr := vc.restoreBackup(); restoreErr != nil {
			log.Error("Failed to restore backup:", restoreErr)
		}
		return errorutils.CheckError(fmt.Errorf("failed to modify product.json: %w\n\nManual setup instructions:\n%s", err, vc.getManualSetupInstructions(vc.serviceURL)))
	}

	log.Info("VSCode configuration updated successfully. Repository URL:", vc.serviceURL, "- Please restart VSCode to apply changes")
	return nil
}

// validateRepository uses the established pattern for repository validation
func (vc *VscodeCommand) validateRepository() error {
	log.Debug("Validating repository...")

	if vc.serverDetails == nil {
		return fmt.Errorf("server details not configured - please run 'jf config add' first")
	}

	artDetails, err := vc.serverDetails.CreateArtAuthConfig()
	if err != nil {
		return fmt.Errorf("failed to create auth config: %w", err)
	}

	if err := utils.ValidateRepoExists(vc.repoKey, artDetails); err != nil {
		return fmt.Errorf("repository validation failed: %w", err)
	}

	log.Info("Repository validation successful")
	return nil
}

// checkWritePermissions checks if we have write permissions to the product.json file
func (vc *VscodeCommand) checkWritePermissions() error {
	// Check if file exists and we can read it
	info, err := os.Stat(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to access product.json: %w", err)
	}

	if runtime.GOOS != "windows" {
		if os.Getuid() == 0 {
			return nil
		}
	}

	file, err := os.OpenFile(vc.productPath, os.O_WRONLY|os.O_APPEND, info.Mode())
	if err != nil {
		if os.IsPermission(err) {
			return vc.handlePermissionError()
		}
		return fmt.Errorf("failed to check write permissions: %w", err)
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("failed to close file: %w", closeErr)
	}
	return nil
}

// handlePermissionError provides appropriate guidance based on the operating system
func (vc *VscodeCommand) handlePermissionError() error {
	if runtime.GOOS == "darwin" && strings.HasPrefix(vc.productPath, "/Applications/") {
		userInfo := "the current user"
		if user := os.Getenv("USER"); user != "" {
			userInfo = user
		}
		return fmt.Errorf(ide.VscodeMacOSPermissionError, vc.serviceURL, vc.productPath, userInfo)
	}
	return fmt.Errorf(ide.VscodeGenericPermissionError, vc.serviceURL)
}

// detectVSCodeInstallation attempts to auto-detect VSCode installation
func (vc *VscodeCommand) detectVSCodeInstallation() (string, error) {
	var possiblePaths []string

	switch runtime.GOOS {
	case "darwin":
		possiblePaths = []string{
			"/Applications/Visual Studio Code.app/Contents/Resources/app/product.json",
			"/Applications/Visual Studio Code - Insiders.app/Contents/Resources/app/product.json",
			// Add user-installed locations
			filepath.Join(os.Getenv("HOME"), "Applications", "Visual Studio Code.app", "Contents", "Resources", "app", "product.json"),
		}
	case "windows":
		possiblePaths = []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "resources", "app", "product.json"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Microsoft VS Code", "resources", "app", "product.json"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Microsoft VS Code", "resources", "app", "product.json"),
		}
	case "linux":
		possiblePaths = []string{
			"/usr/share/code/resources/app/product.json",
			"/opt/visual-studio-code/resources/app/product.json",
			"/snap/code/current/usr/share/code/resources/app/product.json",
			filepath.Join(os.Getenv("HOME"), ".vscode-server", "bin", "*", "product.json"),
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		// Handle glob patterns for Linux
		if strings.Contains(path, "*") {
			matches, _ := filepath.Glob(path)
			for _, match := range matches {
				if _, err := os.Stat(match); err == nil {
					return match, nil
				}
			}
		}
	}

	return "", fmt.Errorf("VSCode installation not found in standard locations")
}

// createBackup creates a backup of the original product.json
func (vc *VscodeCommand) createBackup() error {
	backupDir, err := coreutils.GetJfrogBackupDir()
	if err != nil {
		return fmt.Errorf("failed to get JFrog backup directory: %w", err)
	}

	ideBackupDir := filepath.Join(backupDir, "ide", "vscode")
	err = fileutils.CreateDirIfNotExist(ideBackupDir)
	if err != nil {
		return fmt.Errorf("failed to create IDE backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupFileName := "product.json.backup." + timestamp
	vc.backupPath = filepath.Join(ideBackupDir, backupFileName)

	data, err := os.ReadFile(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to read original product.json: %w", err)
	}

	if err := os.WriteFile(vc.backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	log.Info("Backup created at:", vc.backupPath)
	return nil
}

// restoreBackup restores the backup in case of failure
func (vc *VscodeCommand) restoreBackup() error {
	if vc.backupPath == "" {
		return fmt.Errorf("no backup path available")
	}

	data, err := os.ReadFile(vc.backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := os.WriteFile(vc.productPath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}
	return nil
}

// modifyProductJson modifies the VSCode product.json file
func (vc *VscodeCommand) modifyProductJson(repoURL string) error {
	// Check write permissions first
	if err := vc.checkWritePermissions(); err != nil {
		return err
	}

	// Create backup first
	if err := vc.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	var err error
	if runtime.GOOS == "windows" {
		err = vc.modifyWithPowerShell(repoURL)
	} else {
		err = vc.modifyWithSed(repoURL)
	}

	if err != nil {
		if restoreErr := vc.restoreBackup(); restoreErr != nil {
			log.Error("Failed to restore backup:", restoreErr)
		}
		return err
	}

	return nil
}

// modifyWithSed modifies the product.json file using sed
func (vc *VscodeCommand) modifyWithSed(repoURL string) error {
	// Escape special characters for sed
	escapedURL := strings.ReplaceAll(repoURL, "/", "\\/")
	escapedURL = strings.ReplaceAll(escapedURL, "&", "\\&")

	// sed command to replace serviceUrl in the JSON file (handles both compact and formatted JSON)
	sedCommand := fmt.Sprintf(`s/"serviceUrl" *: *"[^"]*"/"serviceUrl": "%s"/g`, escapedURL)

	// Run sed command - different platforms handle -i differently
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		// macOS requires empty string after -i for no backup
		cmd = exec.Command("sed", "-i", "", sedCommand, vc.productPath)
	} else {
		// Linux and other Unix systems
		cmd = exec.Command("sed", "-i", sedCommand, vc.productPath)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to modify product.json with sed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// modifyWithPowerShell modifies the product.json file using PowerShell
func (vc *VscodeCommand) modifyWithPowerShell(repoURL string) error {
	// Escape quotes for PowerShell
	escapedURL := strings.ReplaceAll(repoURL, `"`, `\"`)

	// PowerShell command to replace serviceUrl in the JSON file (handles both compact and formatted JSON)
	// Uses PowerShell's -replace operator which works similar to sed
	psCommand := fmt.Sprintf(`(Get-Content "%s") -replace '"serviceUrl" *: *"[^"]*"', '"serviceUrl": "%s"' | Set-Content "%s"`,
		vc.productPath, escapedURL, vc.productPath)

	// Run PowerShell command
	// Note: This requires the JF CLI to be run as Administrator on Windows
	cmd := exec.Command("powershell", "-Command", psCommand)

	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Access") && strings.Contains(string(output), "denied") {
			return fmt.Errorf("access denied - please run JF CLI as Administrator on Windows")
		}
		return fmt.Errorf("failed to modify product.json with PowerShell: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// getManualSetupInstructions returns manual setup instructions
func (vc *VscodeCommand) getManualSetupInstructions(serviceURL string) string {
	return fmt.Sprintf(ide.VscodeManualInstructionsTemplate, serviceURL, serviceURL)
}
