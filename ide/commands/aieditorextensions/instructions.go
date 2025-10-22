package aieditorextensions

import "fmt"

// GetManualInstructions returns manual setup instructions for a VSCode fork
func GetManualInstructions(ideName, serviceURL string) string {
	return fmt.Sprintf(`
Manual %s Setup Instructions:
=================================

1. Close %s completely

2. Locate your %s installation directory and find product.json

3. Open the product.json file in a text editor with appropriate permissions:
   • macOS: sudo nano "<path-to-app>/Contents/Resources/app/product.json"
   • Windows: Run editor as Administrator
   • Linux: sudo nano /path/to/resources/app/product.json

4. Find the "extensionsGallery" section and modify the "serviceUrl":
   {
     "extensionsGallery": {
       "serviceUrl": "%s",
       ...
     }
   }

5. Save the file and restart %s

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

To fix this, try running the command with elevated privileges:
    sudo jf ide setup %s --repo-key <your-repo-key>

Or with direct URL:
    sudo jf ide setup %s '%s'

Or use the manual setup instructions provided in the error output.`,
		ideName, ideName, ideName, serviceURL)
}
