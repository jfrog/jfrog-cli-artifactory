package aieditorextensions

import "fmt"

// GetManualInstructions returns manual setup instructions for a VSCode fork
func GetManualInstructions(ideName, serviceURL string) string {
	return fmt.Sprintf(`
Manual %s Setup Instructions:
=================================

1. Close %s completely
2. Find product.json in your %s installation directory
3. Edit the "extensionsGallery" section:
   {
     "extensionsGallery": {
       "serviceUrl": "%s",
       ...
     }
   }
4. Save and restart %s

Service URL: %s
`, ideName, ideName, ideName, serviceURL, ideName, serviceURL)
}

// GetMacOSPermissionError returns macOS-specific permission error message
func GetMacOSPermissionError(ideName, serviceURL, productPath string, cliName string) string {
	return fmt.Sprintf(`insufficient permissions to modify %s configuration.

%s is installed in /Applications/ which requires elevated privileges to modify.

To fix this, run the command with sudo:

    sudo jf ide setup %s --repo-key <your-repo-key>

Or with direct URL:

    sudo jf ide setup %s '%s'

This is the same approach that works with manual editing:
    sudo nano "%s"

Note: This does NOT require disabling System Integrity Protection (SIP).
The file is owned by admin and %s needs elevated privileges to write to it.

Alternative: Install %s in a user-writable location like ~/Applications/`,
		ideName, ideName, ideName, ideName, serviceURL, productPath, cliName, ideName)
}

// GetGenericPermissionError returns generic permission error message
func GetGenericPermissionError(ideName, serviceURL string) string {
	return fmt.Sprintf(`insufficient permissions to modify %s configuration.

Try running with elevated privileges:
  • Linux/macOS: sudo jf ide setup %s '%s'
  • Windows: Run PowerShell as Administrator

Or use the manual setup instructions.`,
		ideName, ideName, serviceURL)
}
