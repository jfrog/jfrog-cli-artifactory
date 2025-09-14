package ide

const VscodeConfigDescription = `
Configure VSCode to use JFrog Artifactory for extensions.

The service URL should be in the format:
https://<artifactory-url>/artifactory/api/aieditorextensions/<repo-key>/_apis/public/gallery

Examples:
  jf vscode-config https://mycompany.jfrog.io/artifactory/api/aieditorextensions/vscode-extensions/_apis/public/gallery

This command will:
- Modify the VSCode product.json file to change the extensions gallery URL
- Create an automatic backup before making changes
- Require VSCode to be restarted to apply changes

Optional: Provide server configuration flags (--url, --user, --password, --access-token, or --server-id) 
to enable repository validation. Without these flags, the command will only modify the local VSCode configuration.

Note: On macOS/Linux, you may need to run with sudo for system-installed VSCode.
`

const JetbrainsConfigDescription = `
Configure JetBrains IDEs to use JFrog Artifactory for plugins.

The repository URL should be in the format:
https://<artifactory-url>/artifactory/api/jetbrainsplugins/<repo-key>

Examples:
  jf jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins

This command will:
- Detect all installed JetBrains IDEs
- Modify each IDE's idea.properties file to add the plugins repository URL
- Create automatic backups before making changes
- Require IDEs to be restarted to apply changes

Optional: Provide server configuration flags (--url, --user, --password, --access-token, or --server-id) 
to enable repository validation. Without these flags, the command will only modify the local IDE configuration.

Supported IDEs: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua
`
