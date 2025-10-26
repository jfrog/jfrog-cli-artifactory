package aieditorextensions

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jfrog/jfrog-cli-artifactory/ide/common"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"github.com/jfrog/jfrog-cli-core/v2/utils/coreutils"
	"github.com/jfrog/jfrog-client-go/utils/io/fileutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// VSCodeForkCommand represents a generic command for VSCode-based IDEs
type VSCodeForkCommand struct {
	forkConfig    *VSCodeForkConfig
	serviceURL    string
	productPath   string
	backupPath    string
	serverDetails *config.ServerDetails
	repoKey       string
	isDirectURL   bool
	updateMode    string
}

// NewVSCodeForkCommand creates a new command for a VSCode-based IDE
func NewVSCodeForkCommand(forkConfig *VSCodeForkConfig, repoKey, productPath, serviceURL string) *VSCodeForkCommand {
	return &VSCodeForkCommand{
		forkConfig:  forkConfig,
		repoKey:     repoKey,
		productPath: productPath,
		serviceURL:  serviceURL,
		isDirectURL: false,
	}
}

func (vc *VSCodeForkCommand) SetServerDetails(serverDetails *config.ServerDetails) *VSCodeForkCommand {
	vc.serverDetails = serverDetails
	return vc
}

func (vc *VSCodeForkCommand) ServerDetails() (*config.ServerDetails, error) {
	return vc.serverDetails, nil
}

func (vc *VSCodeForkCommand) CommandName() string {
	return "setup-" + vc.forkConfig.Name
}

func (vc *VSCodeForkCommand) SetDirectURL(isDirectURL bool) *VSCodeForkCommand {
	vc.isDirectURL = isDirectURL
	return vc
}

func (vc *VSCodeForkCommand) SetUpdateMode(updateMode string) *VSCodeForkCommand {
	vc.updateMode = updateMode
	return vc
}

// Run executes the VSCode fork configuration command
func (vc *VSCodeForkCommand) Run() error {
	log.Info(fmt.Sprintf("Setting up %s to use JFrog Artifactory...", vc.forkConfig.DisplayName))

	// Step 1: Detect installation if no path provided
	if vc.productPath == "" {
		detectedPath, err := vc.detectInstallation()
		if err != nil {
			return fmt.Errorf("failed to detect %s installation: %w\n\n%s",
				vc.forkConfig.DisplayName, err, vc.getManualSetupInstructions())
		}
		vc.productPath = detectedPath
		log.Info(fmt.Sprintf("Detected %s at: %s", vc.forkConfig.DisplayName, vc.productPath))
	}

	// Step 2: Check write permissions
	if err := vc.checkWritePermissions(); err != nil {
		return vc.handlePermissionError()
	}

	// Step 3: Create backup
	if err := vc.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Step 4: Modify product.json
	if err := vc.modifyProductJson(); err != nil {
		// Attempt to restore backup on failure
		if restoreErr := vc.restoreBackup(); restoreErr != nil {
			log.Warn("Failed to restore backup:", restoreErr)
		}
		return fmt.Errorf("failed to modify product.json: %w\n\n%s", err, vc.getManualSetupInstructions())
	}

	log.Info(fmt.Sprintf("%s configuration updated successfully. Repository URL:", vc.forkConfig.DisplayName), vc.serviceURL)

	// Step 5: Configure update mode if specified
	if vc.updateMode != "" {
		if err := SetUpdateMode(UpdateMode(vc.updateMode)); err != nil {
			log.Warn("Failed to set update mode:", err)
			log.Warn(fmt.Sprintf("You can manually add '\"update.mode\": \"%s\"' to your settings.json", vc.updateMode))
		}
	}

	log.Info(fmt.Sprintf("Configuration complete! Please restart %s to apply changes", vc.forkConfig.DisplayName))
	return nil
}

