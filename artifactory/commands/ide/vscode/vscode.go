package vscode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
}

// NewVscodeCommand creates a new VSCode configuration command
func NewVscodeCommand(serviceURL, productPath, repoKey string) *VscodeCommand {
	return &VscodeCommand{
		serviceURL:  serviceURL,
		productPath: productPath,
		repoKey:     repoKey,
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

// Run executes the VSCode configuration command
func (vc *VscodeCommand) Run() error {
	log.Info("Configuring VSCode extensions repository...")

	// Validate repository if we have server details and repo key
	if vc.serverDetails != nil && vc.repoKey != "" {
		if err := vc.validateRepository(); err != nil {
			return errorutils.CheckError(fmt.Errorf("repository validation failed: %w", err))
		}
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

	log.Info("VSCode configuration updated successfully")
	log.Info("Repository URL:", vc.serviceURL)
	log.Info("Please restart VSCode to apply changes")

	return nil
}

// validateRepository uses the established pattern for repository validation
func (vc *VscodeCommand) validateRepository() error {
	log.Info("Validating repository...")

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
		// Get current user info for better error message
		userInfo := "the current user"
		if user := os.Getenv("USER"); user != "" {
			userInfo = user
		}

		return fmt.Errorf(`insufficient permissions to modify VSCode configuration.

VSCode is installed in /Applications/ which requires elevated privileges to modify.

To fix this, run the command with sudo:

    sudo jf vscode set service-url '%s'

This is the same approach that works with manual editing:
    sudo nano "%s"

Note: This does NOT require disabling System Integrity Protection (SIP).
The file is owned by admin and %s needs elevated privileges to write to it.

Alternative: Install VSCode in a user-writable location like ~/Applications/`, vc.serviceURL, vc.productPath, userInfo)
	}

	return fmt.Errorf(`insufficient permissions to modify VSCode configuration.

To fix this, try running the command with elevated privileges:
    sudo jf vscode set service-url '%s'

Or use the manual setup instructions provided in the error output.`, vc.serviceURL)
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

	// sed command to replace serviceUrl in the JSON file
	sedCommand := fmt.Sprintf(`s/"serviceUrl": "[^"]*"/"serviceUrl": "%s"/g`, escapedURL)

	// Run sed command
	cmd := exec.Command("sed", "-i", "", sedCommand, vc.productPath)
	cmd.Stdin = os.Stdin

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to modify product.json with sed: %w\nOutput: %s", err, string(output))
	}

	if err := vc.verifyModification(repoURL); err != nil {
		return fmt.Errorf("modification verification failed: %w", err)
	}
	return nil
}

// modifyWithPowerShell modifies the product.json file using PowerShell
func (vc *VscodeCommand) modifyWithPowerShell(repoURL string) error {
	// Escape quotes for PowerShell
	escapedURL := strings.ReplaceAll(repoURL, `"`, `\"`)

	// PowerShell command to replace serviceUrl in the JSON file
	// Uses PowerShell's -replace operator which works similar to sed
	psCommand := fmt.Sprintf(`(Get-Content "%s") -replace '"serviceUrl": "[^"]*"', '"serviceUrl": "%s"' | Set-Content "%s"`,
		vc.productPath, escapedURL, vc.productPath)

	// Run PowerShell command
	// Note: This requires the JF CLI to be run as Administrator on Windows
	cmd := exec.Command("powershell", "-Command", psCommand)
	cmd.Stdin = os.Stdin

	if output, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "Access") && strings.Contains(string(output), "denied") {
			return fmt.Errorf("access denied - please run JF CLI as Administrator on Windows")
		}
		return fmt.Errorf("failed to modify product.json with PowerShell: %w\nOutput: %s", err, string(output))
	}

	if err := vc.verifyModification(repoURL); err != nil {
		return fmt.Errorf("modification verification failed: %w", err)
	}
	return nil
}

// verifyModification checks that the serviceUrl was actually changed
func (vc *VscodeCommand) verifyModification(expectedURL string) error {
	data, err := os.ReadFile(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to read file for verification: %w", err)
	}

	if !strings.Contains(string(data), expectedURL) {
		return fmt.Errorf("expected URL %s not found in modified file", expectedURL)
	}

	return nil
}

// getManualSetupInstructions returns manual setup instructions
func (vc *VscodeCommand) getManualSetupInstructions(serviceURL string) string {
	instructions := fmt.Sprintf(`
Manual VSCode Setup Instructions:
=================================

1. Close VSCode completely

2. Locate your VSCode installation directory:
   • macOS: /Applications/Visual Studio Code.app/Contents/Resources/app/
   • Windows: %%LOCALAPPDATA%%\Programs\Microsoft VS Code\resources\app\
   • Linux: /usr/share/code/resources/app/

3. Open the product.json file in a text editor with appropriate permissions:
   • macOS: sudo nano "/Applications/Visual Studio Code.app/Contents/Resources/app/product.json"
   • Windows: Run editor as Administrator
   • Linux: sudo nano /usr/share/code/resources/app/product.json

4. Find the "extensionsGallery" section and modify the "serviceUrl":
   {
     "extensionsGallery": {
       "serviceUrl": "%s",
       ...
     }
   }

5. Save the file and restart VSCode

Service URL: %s
`, serviceURL, serviceURL)

	return instructions
}
