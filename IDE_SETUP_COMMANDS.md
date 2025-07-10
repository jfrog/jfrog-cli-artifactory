# JFrog CLI IDE Setup Commands

This document describes the IDE setup commands available in JFrog CLI for configuring Visual Studio Code and JetBrains IDEs to use JFrog Artifactory as their plugin/extension repository.

## Overview

The IDE setup commands allow developers to configure their development environments to download extensions and plugins from JFrog Artifactory instead of the default public repositories. This enables organizations to:

- Host approved/curated extensions/plugins in their private repositories
- Ensure consistent development environments across teams
- Maintain control over which extensions/plugins are available
- Work in air-gapped or restricted network environments

## Commands

### VSCode Configuration: `jf rt vscode-config`

Configures Visual Studio Code to use JFrog Artifactory for extensions.

#### Syntax
```bash
jf rt vscode-config <service-url> [flags]
```

#### Aliases
- `jf rt vscode`

#### Parameters

**Required:**
- `<service-url>`: The Artifactory VSCode extensions service URL

**Optional Flags:**
- `--product-json-path`: Path to VSCode product.json file (auto-detected if not provided)
- `--url`: Artifactory server URL (for repository validation)
- `--user`: Username for Artifactory authentication
- `--password`: Password for Artifactory authentication  
- `--access-token`: Access token for Artifactory authentication
- `--server-id`: Pre-configured server ID from JFrog CLI config

#### Service URL Format
```
https://<artifactory-url>/artifactory/api/vscodeextensions/<repo-key>/_apis/public/gallery
```

#### Examples

**Basic usage (no server authentication):**
```bash
jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery
```

**With custom product.json path:**
```bash
jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery --product-json-path="/custom/path/product.json"
```

**With repository validation:**
```bash
jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery --url https://mycompany.jfrog.io --access-token mytoken123
```

**Using pre-configured server:**
```bash
jf rt vscode-config https://mycompany.jfrog.io/artifactory/api/vscodeextensions/vscode-extensions/_apis/public/gallery --server-id my-artifactory
```

#### Behavior

1. **Auto-detection**: Automatically detects VSCode installation paths on Windows, macOS, and Linux
2. **Backup creation**: Creates automatic backup of original product.json file before modification
3. **Configuration update**: Modifies the VSCode product.json file to change the extensions gallery URL
4. **Restart requirement**: VSCode must be restarted for changes to take effect
5. **Permission handling**: On macOS/Linux, may require sudo for system-installed VSCode

#### Expected File Locations

**Windows:**
- `%USERPROFILE%\AppData\Local\Programs\Microsoft VS Code\resources\app\product.json`
- `%PROGRAMFILES%\Microsoft VS Code\resources\app\product.json`

**macOS:**
- `/Applications/Visual Studio Code.app/Contents/Resources/app/product.json`
- `~/Applications/Visual Studio Code.app/Contents/Resources/app/product.json`

**Linux:**
- `/usr/share/code/resources/app/product.json`
- `/opt/visual-studio-code/resources/app/product.json`
- `~/.local/share/applications/code/resources/app/product.json`

### JetBrains Configuration: `jf rt jetbrains-config`

Configures JetBrains IDEs to use JFrog Artifactory for plugins.

#### Syntax
```bash
jf rt jetbrains-config <repository-url> [flags]
```

#### Aliases
- `jf rt jetbrains`

#### Parameters

**Required:**
- `<repository-url>`: The Artifactory JetBrains plugins repository URL

**Optional Flags:**
- `--url`: Artifactory server URL (for repository validation)
- `--user`: Username for Artifactory authentication
- `--password`: Password for Artifactory authentication
- `--access-token`: Access token for Artifactory authentication  
- `--server-id`: Pre-configured server ID from JFrog CLI config

#### Repository URL Format
```
https://<artifactory-url>/artifactory/api/jetbrainsplugins/<repo-key>
```

#### Examples

**Basic usage (no server authentication):**
```bash
jf rt jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins
```

**With repository validation:**
```bash
jf rt jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins --url https://mycompany.jfrog.io --access-token mytoken123
```

**Using pre-configured server:**
```bash
jf rt jetbrains-config https://mycompany.jfrog.io/artifactory/api/jetbrainsplugins/jetbrains-plugins --server-id my-artifactory
```

#### Behavior

1. **Multi-IDE detection**: Automatically detects all installed JetBrains IDEs
2. **Backup creation**: Creates automatic backups of original idea.properties files before modification
3. **Configuration update**: Modifies each IDE's idea.properties file to add the plugins repository URL
4. **Restart requirement**: All IDEs must be restarted for changes to take effect
5. **Supported IDEs**: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua

#### Expected File Locations

**Windows:**
- `%USERPROFILE%\.{IDE_NAME}{VERSION}\config\idea.properties`

**macOS:**
- `~/Library/Application Support/JetBrains/{IDE_NAME}{VERSION}/idea.properties`

**Linux:**  
- `~/.config/JetBrains/{IDE_NAME}{VERSION}/idea.properties`

## Repository Validation

When server configuration flags are provided (`--url`, `--user`, `--password`, `--access-token`, or `--server-id`), the commands will:

1. Authenticate with the Artifactory server
2. Validate that the specified repository exists
3. Confirm the repository is of the correct package type (VSCodeExtensions or JetBrainsPlugin)
4. Extract repository key from the service/repository URL for validation

**Note:** Repository validation is optional. Without server configuration flags, the commands will only modify local IDE configuration files.

## Error Handling

### Common Errors

**Invalid service/repository URL:**
```
Error: Invalid service URL format. Expected: https://<server>/artifactory/api/vscodeextensions/<repo>/_apis/public/gallery
```

**VSCode not found:**
```
Error: VSCode installation not found. Please specify --product-json-path manually.
```

**Permission denied:**
```
Error: Permission denied writing to product.json. Try running with sudo (macOS/Linux) or as Administrator (Windows).
```

**Repository not found (when validation enabled):**
```
Error: Repository 'my-repo' not found or is not a VSCodeExtensions repository
```

**Authentication failed (when validation enabled):**
```
Error: Authentication failed. Please check your credentials.
```

### Troubleshooting

**VSCode extensions not loading from Artifactory:**
1. Verify VSCode was restarted after configuration
2. Check product.json was modified correctly
3. Ensure Artifactory repository is accessible from your network
4. Verify extensions exist in the specified repository

**JetBrains plugins not loading from Artifactory:**
1. Verify all IDEs were restarted after configuration
2. Check idea.properties files were modified correctly
3. Ensure Artifactory repository is accessible from your network
4. Verify plugins exist in the specified repository

**Permission errors:**
- **Windows**: Run Command Prompt as Administrator
- **macOS/Linux**: Use `sudo` prefix for system-installed applications

**Configuration not applied:**
1. Check if backup files were created (indicates successful file access)
2. Manually verify configuration file modifications
3. Check IDE-specific logs for configuration loading errors

## Security Considerations

1. **Authentication**: Server credentials are only used for repository validation, not stored in IDE configuration
2. **HTTPS**: Always use HTTPS URLs for production environments
3. **Access tokens**: Prefer access tokens over username/password authentication
4. **Network access**: Ensure IDE can reach Artifactory server on configured ports
5. **Repository permissions**: Verify appropriate read permissions for users accessing the repository

## Backup and Recovery

Both commands automatically create backups before making changes:

**VSCode backup location:**
- Same directory as product.json with `.backup` extension

**JetBrains backup location:**
- Same directory as idea.properties with `.backup` extension

**Manual recovery:**
```bash
# Restore VSCode configuration
cp /path/to/product.json.backup /path/to/product.json

# Restore JetBrains configuration  
cp /path/to/idea.properties.backup /path/to/idea.properties
```

## Integration with CI/CD

The IDE setup commands can be integrated into development environment setup scripts:

```bash
#!/bin/bash
# Developer onboarding script

# Configure VSCode
jf rt vscode-config https://company.jfrog.io/artifactory/api/vscodeextensions/approved-extensions/_apis/public/gallery

# Configure JetBrains IDEs
jf rt jetbrains-config https://company.jfrog.io/artifactory/api/jetbrainsplugins/approved-plugins

echo "IDE configuration complete. Please restart your IDEs."
```

## Repository Setup Requirements

### Artifactory Repository Configuration

**For VSCode Extensions:**
- Repository Type: Generic
- Package Type: VSCodeExtensions
- Layout: simple-default

**For JetBrains Plugins:**
- Repository Type: Generic  
- Package Type: JetBrainsPlugin
- Layout: simple-default

### Repository Structure

**VSCode Extensions Repository:**
```
<repo-key>/
├── _apis/
│   └── public/
│       └── gallery/
│           ├── extensionquery
│           └── publishers/
└── files/
    └── <publisher>/
        └── <extension>/
            └── <version>/
```

**JetBrains Plugins Repository:**
```
<repo-key>/
├── plugins.xml
└── <plugin-id>/
    └── <version>/
        └── <plugin-name>-<version>.zip
``` 