// detectInstallation attempts to find the IDE installation
func (vc *VSCodeForkCommand) detectInstallation() (string, error) {
	paths := vc.forkConfig.GetAllInstallPaths()

	for _, path := range paths {
		// Expand environment variables and home directory
		expandedPath := os.ExpandEnv(path)
		if strings.HasPrefix(expandedPath, "~") {
			home, err := os.UserHomeDir()
			if err == nil {
				expandedPath = filepath.Join(home, expandedPath[2:])
			}
		}

		productJsonPath := filepath.Join(expandedPath, vc.forkConfig.ProductJson)
		if fileutils.IsPathExists(productJsonPath, false) {
			return productJsonPath, nil
		}
	}

	return "", fmt.Errorf("%s installation not found in standard locations", vc.forkConfig.DisplayName)
}

// checkWritePermissions verifies write access to product.json
func (vc *VSCodeForkCommand) checkWritePermissions() error {
	file, err := os.OpenFile(vc.productPath, os.O_RDWR, 0644)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("insufficient permissions to modify %s configuration", vc.forkConfig.DisplayName)
		}
		return err
	}
	defer file.Close()
	return nil
}

// createBackup creates a backup of product.json
func (vc *VSCodeForkCommand) createBackup() error {
	timestamp := time.Now().Format("20060102-150405")
	vc.backupPath = fmt.Sprintf("%s.backup_%s", vc.productPath, timestamp)

	// Read source file
	sourceFile, err := os.Open(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	// Create backup file
	backupFile, err := os.Create(vc.backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer backupFile.Close()

	// Copy content
	if _, err := io.Copy(backupFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy content: %w", err)
	}

	log.Debug("Created backup at:", vc.backupPath)
	return nil
}

// restoreBackup restores product.json from backup
func (vc *VSCodeForkCommand) restoreBackup() error {
	if vc.backupPath == "" {
		return fmt.Errorf("no backup path available")
	}

	// Read backup file
	backupFile, err := os.Open(vc.backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer backupFile.Close()

	// Create/overwrite target file
	targetFile, err := os.Create(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer targetFile.Close()

	// Copy content
	if _, err := io.Copy(targetFile, backupFile); err != nil {
		return fmt.Errorf("failed to copy content: %w", err)
	}

	log.Info("Restored from backup:", vc.backupPath)
	return nil
}

// modifyProductJson updates the extensionsGallery.serviceUrl in product.json
func (vc *VSCodeForkCommand) modifyProductJson() error {
	// Read existing file
	data, err := os.ReadFile(vc.productPath)
	if err != nil {
		return fmt.Errorf("failed to read product.json: %w", err)
	}

	// Parse as generic map to preserve all fields
	var productData map[string]interface{}
	if err := json.Unmarshal(data, &productData); err != nil {
		return fmt.Errorf("failed to parse product.json: %w", err)
	}

	// Update only the serviceUrl
	if _, exists := productData["extensionsGallery"]; !exists {
		productData["extensionsGallery"] = make(map[string]interface{})
	}

	gallery, ok := productData["extensionsGallery"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("extensionsGallery is not a valid map")
	}
	gallery["serviceUrl"] = vc.serviceURL

	// Write back with proper formatting
	updatedData, err := json.MarshalIndent(productData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated data: %w", err)
	}

	if err := os.WriteFile(vc.productPath, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write product.json: %w", err)
	}

	log.Debug("Successfully updated product.json")
	return nil
}

// validateRepository validates that the repository exists and is the correct type
func (vc *VSCodeForkCommand) validateRepository() error {
	if vc.serverDetails == nil {
		return fmt.Errorf("server details not configured")
	}

	return common.ValidateRepository(vc.repoKey, vc.serverDetails, ApiType)
}

// handlePermissionError handles permission-related errors
func (vc *VSCodeForkCommand) handlePermissionError() error {
	ideName := vc.forkConfig.DisplayName
	var errMsg string
	if runtime.GOOS == "darwin" && strings.Contains(vc.productPath, "/Applications/") {
		errMsg = GetMacOSPermissionError(
			ideName,
			vc.serviceURL,
			vc.productPath,
			coreutils.GetCliExecutableName())
	} else {
		errMsg = GetGenericPermissionError(ideName, vc.serviceURL)
	}
	return fmt.Errorf("%s", errMsg)
}

// getManualSetupInstructions returns manual setup instructions
func (vc *VSCodeForkCommand) getManualSetupInstructions() string {
	return GetManualInstructions(vc.forkConfig.DisplayName, vc.serviceURL)
}
