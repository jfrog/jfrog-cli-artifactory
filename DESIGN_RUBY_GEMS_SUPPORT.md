# JFrog CLI Ruby Gems Support - Design Document

## Overview

This document outlines the design and implementation of Ruby gems support in the JFrog CLI, enabling users to configure Ruby projects to use Artifactory as a Ruby gems repository through the `jf ruby-config` command.

## Background

JFrog Artifactory has supported Ruby gems repositories for some time, but the JFrog CLI lacked native integration for Ruby package management. This implementation adds Ruby configuration support following the established patterns used by other package managers (npm, pip, maven, etc.) in the JFrog CLI ecosystem.

## Architecture

### Core Components

The Ruby gems support follows the established JFrog CLI architecture pattern with three main components:

1. **Project Type Definition** (`jfrog-cli-core`)
2. **Repository Commands** (`jfrog-cli-artifactory`) 
3. **CLI Integration** (`jfrog-cli`)

### Design Patterns

The implementation follows the same architectural patterns as other package managers:

- **ProjectType Enum**: Ruby added to the core project type system
- **Configuration Commands**: `ruby-config` command for project setup
- **Repository URL Generation**: Credential-embedded URLs for gem sources

**Note**: Ruby is intentionally **excluded from the setup command** to keep it focused on configuration only.

## Implementation Details

### 1. Core Project Type Support (`jfrog-cli-core`)

**File**: `common/project/projectconfig.go`

```go
type ProjectType int

const (
    Maven ProjectType = iota
    Gradle
    Npm
    Pip
    Go
    Nuget
    Dotnet
    Terraform
    Poetry
    Pipenv
    Yarn
    Pnpm
    Ruby  // Added Ruby support
)

var ProjectTypes = []string{
    "maven", "gradle", "npm", "pip", "go", "nuget", 
    "dotnet", "terraform", "poetry", "pipenv", "yarn", 
    "pnpm", "ruby"  // Added "ruby"
}
```

**Purpose**: Establishes Ruby as a first-class project type in the JFrog CLI ecosystem.

### 2. Ruby Commands Package (`jfrog-cli-artifactory`)

**File**: `artifactory/commands/ruby/ruby.go`

#### Core Components:

- **RubyCommand Struct**: Main command structure following established patterns
- **Repository URL Generation**: Creates Artifactory-compatible gem source URLs
- **Credential Management**: Handles authentication tokens and user credentials
- **Configuration Management**: Integrates with JFrog CLI config system

#### Key Functions:

```go
// GetRubyGemsRepoUrlWithCredentials generates authenticated repository URLs
func GetRubyGemsRepoUrlWithCredentials(serverDetails *config.ServerDetails, repoName string) string

// GetRubyGemsRepoUrl generates basic repository URLs  
func GetRubyGemsRepoUrl(serverDetails *config.ServerDetails, repoName string) string

// RunGemCommand executes gem CLI commands with proper configuration
func RunGemCommand(args []string) error

// RunConfigCommand handles Ruby project configuration
func RunConfigCommand(buildTool project.ProjectType, args []string) error
```

#### Repository URL Pattern:
```
https://<user>:<token>@<artifactory-url>/artifactory/api/gems/<repo-name>/
```

### 3. CLI Integration (`jfrog-cli`)

#### Command Registration (`buildtools/cli.go`)

```go
{
    Name:         "ruby-config",
    Flags:        cliutils.GetCommandFlags(cliutils.RubyConfig),
    Aliases:      []string{"rubyc"},
    Usage:        rubyconfig.GetDescription(),
    HelpName:     corecommon.CreateUsage("ruby-config", rubyconfig.GetDescription(), rubyconfig.Usage),
    BashComplete: corecommon.CreateBashCompletionFunc(),
    Action: func(c *cli.Context) error {
        return cliutils.CreateConfigCmd(c, project.Ruby)
    },
}
```

#### Command Flags (`utils/cliutils/commandsflags.go`)

Added `RubyConfig` flag configuration for proper integration.

## Usage Examples

### Ruby Configuration

```bash
# Configure Ruby project to use Artifactory
jf ruby-config --repo-resolve my-gems-repo --server-id-resolve my-server

# Alternative using alias
jf rubyc --repo-resolve my-gems-repo --server-id-resolve my-server

# Global configuration
jf ruby-config --global --repo-resolve my-gems-repo --server-id-resolve my-server
```

## Architecture Decisions

### Why Ruby is NOT in Setup Command

The `setup` command is designed for interactive repository configuration across multiple package managers. Ruby gems have specific characteristics that make them better suited for project-specific configuration:

1. **Project-centric**: Ruby projects typically have specific gem dependencies
2. **Gemfile Integration**: Works better with Ruby's native dependency management
3. **Development vs. Publishing**: Different workflows for consuming vs. publishing gems
4. **Repository Complexity**: Gems repositories often need custom configuration

Therefore, Ruby configuration is available only through:
- `jf ruby-config` command
- `jf rubyc` alias

But **NOT** through:
- `jf setup ruby` (deliberately excluded)

## Integration Points

### File Generation

The `ruby-config` command generates appropriate configuration files for Ruby projects to use Artifactory repositories for gem resolution.

### Credential Management

Integrates with JFrog CLI's existing credential management system using:
- Server configurations (`jf config`)
- Access tokens
- Username/password authentication

## Testing

### Verification Commands

```bash
# Verify ruby-config works
jf ruby-config --help

# Verify rubyc alias works  
jf rubyc --help

# Verify setup ruby does NOT work
jf setup ruby  # Should show "not supported" error
```

### Integration Testing

The implementation includes proper error handling and validation for:
- Repository connectivity
- Authentication
- Configuration file generation

## Future Enhancements

Potential future enhancements could include:
- Enhanced gem source management
- Bundle configuration integration
- Publishing workflow support
- Enhanced repository validation

## Conclusion

This implementation provides comprehensive Ruby configuration support while maintaining clean separation from the interactive setup command, following JFrog CLI's established architectural patterns and providing a solid foundation for Ruby development workflows with Artifactory.
``` 