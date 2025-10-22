package commands

const JetbrainsManualInstructionsTemplate = `
Manual JetBrains IDE Setup Instructions:
=======================================

1. Close all JetBrains IDEs

2. Locate your IDE configuration directory:
   %s

   Examples:
   • IntelliJ IDEA: IntelliJIdea2023.3/idea.properties
   • PyCharm: PyCharm2023.3/idea.properties
   • WebStorm: WebStorm2023.3/idea.properties

3. Open or create the idea.properties file in a text editor

4. Add or modify the following line:
   idea.plugins.host=%s

5. Save the file and restart your IDE

Repository URL: %s

Supported IDEs: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua
`

const VscodeManualInstructionsTemplate = `
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
`

const VscodeMacOSPermissionError = `insufficient permissions to modify VSCode configuration.

VSCode is installed in /Applications/ which requires elevated privileges to modify.

To fix this, run the command with sudo:

    sudo jf ide setup vscode '%s'

This is the same approach that works with manual editing:
    sudo nano "%s"

Note: This does NOT require disabling System Integrity Protection (SIP).
The file is owned by admin and %s needs elevated privileges to write to it.

Alternative: Install VSCode in a user-writable location like ~/Applications/`

const VscodeGenericPermissionError = `insufficient permissions to modify VSCode configuration.

To fix this, try running the command with elevated privileges:
    sudo jf ide setup vscode '%s'

Or use the manual setup instructions provided in the error output.`
